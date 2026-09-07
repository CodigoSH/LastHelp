package database

import (
	"database/sql"
	"fmt"
)

// migration is one step. Steps are append-only and never renumbered: the
// version stored in the file is an index into this list, so changing what step
// 3 means changes what every existing database thinks it has applied.
type migration struct {
	version int
	name    string
	stmts   []string
}

// LatestSchema is the highest schema version this binary knows.
//
// It exists so a candidate binary can be asked, before it replaces anything,
// whether it understands the database it is about to inherit. With a beta
// channel and an updater, somebody will eventually try to install an older
// binary over a newer database, and the only good moment to refuse is before
// the swap.
func LatestSchema() int { return migrations[len(migrations)-1].version }

// SchemaVersion is what a database is currently at.
func SchemaVersion(db *sql.DB) (int, error) { return userVersion(db) }

// Migrate brings the database up to the schema this binary expects.
//
// PRAGMA user_version is the bookkeeping. It costs nothing, it is atomic with
// the transaction that sets it, and it needs no table of its own.
func Migrate(db *sql.DB) error {
	current, err := userVersion(db)
	if err != nil {
		return err
	}

	latest := migrations[len(migrations)-1].version

	// The downgrade guard. With a beta channel and an updater, somebody will
	// eventually run an older binary against a newer database — and the failure
	// without this is not a clean stop, it is an old binary quietly writing
	// rows that ignore columns it does not know about.
	if current > latest {
		return fmt.Errorf(
			"this database is at schema version %d and this binary only knows up to %d: "+
				"it was written by a newer version of LastHelp. Upgrade the binary, or restore "+
				"a backup taken before the upgrade — running on anyway would lose data written "+
				"by the newer one", current, latest)
	}

	for _, m := range migrations {
		if m.version <= current {
			continue
		}
		if err := apply(db, m); err != nil {
			return fmt.Errorf("migration %d (%s): %w", m.version, m.name, err)
		}
	}
	return nil
}

// apply runs one migration and its version bump in a single transaction, so a
// migration that fails half way leaves the database at the version it was.
func apply(db *sql.DB, m migration) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	for _, stmt := range m.stmts {
		if _, err := tx.Exec(stmt); err != nil {
			return fmt.Errorf("%w\n\nstatement:\n%s", err, stmt)
		}
	}

	// PRAGMA does not take a bound parameter, and the value is an int from this
	// file rather than anything a user typed.
	if _, err := tx.Exec(fmt.Sprintf("PRAGMA user_version = %d", m.version)); err != nil {
		return err
	}
	return tx.Commit()
}

func userVersion(db *sql.DB) (int, error) {
	var v int
	if err := db.QueryRow("PRAGMA user_version").Scan(&v); err != nil {
		return 0, fmt.Errorf("reading the schema version: %w", err)
	}
	return v, nil
}

// addColumn is how later migrations add a column, and it is idempotent on
// purpose.
//
// Two rules from this house's scar tissue, both of which cost real data:
//
//  1. Adding a column is always its own ALTER, even when the CREATE TABLE above
//     also lists it. The CREATE only ever runs on a fresh install; every
//     database that already exists gets the column from here and nowhere else.
//  2. An old ALTER is never deleted. Removing one re-breaks every database old
//     enough to still need it, and those are exactly the ones nobody is
//     watching.
func addColumn(tx *sql.Tx, table, column, definition string) error {
	rows, err := tx.Query("SELECT name FROM pragma_table_info(?)", table)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return err
		}
		if name == column {
			return nil
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}

	_, err = tx.Exec(fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s", table, column, definition))
	return err
}
