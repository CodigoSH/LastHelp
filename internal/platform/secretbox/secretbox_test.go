package secretbox

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// seal writes a value the way the retired version wrote it: a random nonce in
// front of an AES-GCM ciphertext. It lives here, in the test, because nothing
// in the product seals any more and a sealing function kept in the package
// would be code with no callers pretending to be a feature.
//
// What it does NOT prove on its own is that this is really the format on disk
// in somebody's installation — a wrong idea of the format written twice agrees
// with itself. That was settled the other way, once: a database built by
// v1.0.0-Beta.03 was opened by this code and the token came back out whole.
// This test is here so that reading stops working loudly if anybody tidies the
// reader afterwards.
func seal(t *testing.T, key []byte, plain string) []byte {
	t.Helper()
	block, err := aes.NewCipher(key)
	if err != nil {
		t.Fatal(err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		t.Fatal(err)
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		t.Fatal(err)
	}
	return aead.Seal(nonce, nonce, []byte(plain), nil)
}

func keyFile(t *testing.T) (string, []byte) {
	t.Helper()
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "lasthelp.key")
	if err := os.WriteFile(path, key, 0o600); err != nil {
		t.Fatal(err)
	}
	return path, key
}

func TestASealedValueComesBackOut(t *testing.T) {
	path, key := keyFile(t)
	sealed := seal(t, key, "ghp_a_forge_token")
	if strings.Contains(string(sealed), "ghp_") {
		t.Fatal("the fixture is not encrypted, so this test proves nothing")
	}

	b, err := OpenExisting(path)
	if err != nil {
		t.Fatal(err)
	}
	plain, err := b.Open(sealed)
	if err != nil {
		t.Fatal(err)
	}
	if plain != "ghp_a_forge_token" {
		t.Errorf("came back as %q", plain)
	}
}

// TestAMissingKeyIsNotMadeUp is the one that matters now. Open used to create a
// key when the file was gone, which is right for a product that seals and wrong
// for a migration: every install that never encrypted anything would get a
// fresh key file written into its data directory on upgrade, decrypting
// nothing, and the file this product stopped wanting would be back.
func TestAMissingKeyIsNotMadeUp(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "lasthelp.key")

	if _, err := OpenExisting(path); !os.IsNotExist(err) {
		t.Fatalf("want a not-exist error, got %v", err)
	}
	if entries, err := os.ReadDir(dir); err != nil || len(entries) != 0 {
		t.Fatalf("something was written: %v, %v", entries, err)
	}
}

// TestTamperingIsDetected. AEAD, not just encryption: a changed byte has to
// fail rather than decrypt to something else.
func TestTamperingIsDetected(t *testing.T) {
	path, key := keyFile(t)
	sealed := seal(t, key, "ghp_a_forge_token")
	sealed[len(sealed)-1] ^= 0xff

	b, err := OpenExisting(path)
	if err != nil {
		t.Fatal(err)
	}
	if plain, err := b.Open(sealed); err == nil {
		t.Fatalf("a tampered value opened as %q", plain)
	}
}

// TestAnotherKeyCannotRead is what makes the migration report a row as
// unreadable instead of silently writing rubbish into the plain column.
func TestAnotherKeyCannotRead(t *testing.T) {
	_, key := keyFile(t)
	sealed := seal(t, key, "ghp_a_forge_token")

	other, _ := keyFile(t)
	b, err := OpenExisting(other)
	if err != nil {
		t.Fatal(err)
	}
	if plain, err := b.Open(sealed); err == nil {
		t.Fatalf("another key read it as %q", plain)
	}
}

func TestAKeyFileOfTheWrongSizeIsRefused(t *testing.T) {
	path := filepath.Join(t.TempDir(), "lasthelp.key")
	if err := os.WriteFile(path, []byte("too short"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenExisting(path); err == nil {
		t.Fatal("a nine-byte key was accepted")
	}
}

// TestAValueTooShortToBeSealedIsRefused: a column holding something that is not
// a sealed value at all must not be read as an empty string and moved.
func TestAValueTooShortToBeSealedIsRefused(t *testing.T) {
	path, _ := keyFile(t)
	b, err := OpenExisting(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := b.Open([]byte("short")); err == nil {
		t.Fatal("five bytes were accepted as a sealed value")
	}
}
