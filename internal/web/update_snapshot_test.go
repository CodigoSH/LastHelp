package web

import (
	"database/sql"
	"path/filepath"
	"testing"
)

// What the updater copies before it replaces anything is the database as the
// server sees it, not its file: the file of a database running in WAL is missing
// whatever was written since the last checkpoint.
func TestTheCopyTakenBeforeAnUpdateIsTheDatabaseAndNotTheFile(t *testing.T) {
	in, s := newPushInstance(t)

	if _, err := in.dbHandle.Exec(`CREATE TABLE probe (id INTEGER PRIMARY KEY, body TEXT)`); err != nil {
		t.Fatal(err)
	}
	if _, err := in.dbHandle.Exec(`PRAGMA wal_checkpoint(TRUNCATE)`); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 200; i++ {
		if _, err := in.dbHandle.Exec(`INSERT INTO probe (body) VALUES ('report')`); err != nil {
			t.Fatal(err)
		}
	}

	snapshot := s.updater().Snapshot
	if snapshot == nil {
		t.Fatal("the updater was given no way to copy the database")
	}
	target := filepath.Join(t.TempDir(), "copy.db")
	if err := snapshot(target); err != nil {
		t.Fatal(err)
	}

	copied, err := sql.Open("sqlite", "file:"+filepath.ToSlash(target)+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = copied.Close() }()
	var n int
	if err := copied.QueryRow(`SELECT COUNT(*) FROM probe`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 200 {
		t.Errorf("the copy holds %d of 200 rows", n)
	}
}
