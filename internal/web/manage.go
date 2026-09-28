package web

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"git.codigosh.com/CodigoSH/LastHelp/internal/core/account"
	"git.codigosh.com/CodigoSH/LastHelp/internal/core/authz"
	"git.codigosh.com/CodigoSH/LastHelp/internal/core/org"
	"git.codigosh.com/CodigoSH/LastHelp/internal/web/middleware"
)

type rename struct {
	Name string `json:"name"`
}

// handleRenameOrganization changes the name, never the address.
func (s *Server) handleRenameOrganization(w http.ResponseWriter, r *http.Request) {
	o, ok := s.lookupOrganization(w, r)
	if !ok {
		return
	}
	if !s.may(w, r, authz.ManageOrganization, authz.Organization(o.ID)) {
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, panelBodyLimit)
	var in rename
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil || strings.TrimSpace(in.Name) == "" {
		writeError(w, http.StatusBadRequest, "error.bad_request")
		return
	}
	if err := s.orgs().RenameOrganization(o.ID, in.Name); err != nil {
		writeError(w, http.StatusInternalServerError, "error.internal")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"name": strings.TrimSpace(in.Name)})
}

// handleDeleteOrganization removes it and everything under it.
//
// The administrator's, not the owner's. An owner runs an organisation; ending
// one — with every report anybody ever wrote in it — is a decision about the
// server, and the person who can do it is the person who can make another.
func (s *Server) handleDeleteOrganization(w http.ResponseWriter, r *http.Request) {
	o, ok := s.lookupOrganization(w, r)
	if !ok {
		return
	}
	if !s.may(w, r, authz.ManageServer, authz.Scope{}) {
		return
	}
	if err := s.orgs().DeleteOrganization(o.ID); err != nil {
		writeError(w, http.StatusInternalServerError, "error.internal")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

type repositoryChange struct {
	Name         string `json:"name"`
	ForgeRepo    string `json:"forge_repo"`
	AcceptsBugs  bool   `json:"accepts_bugs"`
	AcceptsIdeas bool   `json:"accepts_ideas"`
}

func (s *Server) handleUpdateRepository(w http.ResponseWriter, r *http.Request) {
	repo, _, ok := s.lookupRepository(w, r)
	if !ok {
		return
	}
	if !s.may(w, r, authz.ManageOrganization, authz.Organization(repo.OrgID)) {
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, panelBodyLimit)
	var in repositoryChange
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil || strings.TrimSpace(in.Name) == "" {
		writeError(w, http.StatusBadRequest, "error.bad_request")
		return
	}

	switch err := s.orgs().UpdateRepository(repo.ID, in.Name, in.ForgeRepo, in.AcceptsBugs, in.AcceptsIdeas); {
	case errors.Is(err, org.ErrNothingAccepted):
		writeError(w, http.StatusBadRequest, "error.accepts_nothing")
		return
	case err != nil:
		writeError(w, http.StatusInternalServerError, "error.internal")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleDeleteRepository(w http.ResponseWriter, r *http.Request) {
	repo, _, ok := s.lookupRepository(w, r)
	if !ok {
		return
	}
	if !s.may(w, r, authz.ManageServer, authz.Scope{}) {
		return
	}
	if err := s.orgs().DeleteRepository(repo.ID); err != nil {
		writeError(w, http.StatusInternalServerError, "error.internal")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// --- people -----------------------------------------------------------------

type personChange struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Disabled *bool  `json:"disabled"`
	Admin    *bool  `json:"admin"`
}

// handleUpdatePerson changes somebody else's account.
//
// Setting a password here does not ask for the old one, because the person
// doing it is not the person who knows it — that is what an administrator
// resetting a forgotten password is. Changing your own still asks, on the other
// endpoint, and for the other reason.
func (s *Server) handleUpdatePerson(w http.ResponseWriter, r *http.Request) {
	if !s.may(w, r, authz.ManageServer, authz.Scope{}) {
		return
	}
	userID, ok := pathID(w, r)
	if !ok {
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, credentialsBodyLimit)
	var in personChange
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "error.bad_request")
		return
	}

	accounts := s.accounts()
	if in.Username != "" {
		if _, ok := s.rename(w, userID, in.Username); !ok {
			return
		}
	}
	if in.Password != "" {
		if len(in.Password) < minPasswordLength {
			writeError(w, http.StatusBadRequest, "error.password_too_short")
			return
		}
		if err := accounts.ResetPassword(userID, in.Password); err != nil {
			writeError(w, http.StatusInternalServerError, "error.internal")
			return
		}
	}
	if in.Disabled != nil {
		if err := accounts.SetDisabled(userID, *in.Disabled); err != nil {
			writeError(w, http.StatusInternalServerError, "error.internal")
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// handleDeletePerson removes an account.
//
// Not the last administrator, and not yourself. Both are the same mistake seen
// from two sides: an installation with nobody who can administer it cannot be
// fixed from inside, and there is no sign-up to make another.
func (s *Server) handleDeletePerson(w http.ResponseWriter, r *http.Request) {
	if !s.may(w, r, authz.ManageServer, authz.Scope{}) {
		return
	}
	actor, ok := middleware.ActorFrom(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "error.unauthorized")
		return
	}
	userID, ok := pathID(w, r)
	if !ok {
		return
	}
	if userID == actor.UserID {
		writeError(w, http.StatusBadRequest, "error.cannot_delete_self")
		return
	}

	people, err := s.accounts().List()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "error.internal")
		return
	}
	administrators, target := 0, account.User{}
	for _, p := range people {
		if p.IsAdministrator {
			administrators++
		}
		if p.ID == userID {
			target = p
		}
	}
	if target.ID == 0 {
		writeError(w, http.StatusNotFound, "error.not_found")
		return
	}
	if target.IsAdministrator && administrators <= 1 {
		writeError(w, http.StatusBadRequest, "error.last_administrator")
		return
	}

	if err := s.accounts().Delete(userID); err != nil {
		writeError(w, http.StatusInternalServerError, "error.internal")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// pathID reads the {id} every one of these carries.
func pathID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "error.bad_request")
		return 0, false
	}
	return id, true
}
