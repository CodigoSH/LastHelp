package web

import (
	"encoding/json"
	"net/http"
	"strings"

	"git.codigosh.com/CodigoSH/LastHelp/internal/core/authz"
	"git.codigosh.com/CodigoSH/LastHelp/internal/core/settings"
)

// instanceSettings is what an operator can change about the installation
// itself, as opposed to about one organisation.
type instanceSettings struct {
	Name string `json:"name"`
}

// maxInstanceNameLength is a heading, not a paragraph. Bounded here rather than
// only in the browser, where a bound is a suggestion.
const maxInstanceNameLength = 60

// handleGetInstance answers what this installation calls itself.
//
// Readable by anybody who may see the panel, because the name is on every
// screen anyway; only changing it is the administrator's.
func (s *Server) handleGetInstance(w http.ResponseWriter, r *http.Request) {
	if !s.may(w, r, authz.ViewPanel, authz.Scope{}) {
		return
	}
	name, err := settings.Store{DB: s.db}.Get(settings.InstanceName)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "error.internal")
		return
	}
	writeJSON(w, http.StatusOK, instanceSettings{Name: name})
}

// handleSetInstance changes it.
func (s *Server) handleSetInstance(w http.ResponseWriter, r *http.Request) {
	if !s.may(w, r, authz.ManageServer, authz.Scope{}) {
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, panelBodyLimit)
	var in instanceSettings
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "error.bad_request")
		return
	}

	name := strings.TrimSpace(in.Name)
	if len([]rune(name)) > maxInstanceNameLength {
		writeError(w, http.StatusBadRequest, "error.bad_request")
		return
	}
	if err := (settings.Store{DB: s.db}).Set(settings.InstanceName, name); err != nil {
		writeError(w, http.StatusInternalServerError, "error.internal")
		return
	}
	writeJSON(w, http.StatusOK, instanceSettings{Name: name})
}
