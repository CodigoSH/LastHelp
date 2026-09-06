// Package database opens the SQLite file and keeps its schema current.
//
// It is the only package that knows the file is SQLite. Everything above it
// receives an interface narrow enough to be handed a transaction instead.
package database

import (
	"database/sql"
	"fmt"
	"net/url"
	"path/filepath"

	// The driver is pure Go: no cgo, so the binary stays a single static file
	// that runs on a scratch image.
	_ "modernc.org/sqlite"
)

// Execer is what a store takes. It is satisfied by both *sql.DB and *sql.Tx,
// which is the whole point: a caller can hand several stores the same
// transaction and make their work atomic. A store that held *sql.DB could not
// be part of anybody else's transaction.
type Execer interface {
	Exec(query string, args ...any) (sql.Result, error)
	Query(query string, args ...any) (*sql.Rows, error)
	QueryRow(query string, args ...any) *sql.Row
}

// Open opens the database and applies any pending migrations.
func Open(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", dsn(path))
	if err != nil {
		return nil, fmt.Errorf("opening %s: %w", path, err)
	}

	// Ping now rather than on the first query: a permission problem or a
	// directory that does not exist should stop the server at startup, where
	// the message is read, and not on somebody's first request.
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("opening %s: %w", path, err)
	}

	// One writer. SQLite serialises writes anyway, and letting the pool open
	// several write connections turns that serialisation into SQLITE_BUSY
	// errors surfacing as failed requests. Reads are not affected: WAL lets
	// them proceed while a write is in flight.
	db.SetMaxOpenConns(1)

	if err := Migrate(db); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}

// dsn builds the connection string.
//
// The pragmas go here, not in a db.Exec after opening. A pragma executed as a
// statement applies to whichever pooled connection happened to run it, so
// foreign keys would be enforced on some queries and not others — which is the
// kind of bug that shows up months later as an orphaned row nobody can explain.
func dsn(path string) string {
	pragmas := []string{
		// Referential integrity is off by default in SQLite. Everything below
		// leans on ON DELETE CASCADE, so this is not optional.
		"foreign_keys(1)",
		// Readers do not block the writer and the writer does not block
		// readers. A portal serving a public page while a webhook writes is
		// the normal case here.
		"journal_mode(WAL)",
		// Wait rather than fail when the write lock is held.
		"busy_timeout(5000)",
		// WAL makes fsync-on-every-commit unnecessary for durability against a
		// process crash; it still holds against one, and the trade is only
		// against a power cut mid-write.
		"synchronous(NORMAL)",
	}

	q := url.Values{}
	for _, p := range pragmas {
		q.Add("_pragma", p)
	}
	return "file:" + filepath.ToSlash(path) + "?" + q.Encode()
}
