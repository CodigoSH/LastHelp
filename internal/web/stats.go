package web

import (
	"net/http"

	"git.codigosh.com/CodigoSH/LastHelp/internal/core/authz"
)

// overview is what the panel's front page draws.
//
// Counted in the database rather than by fetching everything and counting in
// the browser: a portal that has been open for two years has a lot of reports,
// and a dashboard that gets slower the longer it has been useful is one nobody
// opens.
type overview struct {
	Organizations int            `json:"organizations"`
	Repositories  int            `json:"repositories"`
	People        int            `json:"people"`
	Pending       int            `json:"pending"`
	Kinds         map[string]int `json:"kinds"`
	Statuses      map[string]int `json:"statuses"`
	Moderation    map[string]int `json:"moderation"`
	Votes         int            `json:"votes"`
	Published     int            `json:"published"`
	// LastReport is when the most recent report arrived, in seconds. Zero means
	// none ever has — which is a state worth showing rather than a blank.
	LastReport int64 `json:"last_report"`
}

// handleOverview counts what this installation holds.
//
// Everything an administrator sees; for anybody else it is still their whole
// world, because the queries are not scoped — and that is why this asks for
// ManageServer rather than ViewPanel. A collaborator gets the repositories they
// triage from the screens that are already scoped to them.
func (s *Server) handleOverview(w http.ResponseWriter, r *http.Request) {
	if !s.may(w, r, authz.ManageServer, authz.Scope{}) {
		return
	}

	out := overview{
		Kinds:      map[string]int{},
		Statuses:   map[string]int{},
		Moderation: map[string]int{},
	}

	single := func(query string, into *int) bool {
		if err := s.db.QueryRow(query).Scan(into); err != nil {
			writeError(w, http.StatusInternalServerError, "error.internal")
			return false
		}
		return true
	}
	if !single(`SELECT count(*) FROM organization`, &out.Organizations) ||
		!single(`SELECT count(*) FROM repository`, &out.Repositories) ||
		!single(`SELECT count(*) FROM app_user`, &out.People) ||
		!single(`SELECT count(*) FROM item WHERE moderation_state = 'pending'`, &out.Pending) ||
		!single(`SELECT count(*) FROM item WHERE moderation_state = 'published'`, &out.Published) ||
		!single(`SELECT count(*) FROM vote`, &out.Votes) {
		return
	}

	grouped := func(query string, into map[string]int) bool {
		rows, err := s.db.Query(query)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "error.internal")
			return false
		}
		defer func() { _ = rows.Close() }()
		for rows.Next() {
			var key string
			var n int
			if err := rows.Scan(&key, &n); err != nil {
				writeError(w, http.StatusInternalServerError, "error.internal")
				return false
			}
			into[key] = n
		}
		return rows.Err() == nil
	}
	if !grouped(`SELECT kind, count(*) FROM item GROUP BY kind`, out.Kinds) ||
		!grouped(`SELECT status, count(*) FROM item WHERE moderation_state = 'published' GROUP BY status`, out.Statuses) ||
		!grouped(`SELECT moderation_state, count(*) FROM item GROUP BY moderation_state`, out.Moderation) {
		return
	}

	// COALESCE, because MAX over no rows is NULL and scanning that into an int64
	// fails — which would turn "nothing has been reported yet" into a 500.
	if err := s.db.QueryRow(`SELECT COALESCE(MAX(created_at), 0) FROM item`).Scan(&out.LastReport); err != nil {
		writeError(w, http.StatusInternalServerError, "error.internal")
		return
	}

	writeJSON(w, http.StatusOK, out)
}
