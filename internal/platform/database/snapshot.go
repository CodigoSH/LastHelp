package database

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"slices"
)

// Snapshot writes a consistent copy of db to target, and proves the copy opens
// whole before saying it is one. target must not exist.
//
// VACUUM INTO, never a copy of the file. The database runs in WAL, so what was
// written since the last checkpoint is in the -wal file and not in the main one:
// a copy of the main file looks like a database, passes integrity_check, and is
// missing exactly the latest work. VACUUM INTO reads a consistent snapshot, WAL
// included, and writes it whole. It is what the downloadable backup does, and
// the one the updater takes before it replaces anything.
func Snapshot(db *sql.DB, target string) error {
	if _, err := db.Exec(`VACUUM INTO ?`, target); err != nil {
		return fmt.Errorf("copying the database: %w", err)
	}
	if err := verifySnapshot(db, target); err != nil {
		return fmt.Errorf("the copy of the database is not usable: %w", err)
	}
	return nil
}

// verifySnapshot opens the copy on its own, read-only and without the pragmas or
// migrations of a normal open, and checks that it is a sound database with the
// same tables and the same schema version as the one it was taken from.
//
// integrity_check alone would not do: it says whether the file is intact, not
// whether it is the right file. Completeness is VACUUM INTO's job; this catches
// a copy that is truncated, damaged or not of this database at all.
func verifySnapshot(source *sql.DB, target string) error {
	snap, err := sql.Open("sqlite", "file:"+filepath.ToSlash(target)+"?mode=ro")
	if err != nil {
		return err
	}
	defer func() { _ = snap.Close() }()

	var verdict string
	if err := snap.QueryRow(`PRAGMA integrity_check`).Scan(&verdict); err != nil {
		return err
	}
	if verdict != "ok" {
		return fmt.Errorf("integrity_check says %q", verdict)
	}

	want, err := tableNames(source)
	if err != nil {
		return err
	}
	got, err := tableNames(snap)
	if err != nil {
		return err
	}
	if !slices.Equal(want, got) {
		return fmt.Errorf("it holds %d tables and the database has %d", len(got), len(want))
	}

	wantVersion, err := SchemaVersion(source)
	if err != nil {
		return err
	}
	gotVersion, err := SchemaVersion(snap)
	if err != nil {
		return err
	}
	if wantVersion != gotVersion {
		return fmt.Errorf("it is at schema %d and the database is at %d", gotVersion, wantVersion)
	}
	return nil
}

func tableNames(db *sql.DB) ([]string, error) {
	rows, err := db.Query(`SELECT name FROM sqlite_master WHERE type = 'table' ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	names := []string{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		names = append(names, name)
	}
	return names, rows.Err()
}
