package database

import (
	"database/sql"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// rowsIn counts the rows of the probe table in a database file opened on its own.
func rowsIn(path string) (int, error) {
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(path)+"?mode=ro")
	if err != nil {
		return 0, err
	}
	defer func() { _ = db.Close() }()
	var n int
	err = db.QueryRow(`SELECT COUNT(*) FROM probe`).Scan(&n)
	return n, err
}

func copyFile(t *testing.T, from, to string) {
	t.Helper()
	in, err := os.Open(from)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = in.Close() }()
	out, err := os.Create(to)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = out.Close() }()
	if _, err := io.Copy(out, in); err != nil {
		t.Fatal(err)
	}
}

// The database runs in WAL: what was written since the last checkpoint lives in
// the -wal file, and the main file does not have it. A backup that copies the
// main file is a backup that is missing exactly the latest work, and
// integrity_check calls it fine, because it is: a whole file, just an old one.
func TestASnapshotHoldsWhatWasWrittenSinceTheLastCheckpoint(t *testing.T) {
	dir := t.TempDir()
	live := filepath.Join(dir, "live.db")
	db, err := Open(live)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if _, err := db.Exec(`CREATE TABLE probe (id INTEGER PRIMARY KEY, body TEXT)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`PRAGMA wal_checkpoint(TRUNCATE)`); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 200; i++ {
		if _, err := db.Exec(`INSERT INTO probe (body) VALUES (?)`, "report"); err != nil {
			t.Fatal(err)
		}
	}

	// What the updater used to do: copy the file.
	plain := filepath.Join(dir, "plain.db")
	copyFile(t, live, plain)
	if n, err := rowsIn(plain); err == nil && n == 200 {
		t.Fatal("a plain copy of the file already has every row: this test no longer shows what it is for")
	}

	snapshot := filepath.Join(dir, "snapshot.db")
	if err := Snapshot(db, snapshot); err != nil {
		t.Fatal(err)
	}
	n, err := rowsIn(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if n != 200 {
		t.Errorf("the snapshot holds %d of 200 rows", n)
	}
}

// A copy is proved before anybody relies on it: the file is opened, checked, and
// has to hold the same tables as the database it was taken from.
func TestASnapshotThatIsNotTheDatabaseIsRefused(t *testing.T) {
	dir := t.TempDir()
	db, err := Open(filepath.Join(dir, "live.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	// Not a database at all.
	garbage := filepath.Join(dir, "garbage.db")
	if err := os.WriteFile(garbage, []byte(strings.Repeat("this is not sqlite. ", 400)), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := verifySnapshot(db, garbage); err == nil {
		t.Error("a file that is not a database was accepted")
	}

	// A real database, opening fine, with a page of its content destroyed: the file is there and the
	// tables are right, and only the integrity check says it is not whole.
	damaged := filepath.Join(dir, "damaged.db")
	if _, err := db.Exec(`CREATE TABLE padding (id INTEGER PRIMARY KEY, body TEXT)`); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 400; i++ {
		if _, err := db.Exec(`INSERT INTO padding (body) VALUES (?)`, strings.Repeat("x", 200)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec(`VACUUM INTO ?`, damaged); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(damaged)
	if err != nil {
		t.Fatal(err)
	}
	for i := len(raw) * 2 / 3; i < len(raw)*2/3+4096 && i < len(raw); i++ {
		raw[i] = 0xFF
	}
	if err := os.WriteFile(damaged, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := verifySnapshot(db, damaged); err == nil {
		t.Error("a copy with a damaged page was accepted")
	}

	version, err := SchemaVersion(db)
	if err != nil {
		t.Fatal(err)
	}

	// A real database at the same schema version that holds other tables: only the tables give it away.
	other := filepath.Join(dir, "other.db")
	o, err := sql.Open("sqlite", "file:"+filepath.ToSlash(other))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := o.Exec(`CREATE TABLE unrelated (id INTEGER)`); err != nil {
		t.Fatal(err)
	}
	if _, err := o.Exec(fmt.Sprintf(`PRAGMA user_version = %d`, version)); err != nil {
		t.Fatal(err)
	}
	_ = o.Close()
	if err := verifySnapshot(db, other); err == nil {
		t.Error("a database with other tables was accepted as a copy of this one")
	}

	// A true copy that is at another schema version: only the version gives it away.
	old := filepath.Join(dir, "old.db")
	if _, err := db.Exec(`VACUUM INTO ?`, old); err != nil {
		t.Fatal(err)
	}
	w, err := sql.Open("sqlite", "file:"+filepath.ToSlash(old))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Exec(fmt.Sprintf(`PRAGMA user_version = %d`, version-1)); err != nil {
		t.Fatal(err)
	}
	_ = w.Close()
	if err := verifySnapshot(db, old); err == nil {
		t.Error("a copy at another schema version was accepted")
	}
}
