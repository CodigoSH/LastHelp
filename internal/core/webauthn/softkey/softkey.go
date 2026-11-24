// Package softkey is an authenticator in software, for tests: it makes passkeys
// and signs with them exactly as a security key or a phone does, so the server's
// checks can be driven end to end without a person and a device. Nothing in the
// server imports it.
package softkey

import (
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"sort"
)

// The flags an authenticator sets.
const (
	UserPresent  = 0x01
	UserVerified = 0x04
	attested     = 0x40
)

// Key is one passkey in software.
type Key struct {
	Alg          int64 // -7 (ES256) or -8 (EdDSA)
	CredentialID []byte
	UserHandle   []byte
	Count        uint32 // the signature counter; 0 means it does not count
	ec           *ecdsa.PrivateKey
	ed           ed25519.PrivateKey
}

// New makes a passkey of one kind.
func New(alg int64) *Key {
	k := &Key{Alg: alg, CredentialID: make([]byte, 32)}
	rand.Read(k.CredentialID)
	if alg == -8 {
		_, k.ed, _ = ed25519.GenerateKey(rand.Reader)
	} else {
		k.ec, _ = ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	}
	return k
}

// ClientData is clientDataJSON as a browser writes it.
func ClientData(typ string, challenge []byte, origin string) []byte {
	b, _ := json.Marshal(map[string]any{
		"type":        typ,
		"challenge":   base64.RawURLEncoding.EncodeToString(challenge),
		"origin":      origin,
		"crossOrigin": false,
	})
	return b
}

// COSE is the passkey's public key as a COSE key.
func (k *Key) COSE() []byte {
	if k.Alg == -8 {
		return encode(map[int64]any{1: int64(1), 3: int64(-8), -1: int64(6), -2: []byte(k.ed.Public().(ed25519.PublicKey))})
	}
	point, _ := k.ec.PublicKey.Bytes() // 0x04 || x || y
	x, y := point[1:33], point[33:65]
	return encode(map[int64]any{1: int64(2), 3: int64(-7), -1: int64(1), -2: x, -3: y})
}

func authData(rpID string, flags byte, count uint32, attestedData []byte) []byte {
	h := sha256.Sum256([]byte(rpID))
	out := append([]byte(nil), h[:]...)
	out = append(out, flags)
	out = binary.BigEndian.AppendUint32(out, count)
	return append(out, attestedData...)
}

// Create answers navigator.credentials.create: the attestation object (format
// "none") for this passkey on rpID.
func (k *Key) Create(rpID string, flags byte) []byte {
	att := make([]byte, 16) // AAGUID: zero, as authenticators that stay anonymous send
	att = binary.BigEndian.AppendUint16(att, uint16(len(k.CredentialID)))
	att = append(att, k.CredentialID...)
	att = append(att, k.COSE()...)
	ad := authData(rpID, flags|attested, k.Count, att)
	return encodeAny(map[string]any{"fmt": "none", "attStmt": map[string]any{}, "authData": ad})
}

// Get answers navigator.credentials.get: authenticator data and the signature
// over it and the hash of clientDataJSON. The counter goes up first, as an
// authenticator that counts does.
func (k *Key) Get(rpID string, flags byte, clientDataJSON []byte) (authenticatorData, signature []byte) {
	if k.Count > 0 {
		k.Count++
	}
	ad := authData(rpID, flags, k.Count, nil)
	return ad, k.Sign(ad, clientDataJSON)
}

// Sign signs authenticator data and clientDataJSON as the passkey does.
func (k *Key) Sign(ad, clientDataJSON []byte) []byte {
	sum := sha256.Sum256(clientDataJSON)
	msg := append(append([]byte(nil), ad...), sum[:]...)
	if k.Alg == -8 {
		return ed25519.Sign(k.ed, msg)
	}
	digest := sha256.Sum256(msg)
	sig, _ := ecdsa.SignASN1(rand.Reader, k.ec, digest[:])
	return sig
}

/* A CBOR writer for what an authenticator writes: maps, integers, strings. */

func head(major byte, n uint64) []byte {
	switch {
	case n < 24:
		return []byte{major<<5 | byte(n)}
	case n < 1<<8:
		return []byte{major<<5 | 24, byte(n)}
	case n < 1<<16:
		return binary.BigEndian.AppendUint16([]byte{major<<5 | 25}, uint16(n))
	case n < 1<<32:
		return binary.BigEndian.AppendUint32([]byte{major<<5 | 26}, uint32(n))
	}
	return binary.BigEndian.AppendUint64([]byte{major<<5 | 27}, n)
}

func encodeAny(v any) []byte {
	switch x := v.(type) {
	case int64:
		if x >= 0 {
			return head(0, uint64(x))
		}
		return head(1, uint64(-1-x))
	case []byte:
		return append(head(2, uint64(len(x))), x...)
	case string:
		return append(head(3, uint64(len(x))), x...)
	case map[string]any:
		keys := make([]string, 0, len(x))
		for k := range x {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		out := head(5, uint64(len(x)))
		for _, k := range keys {
			out = append(out, encodeAny(k)...)
			out = append(out, encodeAny(x[k])...)
		}
		return out
	case map[int64]any:
		return encode(x)
	}
	panic("softkey: cannot encode")
}

func encode(m map[int64]any) []byte {
	keys := make([]int64, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
	out := head(5, uint64(len(m)))
	for _, k := range keys {
		out = append(out, encodeAny(k)...)
		out = append(out, encodeAny(m[k])...)
	}
	return out
}
