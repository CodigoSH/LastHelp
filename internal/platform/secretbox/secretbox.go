// Package secretbox encrypts the few things that must not be readable in a
// database backup.
//
// Which is a short list: the forge token and the webhook secret. Everything
// else in this database is either public already or a hash.
//
// The key does not live in the database. That is the whole point: a backup that
// leaked would otherwise carry both the ciphertext and the key to it, which is
// the same as storing the token in plain text with extra steps.
package secretbox

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// ErrNoKey means there is no key file, so nothing encrypted can be read.
var ErrNoKey = errors.New("no encryption key")

// Box seals and opens.
type Box struct {
	aead cipher.AEAD
}

// Open loads the key beside the database, creating it on first use.
//
// 0600 and a separate file. An operator who copies the database file — which is
// what every backup script does — does not copy this, and a leaked backup is
// then a leak of hashes and public text rather than of write access to
// somebody's forge.
func Open(path string) (Box, error) {
	key, err := os.ReadFile(path)
	switch {
	case errors.Is(err, os.ErrNotExist):
		key = make([]byte, 32)
		if _, err := rand.Read(key); err != nil {
			return Box{}, fmt.Errorf("generating the encryption key: %w", err)
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			return Box{}, fmt.Errorf("creating the key directory: %w", err)
		}
		if err := os.WriteFile(path, key, 0o600); err != nil {
			return Box{}, fmt.Errorf("writing the encryption key: %w", err)
		}
	case err != nil:
		return Box{}, fmt.Errorf("reading the encryption key: %w", err)
	case len(key) != 32:
		return Box{}, fmt.Errorf("the key file is %d bytes, want 32", len(key))
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return Box{}, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return Box{}, err
	}
	return Box{aead: aead}, nil
}

// Seal encrypts. The nonce is random per call and stored in front of the
// ciphertext: reusing one with GCM is catastrophic, and the only safe way to
// never reuse one is to never choose one.
func (b Box) Seal(plain string) ([]byte, error) {
	if b.aead == nil {
		return nil, ErrNoKey
	}
	nonce := make([]byte, b.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("generating a nonce: %w", err)
	}
	return b.aead.Seal(nonce, nonce, []byte(plain), nil), nil
}

// Openes decrypts. A value that has been tampered with fails rather than
// decrypting to something else, which is what the authentication in AEAD is for.
func (b Box) Open(sealed []byte) (string, error) {
	if b.aead == nil {
		return "", ErrNoKey
	}
	if len(sealed) < b.aead.NonceSize() {
		return "", errors.New("the stored value is too short to be encrypted")
	}
	nonce, ciphertext := sealed[:b.aead.NonceSize()], sealed[b.aead.NonceSize():]
	plain, err := b.aead.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", fmt.Errorf("this value was not encrypted with this key: %w", err)
	}
	return string(plain), nil
}
