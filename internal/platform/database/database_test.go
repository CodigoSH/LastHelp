package database

import (
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
)

func open(t *testing.T) *sql.DB {
	t.Helper()
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("opening the database: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestAFreshDatabaseIsAtTheLatestVersion(t *testing.T) {
	db := open(t)

	got, err := userVersion(db)
	if err != nil {
		t.Fatal(err)
	}
	want := migrations[len(migrations)-1].version
	if got != want {
		t.Errorf("schema version is %d, want %d", got, want)
	}
}

func TestMigratingTwiceChangesNothing(t *testing.T) {
	db := open(t)

	// Open already migrated. Running it again is what happens on every restart,
	// and it has to be a no-op rather than an error or a second application.
	if err := Migrate(db); err != nil {
		t.Fatalf("second migration: %v", err)
	}
}

// TestItRefusesToRunOnANewerDatabase.
//
// With a beta channel and an updater, somebody eventually runs an older binary
// against a database a newer one wrote. The failure without this guard is not a
// crash — it is an old binary writing rows that ignore columns it does not know
// about, and nobody noticing until the new one comes back.
func TestItRefusesToRunOnANewerDatabase(t *testing.T) {
	db := open(t)

	future := migrations[len(migrations)-1].version + 5
	if _, err := db.Exec("PRAGMA user_version = " + itoa(future)); err != nil {
		t.Fatal(err)
	}

	err := Migrate(db)
	if err == nil {
		t.Fatal("migrating onto a newer schema succeeded; it must refuse")
	}
	if !strings.Contains(err.Error(), "newer version") {
		t.Errorf("the error does not say what is wrong, which is the whole point:\n%v", err)
	}
}

// TestForeignKeysAreOn is the test for a bug this house has already had.
//
// A pragma run as a statement after opening applies to whichever pooled
// connection happened to execute it, so referential integrity would hold for
// some queries and not others. Putting it in the DSN is what makes it apply to
// every connection — and this is what proves it did, rather than trusting that
// the string was spelled correctly.
func TestForeignKeysAreOn(t *testing.T) {
	db := open(t)

	var on int
	if err := db.QueryRow("PRAGMA foreign_keys").Scan(&on); err != nil {
		t.Fatal(err)
	}
	if on != 1 {
		t.Fatal("foreign_keys is off")
	}

	// And that it actually does something: deleting the organisation takes its
	// repositories with it.
	seedOrg(t, db)
	if _, err := db.Exec(`INSERT INTO repository (org_id, slug, name, forge_repo, created_at)
		VALUES (1, 'lastboard', 'Lastboard', 'CodigoSH/Lastboard', 0)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`DELETE FROM organization WHERE id = 1`); err != nil {
		t.Fatal(err)
	}

	var left int
	if err := db.QueryRow(`SELECT count(*) FROM repository`).Scan(&left); err != nil {
		t.Fatal(err)
	}
	if left != 0 {
		t.Errorf("%d repositories survived their organisation; the cascade did not run", left)
	}
}

// TestAnOrphanCannotBeCreated is the other half: the constraint refuses a row
// that points at nothing, rather than silently accepting it.
func TestAnOrphanCannotBeCreated(t *testing.T) {
	db := open(t)

	_, err := db.Exec(`INSERT INTO repository (org_id, slug, name, forge_repo, created_at)
		VALUES (999, 'ghost', 'Ghost', 'nobody/ghost', 0)`)
	if err == nil {
		t.Fatal("a repository was created under an organisation that does not exist")
	}
}

// TestSearchFollowsTheItems.
//
// The FTS index is external-content: the text lives in `item` and triggers keep
// the index in step. If a trigger is wrong the index drifts, and the symptom is
// not an error — it is search quietly returning something that was edited an
// hour ago, or missing something that exists.
func TestSearchFollowsTheItems(t *testing.T) {
	db := open(t)
	seedRepo(t, db)

	insert := func(publicID, title, body string) {
		t.Helper()
		if _, err := db.Exec(`INSERT INTO item
			(public_id, repo_id, kind, title, body, status, follow_token_hash, created_at)
			VALUES (?, 1, 'bug', ?, ?, 'new', 'x', 0)`, publicID, title, body); err != nil {
			t.Fatal(err)
		}
	}
	matches := func(query string) int {
		t.Helper()
		var n int
		if err := db.QueryRow(`SELECT count(*) FROM item_fts WHERE item_fts MATCH ?`, query).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}

	insert("aaa", "crash on import", "it closes when I import photos")
	if got := matches("import"); got != 1 {
		t.Fatalf("after insert, %d matches for 'import', want 1", got)
	}

	if _, err := db.Exec(`UPDATE item SET title = 'crash on export' WHERE public_id = 'aaa'`); err != nil {
		t.Fatal(err)
	}
	if got := matches("export"); got != 1 {
		t.Errorf("after update, %d matches for 'export', want 1 — the update trigger is not reindexing", got)
	}

	if _, err := db.Exec(`DELETE FROM item WHERE public_id = 'aaa'`); err != nil {
		t.Fatal(err)
	}
	if got := matches("export"); got != 0 {
		t.Errorf("after delete, %d matches remain — the delete trigger is leaving rows in the index", got)
	}
}

// TestAddColumnIsIdempotent covers the helper every later migration will use.
//
// It is exercised here rather than left until the first migration that needs
// it: a helper nobody has ever called is a helper nobody knows is broken.
func TestAddColumnIsIdempotent(t *testing.T) {
	db := open(t)

	run := func() error {
		tx, err := db.Begin()
		if err != nil {
			return err
		}
		defer func() { _ = tx.Rollback() }()
		if err := addColumn(tx, "repository", "probe_column", "TEXT NOT NULL DEFAULT ''"); err != nil {
			return err
		}
		return tx.Commit()
	}

	if err := run(); err != nil {
		t.Fatalf("first add: %v", err)
	}
	if err := run(); err != nil {
		t.Fatalf("second add should be a no-op, got: %v", err)
	}

	var found bool
	rows, err := db.Query("SELECT name FROM pragma_table_info('repository')")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatal(err)
		}
		if name == "probe_column" {
			found = true
		}
	}
	if !found {
		t.Error("the column was not added at all")
	}
}

// TestMigrationsAreNumberedInOrder guards the one property the whole scheme
// rests on: the stored number is an index into this list.
func TestMigrationsAreNumberedInOrder(t *testing.T) {
	for i, m := range migrations {
		if m.version != i+1 {
			t.Fatalf("migration %d is numbered %d: the list is renumbered or has a gap, "+
				"and every database in the wild stores one of these numbers", i, m.version)
		}
		if m.name == "" {
			t.Errorf("migration %d has no name; the error message on failure would say nothing", m.version)
		}
	}
}

func seedOrg(t *testing.T, db *sql.DB) {
	t.Helper()
	if _, err := db.Exec(`INSERT INTO organization (id, slug, name, forge_kind, base_url, forge_owner, created_at)
		VALUES (1, 'codigosh', 'CodigoSH', 'forgejo', 'https://git.example.com', 'CodigoSH', 0)`); err != nil {
		t.Fatal(err)
	}
}

func seedRepo(t *testing.T, db *sql.DB) {
	t.Helper()
	seedOrg(t, db)
	if _, err := db.Exec(`INSERT INTO repository (id, org_id, slug, name, forge_repo, created_at)
		VALUES (1, 1, 'lastboard', 'Lastboard', 'CodigoSH/Lastboard', 0)`); err != nil {
		t.Fatal(err)
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}
