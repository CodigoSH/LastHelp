package web

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"git.codigosh.com/CodigoSH/LastHelp/internal/core/authz"
)

// handleBackup hands over a copy of the database.
//
// VACUUM INTO rather than reading the file: a SQLite file copied while the
// server is writing is a file that restores into a database missing whatever
// was in flight. VACUUM INTO takes a consistent snapshot and writes it whole,
// which is the difference between a backup and a file that looks like one.
//
// The copy is made beside the database rather than in a temporary directory,
// because those are often on a different filesystem and a backup that fails at
// three in the morning for lack of space in /tmp is a backup nobody has.
func (s *Server) handleBackup(w http.ResponseWriter, r *http.Request) {
	if !s.may(w, r, authz.ManageServer, authz.Scope{}) {
		return
	}

	snapshot := filepath.Join(filepath.Dir(s.cfg.DBFile), fmt.Sprintf(".backup-%d.db", s.now().UnixNano()))
	defer func() { _ = os.Remove(snapshot) }()

	if _, err := s.db.Exec(`VACUUM INTO ?`, snapshot); err != nil {
		writeError(w, http.StatusInternalServerError, "error.internal")
		return
	}

	file, err := os.Open(snapshot)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "error.internal")
		return
	}
	defer func() { _ = file.Close() }()

	info, err := file.Stat()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "error.internal")
		return
	}

	name := fmt.Sprintf("lasthelp-%s.db", s.now().UTC().Format("2006-01-02-1504"))
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
	w.Header().Set("Content-Length", fmt.Sprintf("%d", info.Size()))
	http.ServeContent(w, r, name, time.Time{}, file)
}
