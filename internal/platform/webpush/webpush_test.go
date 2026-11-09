package webpush

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"encoding/asn1"
	"encoding/binary"
	"encoding/json"
	"math/big"
	"strings"
	"testing"
	"time"
)

// The example of RFC 8291, appendix A: one message, with every input fixed, and the body it must
// become. Passing it is what says this is the format the push services read, and not only a format
// this file can read back.
const (
	rfcPlaintext = "When I grow up, I want to be a watermelon"
	rfcASPrivate = "yfWPiYE-n46HLnH0KqZOF1fJJU3MYrct3AELtAQ-oRw"
	rfcUAPublic  = "BCVxsr7N_eNgVRqvHtD0zTZsEc6-VV-JvLexhqUzORcxaOzi6-AYWXvTBHm4bjyPjs7Vd8pZGH6SRpkNtoIAiw4"
	rfcAuth      = "BTBZMqHH6r4Tts7J_aSIgg"
	rfcSalt      = "DGv6ra1nlYgDCS1FRnbzlw"
	rfcBody      = "DGv6ra1nlYgDCS1FRnbzlwAAEABBBP4z9KsN6nGRTbVYI_c7VJSPQTBtkgcy27mlmlMoZIIgDll6e3vCYLocInmYWAmS6TlzAC8wEqKK6PBru3jl7A_yl95bQpu6cVPTpK4Mqgkf1CXztLVBSt2Ks3oZwbuwXPXLWyouBWLVWGNWQexSgSxsj_Qulcy4a-fN"
)

func mustDecode(t *testing.T, s string) []byte {
	t.Helper()
	b, err := Decode(s)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestTheExampleOfRFC8291BecomesTheBodyItSays(t *testing.T) {
	ephemeral, err := ecdh.P256().NewPrivateKey(mustDecode(t, rfcASPrivate))
	if err != nil {
		t.Fatal(err)
	}
	got, err := encryptWith([]byte(rfcPlaintext), rfcUAPublic, rfcAuth, mustDecode(t, rfcSalt), ephemeral)
	if err != nil {
		t.Fatal(err)
	}
	if Encode(got) != rfcBody {
		t.Fatalf("body = %s\nwant   %s", Encode(got), rfcBody)
	}
}

// open is the browser's side, written from the receiver's end of the same RFC: it has the private
// key of the subscription and finds the sender's public key in the header of the body.
func open(t *testing.T, body []byte, uaPrivate *ecdh.PrivateKey, auth string) ([]byte, error) {
	t.Helper()
	salt := body[:16]
	if rs := binary.BigEndian.Uint32(body[16:20]); rs != RecordSize {
		t.Fatalf("record size %d", rs)
	}
	idLen := int(body[20])
	asPublicRaw := body[21 : 21+idLen]
	asPublic, err := ecdh.P256().NewPublicKey(asPublicRaw)
	if err != nil {
		return nil, err
	}
	shared, err := uaPrivate.ECDH(asPublic)
	if err != nil {
		return nil, err
	}
	keyInfo := append([]byte("WebPush: info\x00"), uaPrivate.PublicKey().Bytes()...)
	keyInfo = append(keyInfo, asPublicRaw...)
	prkKey, _ := hkdf.Extract(sha256.New, shared, mustDecode(t, auth))
	ikm, _ := hkdf.Expand(sha256.New, prkKey, string(keyInfo), 32)
	prk, _ := hkdf.Extract(sha256.New, ikm, salt)
	cek, _ := hkdf.Expand(sha256.New, prk, "Content-Encoding: aes128gcm\x00", 16)
	nonce, _ := hkdf.Expand(sha256.New, prk, "Content-Encoding: nonce\x00", 12)
	block, _ := aes.NewCipher(cek)
	gcm, _ := cipher.NewGCM(block)
	record, err := gcm.Open(nil, nonce, body[21+idLen:], nil)
	if err != nil {
		return nil, err
	}
	// The last record ends in 0x02 after the data.
	if len(record) == 0 || record[len(record)-1] != 0x02 {
		t.Fatalf("record does not end in the delimiter")
	}
	return record[:len(record)-1], nil
}

func newBrowser(t *testing.T) (private *ecdh.PrivateKey, p256dh, auth string) {
	t.Helper()
	k, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	secret := make([]byte, 16)
	if _, err := rand.Read(secret); err != nil {
		t.Fatal(err)
	}
	return k, Encode(k.PublicKey().Bytes()), Encode(secret)
}

func TestWhatIsEncryptedOpensForTheBrowserItWasSentTo(t *testing.T) {
	private, p256dh, auth := newBrowser(t)
	want := []byte(`{"title":"Plex","body":"Service is down","tag":"monitor-1"}`)
	body, err := Encrypt(want, p256dh, auth)
	if err != nil {
		t.Fatal(err)
	}
	got, err := open(t, body, private, auth)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("opened %q, want %q", got, want)
	}
	// The push service in the middle sees none of it.
	if bytes.Contains(body, []byte("Plex")) || bytes.Contains(body, []byte("down")) {
		t.Fatal("the message is readable in the body")
	}
}

func TestAnotherBrowserOrAnotherSecretCannotOpenIt(t *testing.T) {
	private, p256dh, auth := newBrowser(t)
	body, err := Encrypt([]byte("hello"), p256dh, auth)
	if err != nil {
		t.Fatal(err)
	}
	// The right browser with the right secret opens it: the two refusals below are about what changed.
	if _, err := open(t, body, private, auth); err != nil {
		t.Fatalf("the subscribed browser could not open it: %v", err)
	}
	stranger, _, _ := newBrowser(t)
	if _, err := open(t, body, stranger, auth); err == nil {
		t.Fatal("a browser that is not the one subscribed opened the message")
	}
	if _, err := open(t, body, private, Encode(bytes.Repeat([]byte{9}, 16))); err == nil {
		t.Fatal("the wrong authentication secret opened the message")
	}
}

func TestTheLongestMessageFitsAndOneByteMoreDoesNot(t *testing.T) {
	private, p256dh, auth := newBrowser(t)
	fits := bytes.Repeat([]byte("a"), MaxPayload)
	body, err := Encrypt(fits, p256dh, auth)
	if err != nil {
		t.Fatalf("the longest message was refused: %v", err)
	}
	if len(body) != 16+4+1+65+RecordSize {
		t.Fatalf("body is %d bytes, want the header and one full record", len(body))
	}
	if got, err := open(t, body, private, auth); err != nil || !bytes.Equal(got, fits) {
		t.Fatalf("the longest message did not open: %v", err)
	}
	if _, err := Encrypt(append(fits, 'a'), p256dh, auth); err != ErrTooLong {
		t.Fatalf("one byte more: %v, want ErrTooLong", err)
	}
}

func TestKeysThatAreNotAPointOrASecretAreRefused(t *testing.T) {
	_, p256dh, auth := newBrowser(t)
	for name, c := range map[string][2]string{
		"not base64":        {"%%%", auth},
		"a short key":       {Encode(make([]byte, 33)), auth},
		"a point off P-256": {Encode(append([]byte{4}, make([]byte, 64)...)), auth},
		"a short secret":    {p256dh, Encode(make([]byte, 15))},
		"a long secret":     {p256dh, Encode(make([]byte, 17))},
		"an empty secret":   {p256dh, ""},
	} {
		if err := ValidSubscriptionKeys(c[0], c[1]); err == nil {
			t.Errorf("%s: accepted", name)
		}
		if _, err := Encrypt([]byte("x"), c[0], c[1]); err == nil {
			t.Errorf("%s: encrypted for it", name)
		}
	}
	if err := ValidSubscriptionKeys(p256dh, auth); err != nil {
		t.Fatalf("a good subscription refused: %v", err)
	}
}

func TestTheSigningKeysAreAPairAndTheBrowserGetsThePublicHalf(t *testing.T) {
	private, public, err := GenerateVAPIDKeys()
	if err != nil {
		t.Fatal(err)
	}
	if got := mustDecode(t, public); len(got) != 65 || got[0] != 4 {
		t.Fatalf("public key is %d bytes starting %d, want an uncompressed point of 65", len(got), got[0])
	}
	if len(mustDecode(t, private)) != 32 {
		t.Fatal("private key is not 32 bytes")
	}
	derived, err := PublicFromPrivate(private)
	if err != nil || derived != public {
		t.Fatalf("derived %s (%v), want %s", derived, err, public)
	}
	if _, err := PublicFromPrivate("not a key"); err == nil {
		t.Fatal("a bad private key was accepted")
	}
}

func TestTheAuthorizationIsSignedForThisEndpointAndVerifiesWithThePublicKey(t *testing.T) {
	private, public, _ := GenerateVAPIDKeys()
	now := time.Unix(1_800_000_000, 0)
	header, err := Authorization("https://fcm.googleapis.com/fcm/send/abc", "https://example.org", private, now)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(header, "vapid t=") || !strings.HasSuffix(header, ", k="+public) {
		t.Fatalf("header = %s", header)
	}
	token := strings.TrimSuffix(strings.TrimPrefix(header, "vapid t="), ", k="+public)
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		t.Fatalf("token has %d parts", len(parts))
	}

	var claims struct {
		Aud string `json:"aud"`
		Exp int64  `json:"exp"`
		Sub string `json:"sub"`
	}
	if err := json.Unmarshal(mustDecode(t, parts[1]), &claims); err != nil {
		t.Fatal(err)
	}
	if claims.Aud != "https://fcm.googleapis.com" {
		t.Errorf("aud = %q: the token is for the origin of the endpoint, not its path", claims.Aud)
	}
	if claims.Sub != "https://example.org" {
		t.Errorf("sub = %q", claims.Sub)
	}
	if got := time.Unix(claims.Exp, 0).Sub(now); got != 12*time.Hour {
		t.Errorf("expires in %v, want 12h: the services refuse more than a day", got)
	}

	// The signature is 64 bytes, r then s, and checks against the public key.
	sig := mustDecode(t, parts[2])
	if len(sig) != 64 {
		t.Fatalf("signature is %d bytes, want 64", len(sig))
	}
	pubKey, err := ecdsa.ParseUncompressedPublicKey(elliptic.P256(), mustDecode(t, public))
	if err != nil {
		t.Fatal(err)
	}
	der, _ := asn1.Marshal(struct{ R, S *big.Int }{new(big.Int).SetBytes(sig[:32]), new(big.Int).SetBytes(sig[32:])})
	sum := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if !ecdsa.VerifyASN1(pubKey, sum[:], der) {
		t.Fatal("the signature does not verify with the public key")
	}
	// And it is only for what was signed: another token does not verify under it.
	other := sha256.Sum256([]byte(parts[0] + "." + Encode([]byte(`{"aud":"https://evil.example"}`))))
	if ecdsa.VerifyASN1(pubKey, other[:], der) {
		t.Fatal("the signature verifies for a different token")
	}
}

func TestAnEndpointThatIsNotHTTPSGetsNoAuthorization(t *testing.T) {
	private, _, _ := GenerateVAPIDKeys()
	for _, ep := range []string{"http://fcm.googleapis.com/x", "ftp://a/b", "https://", "::", ""} {
		if _, err := Authorization(ep, "https://example.org", private, time.Now()); err == nil {
			t.Errorf("%q: authorized", ep)
		}
	}
	if _, err := Authorization("https://a.example/x", "https://example.org", "garbage", time.Now()); err == nil {
		t.Error("a bad private key signed")
	}
}
