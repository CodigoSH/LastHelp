package totp

import (
	"net/url"
	"strings"
	"testing"
	"time"
)

// The SHA-1 test vectors of RFC 6238, appendix B. The RFC gives eight digits;
// six are the last six of those, which is what dynamic truncation yields.
func TestCodesMatchTheRFC(t *testing.T) {
	secret := []byte("12345678901234567890")
	cases := []struct {
		unix int64
		want string
	}{
		{59, "287082"},
		{1111111109, "081804"},
		{1111111111, "050471"},
		{1234567890, "005924"},
		{2000000000, "279037"},
		{20000000000, "353130"},
	}
	for _, c := range cases {
		if got := Code(secret, time.Unix(c.unix, 0)); got != c.want {
			t.Errorf("Code at %d = %s, want %s", c.unix, got, c.want)
		}
	}
}

func TestVerifyAcceptsThePeriodEitherSideAndNothingFurther(t *testing.T) {
	secret := []byte("12345678901234567890")
	now := time.Unix(1111111111, 0)
	for _, d := range []time.Duration{-Period, 0, Period} {
		code := Code(secret, now.Add(d))
		if _, ok := Verify(secret, code, now, 0); !ok {
			t.Errorf("a code %v away was refused", d)
		}
	}
	for _, d := range []time.Duration{-2 * Period, 2 * Period} {
		if _, ok := Verify(secret, Code(secret, now.Add(d)), now, 0); ok {
			t.Errorf("a code %v away was accepted", d)
		}
	}
	for _, bad := range []string{"", "12345", "1234567", "abcdef"} {
		if _, ok := Verify(secret, bad, now, 0); ok {
			t.Errorf("%q was accepted", bad)
		}
	}
}

// A code that signed in once does not sign in again — nor does one from an
// earlier period still inside the window.
func TestACodeIsNotAcceptedTwice(t *testing.T) {
	secret := []byte("12345678901234567890")
	now := time.Unix(1111111111, 0)
	code := Code(secret, now)
	used, ok := Verify(secret, code, now, 0)
	if !ok {
		t.Fatal("the code was refused the first time")
	}
	if _, ok := Verify(secret, code, now, used); ok {
		t.Error("the same code was accepted a second time")
	}
	if _, ok := Verify(secret, Code(secret, now.Add(-Period)), now, used); ok {
		t.Error("an earlier code was accepted after a later one was used")
	}
	if _, ok := Verify(secret, Code(secret, now.Add(Period)), now, used); !ok {
		t.Error("the next period's code was refused")
	}
}

func TestTheSecretRoundTripsAsAppsTypeIt(t *testing.T) {
	s, err := NewSecret()
	if err != nil || len(s) != SecretSize {
		t.Fatalf("NewSecret = %d bytes, %v", len(s), err)
	}
	enc := EncodeSecret(s)
	if strings.Contains(enc, "=") {
		t.Errorf("%q carries padding, which some apps refuse", enc)
	}
	spaced := strings.ToLower(enc[:4] + " " + enc[4:])
	back, err := DecodeSecret(spaced)
	if err != nil || string(back) != string(s) {
		t.Errorf("DecodeSecret(%q) did not give the secret back: %v", spaced, err)
	}
}

func TestTheURIIsWhatAppsRead(t *testing.T) {
	u, err := url.Parse(URI("Acme", "ana garcía", []byte("12345678901234567890")))
	if err != nil {
		t.Fatal(err)
	}
	if u.Scheme != "otpauth" || u.Host != "totp" {
		t.Errorf("scheme/host = %s/%s", u.Scheme, u.Host)
	}
	if label, _ := url.PathUnescape(strings.TrimPrefix(u.EscapedPath(), "/")); label != "Acme:ana garcía" {
		t.Errorf("label = %q", label)
	}
	q := u.Query()
	if q.Get("secret") != "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ" || q.Get("issuer") != "Acme" ||
		q.Get("digits") != "6" || q.Get("period") != "30" || q.Get("algorithm") != "SHA1" {
		t.Errorf("query = %v", q)
	}
}

func TestRecoveryCodes(t *testing.T) {
	codes, err := NewRecoveryCodes()
	if err != nil || len(codes) != RecoveryCodes {
		t.Fatalf("NewRecoveryCodes = %d, %v", len(codes), err)
	}
	seen := map[string]bool{}
	for _, c := range codes {
		if len(c) != 19 || c[4] != '-' || c[9] != '-' || c[14] != '-' {
			t.Errorf("%q is not four groups of four", c)
		}
		if strings.ContainsAny(c, "ilou") {
			t.Errorf("%q uses a letter people misread", c)
		}
		if seen[c] {
			t.Errorf("%q drawn twice", c)
		}
		seen[c] = true
	}
	// Typed back as people type: upper case, no dashes, an O for a zero.
	c := "0abc-1def-2ghj-3kmn"
	typed := "OABC 1DEF2GHJ-3KMN"
	if HashRecoveryCode(typed) != HashRecoveryCode(c) {
		t.Errorf("%q does not match %q", typed, c)
	}
	if HashRecoveryCode("0abc-1def-2ghj-3kmp") == HashRecoveryCode(c) {
		t.Error("two different codes hash the same")
	}
}

func TestLooksLikeTOTP(t *testing.T) {
	for s, want := range map[string]bool{"123456": true, " 123 456 ": true, "12345": false, "k7fq-2mzx-9wpr-h4tn": false, "12345a": false} {
		if LooksLikeTOTP(s) != want {
			t.Errorf("LooksLikeTOTP(%q) = %v", s, !want)
		}
	}
}
