package web

import (
	"net/http"
	"os"
	"path/filepath"
	"runtime"

	"git.codigosh.com/CodigoSH/LastHelp/internal/core/authz"
	"git.codigosh.com/CodigoSH/LastHelp/internal/platform/database"
	"git.codigosh.com/CodigoSH/LastHelp/internal/version"
)

// handleServerInfo is what the About screen shows an administrator.
//
// It used to show the version, the commit hash, the schema number and the name
// of the installation — three of which answer questions nobody asks, and the
// fourth of which is a setting that lives on the Personalization screen. What
// somebody running a server wants from this page is whether it is healthy: how
// long it has been up, how big the database has grown, whether the disk it
// sits on is filling, and what it is costing in memory.
//
// Administrator only. None of it is secret, exactly, but the free space on the
// operator's disk is not the public's business.
func (s *Server) handleServerInfo(w http.ResponseWriter, r *http.Request) {
	if !s.may(w, r, authz.ManageServer, authz.Scope{}) {
		return
	}

	out := map[string]any{
		"version": version.Current,
		"go":      runtime.Version(),
		"os":      runtime.GOOS + "/" + runtime.GOARCH,
	}

	// Seconds rather than a formatted string: the page knows the reader's
	// language and this does not.
	out["uptime_seconds"] = int64(s.now().Sub(s.startedAt).Seconds())
	out["started_at"] = s.startedAt.Unix()

	// The database and everything SQLite keeps beside it. Reporting only the
	// main file understates it by however much is sitting in the journal.
	var dbBytes int64
	for _, suffix := range []string{"", "-wal", "-shm"} {
		if info, err := os.Stat(s.cfg.DBFile + suffix); err == nil {
			dbBytes += info.Size()
		}
	}
	// Omitted rather than reported as zero when there is nothing to stat. A
	// zero on that card reads as "the database is empty"; the truth is "this
	// server could not find its file", and those want different reactions.
	if dbBytes > 0 {
		out["database_bytes"] = dbBytes
	}

	if schema, err := database.SchemaVersion(s.db); err == nil {
		out["schema"] = schema
	}

	dir := filepath.Dir(s.cfg.DBFile)
	if dir == "" {
		dir = "."
	}
	if free, total, ok := diskFree(dir); ok {
		out["disk_free_bytes"] = free
		out["disk_total_bytes"] = total
	}

	// What this process is using, which is the number the operator can act on.
	// Not the machine's total: that belongs to whatever else runs there, and a
	// figure this server cannot influence is one it should not report as its
	// own.
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)
	out["memory_bytes"] = mem.Sys - mem.HeapReleased
	out["goroutines"] = runtime.NumGoroutine()

	writeJSON(w, http.StatusOK, out)
}
