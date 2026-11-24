package webauthn

import (
	"bytes"
	"errors"
	"testing"

	"git.codigosh.com/CodigoSH/LastHelp/internal/core/webauthn/softkey"
)

const (
	rpID   = "dash.example.lan"
	origin = "https://dash.example.lan"
)

var challenge = bytes.Repeat([]byte{7}, 32)

const both = softkey.UserPresent | softkey.UserVerified

// A passkey is created, read back and then signs in — for both kinds of key.
func TestAPasskeyIsCreatedAndSigns(t *testing.T) {
	for _, alg := range []int64{AlgES256, AlgEdDSA} {
		k := softkey.New(alg)
		ad, err := ParseAttestationObject(k.Create(rpID, both))
		if err != nil {
			t.Fatalf("alg %d: %v", alg, err)
		}
		if err := ad.Check(rpID); err != nil {
			t.Fatalf("alg %d: %v", alg, err)
		}
		if !bytes.Equal(ad.CredentialID, k.CredentialID) {
			t.Fatalf("alg %d: credential id %x, want %x", alg, ad.CredentialID, k.CredentialID)
		}
		pub, err := ParsePublicKey(ad.PublicKey)
		if err != nil || pub.Alg != alg {
			t.Fatalf("alg %d: key %v %v", alg, pub.Alg, err)
		}

		cd := softkey.ClientData(TypeGet, challenge, origin)
		got, err := CheckClientData(cd, TypeGet, origin)
		if err != nil || !bytes.Equal(got, challenge) {
			t.Fatalf("alg %d: client data %v", alg, err)
		}
		authData, sig := k.Get(rpID, both, cd)
		read, err := ParseAuthData(authData)
		if err != nil || read.Check(rpID) != nil {
			t.Fatalf("alg %d: authenticator data %v", alg, err)
		}
		if err := pub.Verify(authData, cd, sig); err != nil {
			t.Fatalf("alg %d: a good signature was refused: %v", alg, err)
		}

		// Nothing signed can be changed afterwards.
		other := softkey.ClientData(TypeGet, bytes.Repeat([]byte{8}, 32), origin)
		if pub.Verify(authData, other, sig) == nil {
			t.Errorf("alg %d: a signature was accepted for other client data", alg)
		}
		changed := append([]byte(nil), authData...)
		changed[32] ^= softkey.UserVerified
		if pub.Verify(changed, cd, sig) == nil {
			t.Errorf("alg %d: a signature was accepted for changed flags", alg)
		}
		stranger := softkey.New(alg)
		_, foreign := stranger.Get(rpID, both, cd)
		if pub.Verify(authData, cd, foreign) == nil {
			t.Errorf("alg %d: another passkey's signature was accepted", alg)
		}
	}
}

func TestTheAuthenticatorMustHaveBeenAskedForThisSiteAndVerifyThePerson(t *testing.T) {
	k := softkey.New(AlgES256)
	cd := softkey.ClientData(TypeGet, challenge, origin)
	for name, c := range map[string]struct {
		rp    string
		flags byte
	}{
		"another site":         {"evil.example", both},
		"nobody present":       {rpID, softkey.UserVerified},
		"the person unchecked": {rpID, softkey.UserPresent},
	} {
		ad, _ := k.Get(c.rp, c.flags, cd)
		read, err := ParseAuthData(ad)
		if err != nil {
			t.Fatal(err)
		}
		if err := read.Check(rpID); !errors.Is(err, ErrRefused) {
			t.Errorf("%s: %v, want refused", name, err)
		}
	}
}

func TestClientDataMustBeThisCeremonyOnThisOrigin(t *testing.T) {
	cases := map[string][]byte{
		"the other ceremony": softkey.ClientData(TypeCreate, challenge, origin),
		"another origin":     softkey.ClientData(TypeGet, challenge, "https://dash.example.lan.evil.example"),
		"plain http":         softkey.ClientData(TypeGet, challenge, "http://dash.example.lan"),
		"a short challenge":  softkey.ClientData(TypeGet, []byte{1, 2, 3}, origin),
		"inside a frame":     []byte(`{"type":"webauthn.get","challenge":"` + B64.EncodeToString(challenge) + `","origin":"` + origin + `","crossOrigin":true}`),
		"not JSON":           []byte(`{"type":`),
	}
	for name, cd := range cases {
		if _, err := CheckClientData(cd, TypeGet, origin); !errors.Is(err, ErrRefused) {
			t.Errorf("%s: %v, want refused", name, err)
		}
	}
}

func TestOnlyTheTwoKindsOfKeyAreAccepted(t *testing.T) {
	k := softkey.New(AlgES256)
	cose := k.COSE()
	// The last byte belongs to y: change it and the point leaves the curve.
	off := append([]byte(nil), cose...)
	off[len(off)-1] ^= 0x01
	if _, err := ParsePublicKey(off); !errors.Is(err, ErrRefused) {
		t.Errorf("a point off the curve: %v, want refused", err)
	}
	// An RSA key (RS256, -257), which this server does not offer.
	rsa := []byte{0xa2, 0x01, 0x03, 0x03, 0x39, 0x01, 0x00}
	if _, err := ParsePublicKey(rsa); !errors.Is(err, ErrRefused) {
		t.Errorf("an RSA key: %v, want refused", err)
	}
}

func TestACopiedPasskeyIsCaughtByItsCounter(t *testing.T) {
	for _, c := range []struct {
		stored, sent uint32
		want         bool
	}{
		{0, 0, true}, {0, 1, true}, {5, 6, true},
		{5, 5, false}, {5, 4, false}, {5, 0, false},
	} {
		if got := CounterAllows(c.stored, c.sent); got != c.want {
			t.Errorf("CounterAllows(%d, %d) = %v, want %v", c.stored, c.sent, got, c.want)
		}
	}
}

func TestCBORThatIsNotWhatAnAuthenticatorWritesIsRefused(t *testing.T) {
	deep := bytes.Repeat([]byte{0x81}, maxDepth+2) // arrays inside arrays
	deep = append(deep, 0x00)
	for name, b := range map[string][]byte{
		"cut short":             {0xa1, 0x01},
		"indefinite length":     {0x5f, 0x41, 0x00, 0xff},
		"a tag":                 {0xc0, 0x00},
		"a float":               {0xfa, 0, 0, 0, 0},
		"too deep":              deep,
		"a key twice":           {0xa2, 0x01, 0x00, 0x01, 0x00},
		"a byte string key":     {0xa1, 0x41, 0x00, 0x00},
		"a length past the end": {0x5a, 0xff, 0xff, 0xff, 0xff},
	} {
		if _, _, err := decodeCBOR(b); err == nil {
			t.Errorf("%s was read", name)
		}
	}
	if _, err := ParsePublicKey(append(softkey.New(AlgEdDSA).COSE(), 0x00)); err == nil {
		t.Error("a key with a byte left over was read")
	}
}
