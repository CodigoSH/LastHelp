package secretbox

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func box(t *testing.T) (Box, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "key")
	b, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	return b, path
}

func TestARoundTrip(t *testing.T) {
	b, _ := box(t)

	sealed, err := b.Seal("ghp_a_forge_token")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(sealed), "ghp_") {
		t.Fatal("the token is readable in the stored bytes")
	}

	plain, err := b.Open(sealed)
	if err != nil {
		t.Fatal(err)
	}
	if plain != "ghp_a_forge_token" {
		t.Errorf("came back as %q", plain)
	}
}

// TestTheSameValueSealsDifferentlyEveryTime.
//
// A random nonce per call. Reusing one with GCM is catastrophic, and two
// organisations with the same token must not be visibly the same in a backup.
func TestTheSameValueSealsDifferentlyEveryTime(t *testing.T) {
	b, _ := box(t)

	first, err := b.Seal("same")
	if err != nil {
		t.Fatal(err)
	}
	second, err := b.Seal("same")
	if err != nil {
		t.Fatal(err)
	}
	if string(first) == string(second) {
		t.Error("two seals of the same value are identical: the nonce is not random")
	}
}

// TestTamperingIsDetected. AEAD, not just encryption: a changed byte has to
// fail rather than decrypt to something else.
func TestTamperingIsDetected(t *testing.T) {
	b, _ := box(t)
	sealed, err := b.Seal("a token")
	if err != nil {
		t.Fatal(err)
	}
	sealed[len(sealed)-1] ^= 0xff

	if _, err := b.Open(sealed); err == nil {
		t.Error("a tampered value decrypted")
	}
}

// TestAnotherKeyCannotRead is what makes the separate file worth having.
func TestAnotherKeyCannotRead(t *testing.T) {
	first, _ := box(t)
	second, _ := box(t)

	sealed, err := first.Seal("a token")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := second.Open(sealed); err == nil {
		t.Error("a different key opened it")
	}
}

// TestTheKeyFileIsNotWorldReadable.
//
// It sits next to the database, and the database is what backup scripts copy.
// A key anybody on the machine can read is a key that travels with it.
func TestTheKeyFileIsNotWorldReadable(t *testing.T) {
	_, path := box(t)

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if mode := info.Mode().Perm(); mode != 0o600 {
		t.Errorf("the key file is %o, want 600", mode)
	}
}

func TestTheKeyIsKeptBetweenStarts(t *testing.T) {
	b, path := box(t)
	sealed, err := b.Seal("a token")
	if err != nil {
		t.Fatal(err)
	}

	again, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	plain, err := again.Open(sealed)
	if err != nil {
		t.Fatalf("restarting could not read what it wrote: %v", err)
	}
	if plain != "a token" {
		t.Errorf("came back as %q", plain)
	}
}

func TestAKeyFileOfTheWrongSizeIsRefused(t *testing.T) {
	path := filepath.Join(t.TempDir(), "key")
	if err := os.WriteFile(path, []byte("too short"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(path); err == nil {
		t.Error("a truncated key file was accepted; everything sealed with it would be unreadable later")
	}
}
