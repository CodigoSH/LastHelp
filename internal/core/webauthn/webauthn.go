// Package webauthn checks what a browser sends when a passkey is created or
// used (W3C Web Authentication, level 2), with nothing but the standard
// library.
//
// It is the part of passkeys that decides: whether the browser was on this
// site, whether the person was there and verified themselves, and whether the
// signature is the passkey's. Where the challenge came from, and which account
// a passkey belongs to, is the server's business; nothing here knows about HTTP
// or the database.
//
// Two kinds of key are accepted, the two every authenticator offers: ES256
// (P-256) and EdDSA (Ed25519). Attestation is asked for as "none" and not
// judged: what is trusted is that the account holder made the passkey while
// signed in, not who built the authenticator.
package webauthn

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
)

// The algorithms, by their COSE numbers (what the browser is offered).
const (
	AlgES256 int64 = -7
	AlgEdDSA int64 = -8
)

// The flags of authenticator data.
const (
	flagUserPresent  = 0x01
	flagUserVerified = 0x04
	flagAttested     = 0x40
	flagExtensions   = 0x80
)

// The two ceremonies, as clientDataJSON names them.
const (
	TypeCreate = "webauthn.create"
	TypeGet    = "webauthn.get"
)

// ErrRefused is what every check returns when the answer is no; the wrapped
// text says which check, for the logs and the tests, never for the browser.
var ErrRefused = errors.New("webauthn: refused")

func refuse(why string) error { return fmt.Errorf("%w: %s", ErrRefused, why) }

// B64 is the base64 the browser and the server speak: URL-safe, no padding.
var B64 = base64.RawURLEncoding

/* ---------------------------------------------------------------------------
 * clientDataJSON — what the browser says happened
 * ------------------------------------------------------------------------ */

// ClientData is the part of clientDataJSON that is checked.
type ClientData struct {
	Type        string `json:"type"`
	Challenge   string `json:"challenge"`
	Origin      string `json:"origin"`
	CrossOrigin bool   `json:"crossOrigin"`
}

// CheckClientData reads clientDataJSON and checks that it is the ceremony
// expected, on this site's origin and not inside another site's frame. It
// returns the challenge, for the caller to find among the ones it issued.
func CheckClientData(raw []byte, wantType, origin string) ([]byte, error) {
	var cd ClientData
	if err := json.Unmarshal(raw, &cd); err != nil {
		return nil, refuse("clientDataJSON is not JSON")
	}
	if cd.Type != wantType {
		return nil, refuse("the ceremony is " + cd.Type)
	}
	if cd.Origin != origin {
		return nil, refuse("the origin is " + cd.Origin + ", not " + origin)
	}
	if cd.CrossOrigin {
		return nil, refuse("the page was inside another site")
	}
	challenge, err := B64.DecodeString(cd.Challenge)
	if err != nil || len(challenge) < 16 {
		return nil, refuse("the challenge is not one")
	}
	return challenge, nil
}

/* ---------------------------------------------------------------------------
 * authenticatorData — what the authenticator signs
 * ------------------------------------------------------------------------ */

// AuthData is authenticator data, read.
type AuthData struct {
	RPIDHash  []byte
	Flags     byte
	SignCount uint32
	// Present only when a passkey is created.
	CredentialID []byte
	PublicKey    []byte // the COSE key, as sent; it is what is stored
}

// ParseAuthData reads authenticator data.
func ParseAuthData(b []byte) (AuthData, error) {
	var ad AuthData
	if len(b) < 37 {
		return ad, refuse("authenticator data is too short")
	}
	ad.RPIDHash = b[:32]
	ad.Flags = b[32]
	ad.SignCount = binary.BigEndian.Uint32(b[33:37])
	rest := b[37:]
	if ad.Flags&flagAttested != 0 {
		if len(rest) < 18 {
			return ad, refuse("attested credential data is too short")
		}
		n := int(binary.BigEndian.Uint16(rest[16:18]))
		rest = rest[18:]
		if n == 0 || n > 1023 || len(rest) < n {
			return ad, refuse("the credential id has a bad length")
		}
		ad.CredentialID = rest[:n]
		rest = rest[n:]
		_, after, err := decodeCBOR(rest)
		if err != nil {
			return ad, refuse("the public key is not CBOR")
		}
		ad.PublicKey = rest[:len(rest)-len(after)]
		rest = after
	}
	if ad.Flags&flagExtensions != 0 {
		_, after, err := decodeCBOR(rest)
		if err != nil {
			return ad, refuse("the extensions are not CBOR")
		}
		rest = after
	}
	if len(rest) != 0 {
		return ad, refuse("authenticator data has bytes left over")
	}
	return ad, nil
}

// Check says whether the authenticator was asked for this site, and whether the
// person was there and verified themselves — with a fingerprint, a face or a
// PIN. A passkey replaces both the password and the second step, so a passkey
// used without verification is refused.
func (ad AuthData) Check(rpID string) error {
	want := sha256.Sum256([]byte(rpID))
	if !bytes.Equal(ad.RPIDHash, want[:]) {
		return refuse("the authenticator answered for another site")
	}
	if ad.Flags&flagUserPresent == 0 {
		return refuse("nobody was present")
	}
	if ad.Flags&flagUserVerified == 0 {
		return refuse("the person was not verified")
	}
	return nil
}

// ParseAttestationObject reads what a browser sends when a passkey is created
// and returns its authenticator data. The attestation statement is not judged.
func ParseAttestationObject(raw []byte) (AuthData, error) {
	v, rest, err := decodeCBOR(raw)
	if err != nil || len(rest) != 0 {
		return AuthData{}, refuse("the attestation object is not CBOR")
	}
	m, ok := v.(map[any]any)
	if !ok {
		return AuthData{}, refuse("the attestation object is not a map")
	}
	authData, ok := m["authData"].([]byte)
	if !ok {
		return AuthData{}, refuse("the attestation object has no authData")
	}
	ad, err := ParseAuthData(authData)
	if err != nil {
		return ad, err
	}
	if ad.CredentialID == nil {
		return ad, refuse("no passkey was created")
	}
	return ad, nil
}

/* ---------------------------------------------------------------------------
 * The public key and the signature
 * ------------------------------------------------------------------------ */

// PublicKey is a passkey's key, ready to check a signature.
type PublicKey struct {
	Alg int64
	ec  *ecdsa.PublicKey
	ed  ed25519.PublicKey
}

func intKey(m map[any]any, k int64) (int64, bool) {
	v, ok := m[k].(int64)
	return v, ok
}

func bytesKey(m map[any]any, k int64) ([]byte, bool) {
	v, ok := m[k].([]byte)
	return v, ok
}

// ParsePublicKey reads a COSE key (RFC 9053) of one of the two kinds accepted.
func ParsePublicKey(cose []byte) (PublicKey, error) {
	v, rest, err := decodeCBOR(cose)
	if err != nil || len(rest) != 0 {
		return PublicKey{}, refuse("the key is not CBOR")
	}
	m, ok := v.(map[any]any)
	if !ok {
		return PublicKey{}, refuse("the key is not a map")
	}
	kty, _ := intKey(m, 1)
	alg, _ := intKey(m, 3)
	crv, _ := intKey(m, -1)
	x, _ := bytesKey(m, -2)
	switch {
	case kty == 2 && alg == AlgES256 && crv == 1:
		y, _ := bytesKey(m, -3)
		if len(x) != 32 || len(y) != 32 {
			return PublicKey{}, refuse("the P-256 point has a bad size")
		}
		// It refuses a point that is not on the curve.
		key, err := ecdsa.ParseUncompressedPublicKey(elliptic.P256(), append(append([]byte{4}, x...), y...))
		if err != nil {
			return PublicKey{}, refuse("the P-256 point is not on the curve")
		}
		return PublicKey{Alg: AlgES256, ec: key}, nil
	case kty == 1 && alg == AlgEdDSA && crv == 6:
		if len(x) != ed25519.PublicKeySize {
			return PublicKey{}, refuse("the Ed25519 key has a bad size")
		}
		return PublicKey{Alg: AlgEdDSA, ed: ed25519.PublicKey(x)}, nil
	}
	return PublicKey{}, refuse(fmt.Sprintf("a key of kind %d, algorithm %d, curve %d is not accepted", kty, alg, crv))
}

// Verify checks the signature an authenticator made over its authenticator data
// and the hash of clientDataJSON — the two together, so neither can be swapped.
func (k PublicKey) Verify(authData, clientDataJSON, sig []byte) error {
	sum := sha256.Sum256(clientDataJSON)
	msg := append(append([]byte(nil), authData...), sum[:]...)
	switch k.Alg {
	case AlgES256:
		digest := sha256.Sum256(msg)
		if k.ec == nil || !ecdsa.VerifyASN1(k.ec, digest[:], sig) {
			return refuse("the signature is not the passkey's")
		}
		return nil
	case AlgEdDSA:
		if k.ed == nil || !ed25519.Verify(k.ed, msg, sig) {
			return refuse("the signature is not the passkey's")
		}
		return nil
	}
	return refuse("no key")
}

// CounterAllows says whether a signature counter may follow the stored one. A
// counter that does not go up, from an authenticator that counts, means the
// passkey was copied. Authenticators that do not count send zero every time.
func CounterAllows(stored, sent uint32) bool {
	if stored == 0 && sent == 0 {
		return true
	}
	return sent > stored
}
