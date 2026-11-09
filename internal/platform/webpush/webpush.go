// Package webpush is the sender's half of Web Push, written against the standard library alone.
//
// A push service (Google's, Apple's, Mozilla's, Microsoft's) hands a message to a browser that
// may have no page open. Three RFCs say how: 8030 (the request), 8291 (the message is encrypted
// for one browser, so the service in the middle reads nothing) and 8292 (VAPID, the server signs
// with a key whose public half the browser subscribed to, so only this server can send to it).
//
// Nothing here touches the network or the database: it builds the bytes and the header. Sending
// them, and who may be sent to, belong to the caller.
package webpush

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"encoding/asn1"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/url"
	"time"
)

// RecordSize is the size of the one record a message is sent in. 4096 is what the push services
// accept; a payload that does not fit is refused rather than split.
const RecordSize = 4096

// MaxPayload is the longest plaintext one record holds: the record, less the tag of AES-GCM (16)
// and the delimiter byte that ends it (1).
const MaxPayload = RecordSize - 16 - 1

// ErrTooLong says the message does not fit in a record.
var ErrTooLong = errors.New("webpush: payload does not fit in one record")

var b64 = base64.RawURLEncoding

// Encode and Decode are the base64url without padding every Web Push value travels in.
func Encode(b []byte) string { return b64.EncodeToString(b) }

func Decode(s string) ([]byte, error) { return b64.DecodeString(s) }

// GenerateVAPIDKeys makes a P-256 key pair for signing, as the two strings a server stores and a
// browser is given: the private scalar (32 bytes) and the public point (65 bytes, uncompressed).
func GenerateVAPIDKeys() (private, public string, err error) {
	k, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		return "", "", err
	}
	return Encode(k.Bytes()), Encode(k.PublicKey().Bytes()), nil
}

// PublicFromPrivate derives the public half a browser subscribes with.
func PublicFromPrivate(private string) (string, error) {
	raw, err := Decode(private)
	if err != nil {
		return "", fmt.Errorf("webpush: private key: %w", err)
	}
	k, err := ecdh.P256().NewPrivateKey(raw)
	if err != nil {
		return "", fmt.Errorf("webpush: private key: %w", err)
	}
	return Encode(k.PublicKey().Bytes()), nil
}

// ValidSubscriptionKeys says whether the two values a browser gives with a subscription are what
// they claim: its public key a point on P-256, and its authentication secret 16 bytes.
func ValidSubscriptionKeys(p256dh, auth string) error {
	pub, err := Decode(p256dh)
	if err != nil {
		return errors.New("webpush: p256dh is not base64url")
	}
	if _, err := ecdh.P256().NewPublicKey(pub); err != nil {
		return errors.New("webpush: p256dh is not a P-256 point")
	}
	secret, err := Decode(auth)
	if err != nil || len(secret) != 16 {
		return errors.New("webpush: auth is not 16 bytes of base64url")
	}
	return nil
}

// Authorization is the header that says who is sending (RFC 8292): a short-lived signed token for
// the origin of the endpoint, and the public key it was signed for.
func Authorization(endpoint, subject, private string, now time.Time) (string, error) {
	u, err := url.Parse(endpoint)
	if err != nil || u.Scheme != "https" || u.Host == "" {
		return "", errors.New("webpush: endpoint is not an https address")
	}
	raw, err := Decode(private)
	if err != nil {
		return "", fmt.Errorf("webpush: private key: %w", err)
	}
	signer, err := ecdsa.ParseRawPrivateKey(elliptic.P256(), raw)
	if err != nil {
		return "", fmt.Errorf("webpush: private key: %w", err)
	}
	pub, err := PublicFromPrivate(private)
	if err != nil {
		return "", err
	}

	header, _ := json.Marshal(map[string]string{"typ": "JWT", "alg": "ES256"})
	claims, _ := json.Marshal(map[string]any{
		"aud": u.Scheme + "://" + u.Host,
		// The services refuse more than 24 hours; twelve leaves room for a clock that is off.
		"exp": now.Add(12 * time.Hour).Unix(),
		"sub": subject,
	})
	signing := Encode(header) + "." + Encode(claims)
	sum := sha256.Sum256([]byte(signing))
	der, err := ecdsa.SignASN1(rand.Reader, signer, sum[:])
	if err != nil {
		return "", err
	}
	sig, err := rawSignature(der)
	if err != nil {
		return "", err
	}
	return "vapid t=" + signing + "." + Encode(sig) + ", k=" + pub, nil
}

// rawSignature turns the DER signature Go makes into the fixed 64 bytes (r then s) JWT wants.
func rawSignature(der []byte) ([]byte, error) {
	var sig struct{ R, S *big.Int }
	if rest, err := asn1.Unmarshal(der, &sig); err != nil || len(rest) != 0 {
		return nil, errors.New("webpush: malformed signature")
	}
	out := make([]byte, 64)
	sig.R.FillBytes(out[:32])
	sig.S.FillBytes(out[32:])
	return out, nil
}

// Encrypt makes the body of a push request: the message encrypted for the one browser that holds
// the private half of p256dh and knows the auth secret (RFC 8291, content coding aes128gcm).
func Encrypt(plaintext []byte, p256dh, auth string) ([]byte, error) {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return nil, err
	}
	ephemeral, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	return encryptWith(plaintext, p256dh, auth, salt, ephemeral)
}

// encryptWith is Encrypt with the two random inputs given, so the RFC's own example can be checked.
func encryptWith(plaintext []byte, p256dh, auth string, salt []byte, ephemeral *ecdh.PrivateKey) ([]byte, error) {
	if len(plaintext) > MaxPayload {
		return nil, ErrTooLong
	}
	uaPublicRaw, err := Decode(p256dh)
	if err != nil {
		return nil, errors.New("webpush: p256dh is not base64url")
	}
	uaPublic, err := ecdh.P256().NewPublicKey(uaPublicRaw)
	if err != nil {
		return nil, errors.New("webpush: p256dh is not a P-256 point")
	}
	authSecret, err := Decode(auth)
	if err != nil || len(authSecret) != 16 {
		return nil, errors.New("webpush: auth is not 16 bytes of base64url")
	}

	shared, err := ephemeral.ECDH(uaPublic)
	if err != nil {
		return nil, err
	}
	asPublic := ephemeral.PublicKey().Bytes()

	// The key the two ends share, mixed with what only this subscription knows (RFC 8291 §3.4).
	keyInfo := append([]byte("WebPush: info\x00"), uaPublicRaw...)
	keyInfo = append(keyInfo, asPublic...)
	prkKey, err := hkdf.Extract(sha256.New, shared, authSecret)
	if err != nil {
		return nil, err
	}
	ikm, err := hkdf.Expand(sha256.New, prkKey, string(keyInfo), 32)
	if err != nil {
		return nil, err
	}

	// The content encryption key and the nonce of the one record (RFC 8188 §2.2, §2.3).
	prk, err := hkdf.Extract(sha256.New, ikm, salt)
	if err != nil {
		return nil, err
	}
	cek, err := hkdf.Expand(sha256.New, prk, "Content-Encoding: aes128gcm\x00", 16)
	if err != nil {
		return nil, err
	}
	nonce, err := hkdf.Expand(sha256.New, prk, "Content-Encoding: nonce\x00", 12)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(cek)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	// The data, then 0x02: the delimiter that says this is the last record.
	record := append(append([]byte{}, plaintext...), 0x02)
	sealed := gcm.Seal(nil, nonce, record, nil)

	// The header: salt, record size, the length of the key id, and the key id, which here is the
	// sender's public key so the browser can derive the same secret.
	out := make([]byte, 0, 16+4+1+len(asPublic)+len(sealed))
	out = append(out, salt...)
	out = binary.BigEndian.AppendUint32(out, RecordSize)
	out = append(out, byte(len(asPublic)))
	out = append(out, asPublic...)
	return append(out, sealed...), nil
}
