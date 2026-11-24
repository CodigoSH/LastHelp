// Package totp is the second step of signing in: the six-digit codes an
// authenticator app shows (RFC 6238, the settings every app understands), and
// the recovery codes that stand in for the phone when it is lost.
//
// It is pure on purpose: nothing here knows about HTTP or the database, so what
// makes a code valid is read and tested in one place.
package totp

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/base32"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"net/url"
	"strings"
	"time"
)

const (
	// SecretSize is the length of a secret in bytes: 160 bits, what RFC 4226
	// recommends for HMAC-SHA1.
	SecretSize = 20
	// Digits and Period are the ones every authenticator app assumes; anything
	// else is a setting some apps ignore without saying so.
	Digits = 6
	Period = 30 * time.Second
	// Skew is how many periods either side of now a code is accepted for, so a
	// phone whose clock is half a minute out still signs in.
	Skew = 1
)

var b32 = base32.StdEncoding.WithPadding(base32.NoPadding)

// NewSecret draws a new secret.
func NewSecret() ([]byte, error) {
	s := make([]byte, SecretSize)
	if _, err := rand.Read(s); err != nil {
		return nil, err
	}
	return s, nil
}

// EncodeSecret is the secret as an app takes it typed in: base32, no padding.
func EncodeSecret(secret []byte) string { return b32.EncodeToString(secret) }

// DecodeSecret reads what EncodeSecret wrote, forgiving case and spaces.
func DecodeSecret(s string) ([]byte, error) {
	return b32.DecodeString(strings.ToUpper(strings.ReplaceAll(s, " ", "")))
}

// step is the number of the period t falls in.
func step(t time.Time) int64 { return t.Unix() / int64(Period/time.Second) }

// codeAt is the code for one period (RFC 4226 §5.3, dynamic truncation).
func codeAt(secret []byte, counter int64) string {
	var msg [8]byte
	binary.BigEndian.PutUint64(msg[:], uint64(counter))
	mac := hmac.New(sha1.New, secret)
	mac.Write(msg[:])
	sum := mac.Sum(nil)
	offset := sum[len(sum)-1] & 0x0f
	value := binary.BigEndian.Uint32(sum[offset:offset+4]) & 0x7fffffff
	mod := uint32(1)
	for i := 0; i < Digits; i++ {
		mod *= 10
	}
	return fmt.Sprintf("%0*d", Digits, value%mod)
}

// Code is the code an app shows at t.
func Code(secret []byte, t time.Time) string { return codeAt(secret, step(t)) }

// Verify says whether code is right at now, and for which period. A code is
// accepted only for a period after lastStep — the one the account last signed
// in with — so a code that was seen, over a shoulder or in a log, cannot be used
// a second time. The caller stores the period it returns.
func Verify(secret []byte, code string, now time.Time, lastStep int64) (int64, bool) {
	code = strings.ReplaceAll(strings.TrimSpace(code), " ", "")
	if len(code) != Digits {
		return 0, false
	}
	current := step(now)
	for d := -Skew; d <= Skew; d++ {
		s := current + int64(d)
		if s <= lastStep {
			continue
		}
		if hmac.Equal([]byte(codeAt(secret, s)), []byte(code)) {
			return s, true
		}
	}
	return 0, false
}

// URI is what the QR code carries: the otpauth address every authenticator app
// reads. The issuer goes both in the label and as a parameter, as the apps
// that read only one of them expect.
func URI(issuer, account string, secret []byte) string {
	label := url.PathEscape(issuer) + ":" + url.PathEscape(account)
	q := url.Values{}
	q.Set("secret", EncodeSecret(secret))
	q.Set("issuer", issuer)
	q.Set("algorithm", "SHA1")
	q.Set("digits", fmt.Sprint(Digits))
	q.Set("period", fmt.Sprint(int(Period/time.Second)))
	return "otpauth://totp/" + label + "?" + q.Encode()
}

/* ---------------------------------------------------------------------------
 * Recovery codes
 *
 * Each one signs in once instead of a code from the phone. They are random —
 * 80 bits, in an alphabet without the letters people misread (no i, l, o, u) —
 * so a plain SHA-256 is enough to keep them: there is nothing to guess that a
 * slow hash would protect. Only the hash is stored; the person sees them once.
 * ------------------------------------------------------------------------ */

// RecoveryCodes is how many an account gets at a time.
const RecoveryCodes = 10

const recoveryAlphabet = "0123456789abcdefghjkmnpqrstvwxyz"

// recoveryLength is the characters in a code, shown in groups of four.
const recoveryLength = 16

// NewRecoveryCodes draws a fresh set.
func NewRecoveryCodes() ([]string, error) {
	out := make([]string, RecoveryCodes)
	buf := make([]byte, recoveryLength)
	for i := range out {
		if _, err := rand.Read(buf); err != nil {
			return nil, err
		}
		var b strings.Builder
		for j, c := range buf {
			if j > 0 && j%4 == 0 {
				b.WriteByte('-')
			}
			b.WriteByte(recoveryAlphabet[c&31])
		}
		out[i] = b.String()
	}
	return out, nil
}

// NormalizeRecoveryCode is what was typed, as it was generated: lower case, no
// dashes or spaces, and the letters people write for digits read as the digits.
func NormalizeRecoveryCode(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		switch r {
		case '-', ' ', '\t':
			continue
		case 'i', 'l':
			r = '1'
		case 'o':
			r = '0'
		}
		b.WriteRune(r)
	}
	return b.String()
}

// HashRecoveryCode is what is stored for a code, typed or generated.
func HashRecoveryCode(code string) string {
	sum := sha256.Sum256([]byte(NormalizeRecoveryCode(code)))
	return hex.EncodeToString(sum[:])
}

// LooksLikeTOTP says whether what was typed is a code from the app rather than
// a recovery code: the six digits and nothing else.
func LooksLikeTOTP(s string) bool {
	s = strings.ReplaceAll(strings.TrimSpace(s), " ", "")
	if len(s) != Digits {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
