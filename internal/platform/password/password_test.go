package password

import (
	"encoding/base64"
	"errors"
	"strings"
	"testing"

	"golang.org/x/crypto/argon2"
)

func TestARoundTrip(t *testing.T) {
	hash, err := Hash("correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	if err := Verify("correct horse battery staple", hash); err != nil {
		t.Fatalf("the right password did not verify: %v", err)
	}
	if err := Verify("wrong horse battery staple", hash); !errors.Is(err, ErrMismatch) {
		t.Fatalf("the wrong password gave %v, want ErrMismatch", err)
	}
}

// TestTheSamePasswordHashesDifferently is the salt, asserted. Without it, two
// people who chose the same password are visibly the same in the database.
func TestTheSamePasswordHashesDifferently(t *testing.T) {
	a, err := Hash("same")
	if err != nil {
		t.Fatal(err)
	}
	b, err := Hash("same")
	if err != nil {
		t.Fatal(err)
	}
	if a == b {
		t.Error("two hashes of the same password are identical: the salt is not being used")
	}
}

// TestALongPassphraseIsNotTruncated is why this is not bcrypt.
//
// bcrypt stops at 72 bytes, so two passphrases that share a long prefix verify
// against each other's hash and nobody is told.
func TestALongPassphraseIsNotTruncated(t *testing.T) {
	prefix := strings.Repeat("a", 72)
	hash, err := Hash(prefix + "ending one")
	if err != nil {
		t.Fatal(err)
	}
	if err := Verify(prefix+"ending two", hash); !errors.Is(err, ErrMismatch) {
		t.Error("two passphrases differing after 72 bytes verified against the same hash")
	}
}

// TestAMalformedHashDoesNotPanicAndDoesNotPass.
//
// A truncated column, a hash from a different tool, an empty string in a row
// somebody inserted by hand: none of them may be a way in, and none of them may
// take the server down either.
func TestAMalformedHashDoesNotPanicAndDoesNotPass(t *testing.T) {
	for _, bad := range []string{
		"",
		"$argon2id$",
		"$2a$10$abcdefghijklmnopqrstuv", // a bcrypt hash
		"$argon2id$v=19$m=x,t=y,p=z$AAAA$BBBB",
		"$argon2id$v=19$m=65536,t=3,p=2$not-base64!$also-not",
		strings.Repeat("$", 6),
	} {
		if err := Verify("anything", bad); err == nil {
			t.Errorf("the malformed hash %q accepted a password", bad)
		}
	}
}

// TestParametersComeFromTheStoredHash.
//
// The constants above will be raised one day. Everything stored before that
// must keep verifying, or raising them locks the whole team out at once.
func TestParametersComeFromTheStoredHash(t *testing.T) {
	// A hash made with deliberately weaker parameters than the current
	// constants, as an old one would be.
	old := "$argon2id$v=19$m=8192,t=1,p=1$" +
		"c2FsdHNhbHRzYWx0c2FsdA" + "$"
	// Build the digest for those parameters the same way Hash would.
	weak, err := hashWith("legacy", []byte("saltsaltsaltsalt"), 1, 8192, 1)
	if err != nil {
		t.Fatal(err)
	}
	old += weak

	if err := Verify("legacy", old); err != nil {
		t.Errorf("a hash written with older parameters stopped verifying: %v", err)
	}
}

// hashWith is the test's own encoder for parameters other than the current
// ones, so the test above does not need a hash pasted in from somewhere.
func hashWith(plain string, salt []byte, time, memory uint32, threads uint8) (string, error) {
	key := argon2.IDKey([]byte(plain), salt, time, memory, threads, keyLength)
	return base64.RawStdEncoding.EncodeToString(key), nil
}
