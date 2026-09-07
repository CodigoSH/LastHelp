// Package password hashes and verifies the team's passwords.
//
// Argon2id, not bcrypt: it is the current recommendation, it is memory-hard —
// which is what makes a GPU farm expensive rather than merely busy — and bcrypt
// silently truncates at 72 bytes, so a long passphrase is not as strong as the
// person typing it believes.
//
// The public has no accounts at all. This exists for the handful of people who
// run the instance, which is why the parameters can be generous.
package password

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

// ErrMismatch is returned when the password is wrong. It is deliberately the
// same error whatever went wrong below, so that nothing about the stored hash
// leaks through the difference.
var ErrMismatch = errors.New("password does not match")

// The cost. Roughly 64 MiB and a few tens of milliseconds on a modern machine,
// which is the point: it has to be slow enough to matter to somebody trying
// millions of guesses, and quick enough that signing in feels instant.
//
// These live in the encoded hash, so raising them later does not invalidate
// what is already stored — an old hash keeps verifying with the parameters it
// was made with.
const (
	timeCost    = 3
	memoryCost  = 64 * 1024 // KiB
	parallelism = 2
	saltLength  = 16
	keyLength   = 32
)

// Hash returns a PHC-format string: the algorithm, its parameters, the salt and
// the digest, all in one column. Storing the parameters alongside is what makes
// them changeable.
func Hash(plain string) (string, error) {
	salt := make([]byte, saltLength)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("generating a salt: %w", err)
	}

	key := argon2.IDKey([]byte(plain), salt, timeCost, memoryCost, parallelism, keyLength)

	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, memoryCost, timeCost, parallelism,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(key),
	), nil
}

// Verify checks a password against a stored hash.
//
// The comparison is constant-time. The parameters come from the stored string
// rather than from the constants above, so a hash written before they changed
// still verifies.
func Verify(plain, encoded string) error {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" {
		return ErrMismatch
	}

	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil || version != argon2.Version {
		return ErrMismatch
	}

	var memory uint32
	var time uint32
	var threads uint8
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &memory, &time, &threads); err != nil {
		return ErrMismatch
	}

	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return ErrMismatch
	}
	want, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return ErrMismatch
	}

	got := argon2.IDKey([]byte(plain), salt, time, memory, threads, uint32(len(want)))
	if subtle.ConstantTimeCompare(got, want) != 1 {
		return ErrMismatch
	}
	return nil
}
