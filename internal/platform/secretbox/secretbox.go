// Package secretbox reads what an earlier version of this product encrypted.
//
// Nothing here seals anything any more. Secrets live in the database as they
// were given: a key kept on the same machine as the data protects against
// nothing that a password on the backup does not protect better, and it carried
// a failure of its own — a key file is a file, and a file next to the database
// is a file that ends up copied somewhere it should not be.
//
// What is left is the reading half, called once by the startup migration that
// moves sealed values into the plain columns beside them. It cannot make a key:
// a migration that generated one would write a fresh file on every install that
// has none, decrypt nothing with it, and leave behind exactly the thing this
// product stopped wanting to exist.
package secretbox

import (
	"crypto/aes"
	"crypto/cipher"
	"errors"
	"fmt"
	"os"
)

// ErrNoKey means there is no key file, so nothing encrypted can be read.
var ErrNoKey = errors.New("no encryption key")

// Box opens values sealed by an earlier version.
type Box struct {
	aead cipher.AEAD
}

// OpenExisting reads a key that is already there. A missing file comes back as
// os.ErrNotExist, which the caller reports to the operator rather than treating
// as a failure to start: on every install that never encrypted anything, there
// is no key and there is nothing to open.
func OpenExisting(path string) (Box, error) {
	key, err := os.ReadFile(path)
	if err != nil {
		return Box{}, err
	}
	if len(key) != 32 {
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

// Open decrypts. The nonce sits in front of the ciphertext, which is where the
// version that wrote these bytes put it. A value that has been tampered with
// fails rather than decrypting to something else, which is what the
// authentication in AEAD is for.
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
