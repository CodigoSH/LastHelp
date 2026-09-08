package web

import (
	"database/sql"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"git.codigosh.com/CodigoSH/LastHelp/internal/core/authz"
	"git.codigosh.com/CodigoSH/LastHelp/internal/platform/database"
)

// maxBackupSize bounds an upload that is written to disk before it is read.
const maxBackupSize = 512 << 20 // 512 MiB

// handleRestore replaces this installation's database with an uploaded one.
//
// The order matters more than anything else here, and it is the order of the
// updater for the same reason: nothing is replaced until the replacement has
// been proved.
//
//  1. The upload is written beside the database, never over it.
//  2. It is opened as a database and asked what it is. A file that is not
//     SQLite, or is SQLite but is not this application's, stops here.
//  3. Its schema version is checked against this binary's. Newer means it came
//     from a later version and this one would write into a shape it does not
//     understand; older is fine, because the migrations run on start.
//  4. The current database is kept, not deleted, under a name with the time in
//     it — so a restore of the wrong file is survivable.
//
// The server then has to be restarted to pick it up, which the panel says
// plainly rather than pretending the swap was live.
func (s *Server) handleRestore(w http.ResponseWriter, r *http.Request) {
	if !s.may(w, r, authz.ManageServer, authz.Scope{}) {
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxBackupSize)
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		writeError(w, http.StatusBadRequest, "error.bad_request")
		return
	}
	upload, _, err := r.FormFile("backup")
	if err != nil {
		writeError(w, http.StatusBadRequest, "error.bad_request")
		return
	}
	defer func() { _ = upload.Close() }()

	dir := filepath.Dir(s.cfg.DBFile)
	candidate := filepath.Join(dir, fmt.Sprintf(".restore-%d.db", s.now().UnixNano()))
	out, err := os.OpenFile(candidate, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "error.internal")
		return
	}
	written := false
	defer func() {
		_ = out.Close()
		if !written {
			_ = os.Remove(candidate)
		}
	}()

	if _, err := io.Copy(out, upload); err != nil {
		writeError(w, http.StatusBadRequest, "error.bad_request")
		return
	}
	if err := out.Close(); err != nil {
		writeError(w, http.StatusInternalServerError, "error.internal")
		return
	}

	if err := looksLikeOurDatabase(candidate); err != nil {
		writeError(w, http.StatusBadRequest, "error.not_a_backup")
		return
	}

	kept := filepath.Join(dir, fmt.Sprintf("lasthelp-replaced-%s.db", s.now().UTC().Format("2006-01-02-1504")))
	if err := os.Rename(s.cfg.DBFile, kept); err != nil {
		writeError(w, http.StatusInternalServerError, "error.internal")
		return
	}
	if err := os.Rename(candidate, s.cfg.DBFile); err != nil {
		// Put back what was there. A failure here that left no database at all
		// would be worse than the restore not happening.
		_ = os.Rename(kept, s.cfg.DBFile)
		writeError(w, http.StatusInternalServerError, "error.internal")
		return
	}
	written = true

	writeJSON(w, http.StatusOK, map[string]any{"restored": true, "kept": filepath.Base(kept)})
}

// looksLikeOurDatabase opens the file and asks it what it is.
//
// Checking the header bytes would only prove it is SQLite. What matters is
// whether it is *this* application's database and whether this binary can read
// its shape, and both questions are answered by opening it.
func looksLikeOurDatabase(path string) error {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()

	var version int
	if err := db.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
		return fmt.Errorf("not a database this can read: %w", err)
	}
	if version > database.LatestSchema() {
		return fmt.Errorf("the backup is from a newer version (schema %d, this understands %d)",
			version, database.LatestSchema())
	}

	var n int
	if err := db.QueryRow(`SELECT count(*) FROM app_user`).Scan(&n); err != nil {
		return fmt.Errorf("not a LastHelp backup: %w", err)
	}
	return nil
}
