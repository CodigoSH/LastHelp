package web

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"git.codigosh.com/CodigoSH/LastHelp/internal/platform/database"
)

// sealedLikeTheOldVersion writes a value the way the version that had a key
// wrote it: a random nonce in front of an AES-GCM ciphertext. The migration is
// the only thing left that reads this shape, so the fixture has to be built
// here rather than by calling a sealer that no longer exists.
func sealedLikeTheOldVersion(t *testing.T, key []byte, plain string) []byte {
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

// anUpgradedDatabase is what an installation looks like the moment the new
// binary has migrated the schema and not yet touched the values: the plain
// columns exist and are empty, and the sealed ones still hold everything.
func anUpgradedDatabase(t *testing.T) (*sql.DB, string, []byte) {
	t.Helper()

	dir := t.TempDir()
	db, err := database.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatal(err)
	}
	keyFile := filepath.Join(dir, "lasthelp.key")
	if err := os.WriteFile(keyFile, key, 0o600); err != nil {
		t.Fatal(err)
	}
	return db, keyFile, key
}

func sealedOrg(t *testing.T, db *sql.DB, key []byte, token, secret string) int64 {
	t.Helper()
	res, err := db.Exec(`
		INSERT INTO organization (slug, name, forge_kind, base_url, forge_owner, token_enc, webhook_secret_enc, created_at)
		VALUES ('codigosh', 'CodigoSH', 'forgejo', 'https://forge.example', 'CodigoSH', ?, ?, 0)`,
		sealedLikeTheOldVersion(t, key, token), sealedLikeTheOldVersion(t, key, secret))
	if err != nil {
		t.Fatal(err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatal(err)
	}
	return id
}

// sealedItem is a report whose reporter asked to be told when it closes, with
// the address sealed the way the old version sealed it.
func sealedItem(t *testing.T, db *sql.DB, orgID int64, email []byte) int64 {
	t.Helper()

	repo, err := db.Exec(`
		INSERT INTO repository (org_id, slug, name, forge_repo, created_at)
		VALUES (?, 'lastboard', 'Lastboard', 'CodigoSH/Lastboard', 0)`, orgID)
	if err != nil {
		t.Fatal(err)
	}
	repoID, err := repo.LastInsertId()
	if err != nil {
		t.Fatal(err)
	}

	res, err := db.Exec(`
		INSERT INTO item (public_id, repo_id, kind, title, body, status, author_email_enc, follow_token_hash, created_at)
		VALUES ('abc', ?, 'bug', 'It broke', 'Here is how', 'open', ?, 'h', 0)`, repoID, email)
	if err != nil {
		t.Fatal(err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatal(err)
	}
	return id
}

// TestUpgradingMovesEverySealedValue covers all three columns, and the reason
// it does is the third one: the forge token and the webhook secret are visible
// the moment a bridge stops working, but a reporter's address failing to move
// is silent — nobody is told about their report and nothing says why.
func TestUpgradingMovesEverySealedValue(t *testing.T) {
	db, keyFile, key := anUpgradedDatabase(t)
	orgID := sealedOrg(t, db, key, "ghp_a_forge_token", "a-webhook-secret")

	itemID := sealedItem(t, db, orgID, sealedLikeTheOldVersion(t, key, "reporter@example.org"))

	if err := unsealOnce(db, keyFile); err != nil {
		t.Fatal(err)
	}

	for _, want := range []struct {
		query    string
		id       int64
		expected string
	}{
		{"SELECT token FROM organization WHERE id = ?", orgID, "ghp_a_forge_token"},
		{"SELECT webhook_secret FROM organization WHERE id = ?", orgID, "a-webhook-secret"},
		{"SELECT author_email FROM item WHERE id = ?", itemID, "reporter@example.org"},
	} {
		var got string
		if err := db.QueryRow(want.query, want.id).Scan(&got); err != nil {
			t.Fatal(err)
		}
		if got != want.expected {
			t.Errorf("%s came back as %q, want %q", want.query, got, want.expected)
		}
	}
}

// TestARowThatStaysSealedDoesNotUndoWhatTheOperatorTyped. This runs on every
// start, not once, and normally the second start has nothing to do: every value
// moved, so the count comes back zero and the pass returns before reading
// anything.
//
// The row that keeps it honest is the one that never finishes. A value sealed
// with a key that is gone counts as pending forever, so this migration reads
// that organisation on every start for the rest of its life — and the token
// beside it is one the operator pasted in the panel afterwards. Without the
// clause that skips a plain column already filled in, every restart would put
// the dead token back and the bridge would break again each time.
func TestARowThatStaysSealedDoesNotUndoWhatTheOperatorTyped(t *testing.T) {
	db, keyFile, key := anUpgradedDatabase(t)
	orgID := sealedOrg(t, db, key, "ghp_the_old_token", "a-webhook-secret")

	// The webhook secret was sealed with a key this installation no longer
	// has, which is what keeps the row pending.
	lost := make([]byte, 32)
	if _, err := rand.Read(lost); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("UPDATE organization SET webhook_secret_enc = ? WHERE id = ?",
		sealedLikeTheOldVersion(t, lost, "gone"), orgID); err != nil {
		t.Fatal(err)
	}

	if err := unsealOnce(db, keyFile); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("UPDATE organization SET token = ? WHERE id = ?", "ghp_pasted_in_the_panel", orgID); err != nil {
		t.Fatal(err)
	}
	if err := unsealOnce(db, keyFile); err != nil {
		t.Fatal(err)
	}

	var got string
	if err := db.QueryRow("SELECT token FROM organization WHERE id = ?", orgID).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != "ghp_pasted_in_the_panel" {
		t.Errorf("a later start put the old token back: %q", got)
	}
}

// TestUpgradingWithNoKeyStartsAnyway is the installation whose key was lost, or
// that is being restored on another machine. The values cannot be read by
// anybody, including us; that is a thing to say once in the log, not a reason
// to refuse to start.
func TestUpgradingWithNoKeyStartsAnyway(t *testing.T) {
	db, keyFile, key := anUpgradedDatabase(t)
	sealedOrg(t, db, key, "ghp_a_forge_token", "a-webhook-secret")
	if err := os.Remove(keyFile); err != nil {
		t.Fatal(err)
	}

	if err := unsealOnce(db, keyFile); err != nil {
		t.Fatalf("a missing key stopped the server: %v", err)
	}
	if _, err := os.Stat(keyFile); !os.IsNotExist(err) {
		t.Fatal("a key file was created by the migration")
	}
}

// TestOneUnreadableValueDoesNotStopTheOthers: a value sealed with a key that
// was since replaced is gone, and gone is something the operator fixes by
// pasting it again — it must not cost them the values that are still readable.
func TestOneUnreadableValueDoesNotStopTheOthers(t *testing.T) {
	db, keyFile, key := anUpgradedDatabase(t)
	orgID := sealedOrg(t, db, key, "ghp_a_forge_token", "a-webhook-secret")

	other := make([]byte, 32)
	if _, err := rand.Read(other); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("UPDATE organization SET webhook_secret_enc = ? WHERE id = ?",
		sealedLikeTheOldVersion(t, other, "sealed-with-a-key-that-is-gone"), orgID); err != nil {
		t.Fatal(err)
	}

	if err := unsealOnce(db, keyFile); err != nil {
		t.Fatal(err)
	}

	var token, secret string
	if err := db.QueryRow("SELECT token, webhook_secret FROM organization WHERE id = ?", orgID).
		Scan(&token, &secret); err != nil {
		t.Fatal(err)
	}
	if token != "ghp_a_forge_token" {
		t.Errorf("the readable value was lost too: %q", token)
	}
	if secret != "" {
		t.Errorf("an unreadable value was written as %q", secret)
	}
}
