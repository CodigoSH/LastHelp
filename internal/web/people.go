package web

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"git.codigosh.com/CodigoSH/LastHelp/internal/core/account"
	"git.codigosh.com/CodigoSH/LastHelp/internal/core/authz"
	"git.codigosh.com/CodigoSH/LastHelp/internal/web/middleware"
)

// --- the account you are signed in as ---------------------------------------

type passwordChange struct {
	Current string `json:"current"`
	New     string `json:"new"`
}

// handleChangePassword changes your own password and nobody else's.
//
// The current one is asked for even though the session already proves who you
// are: a session left open on a shared machine is the ordinary way an account
// is taken, and a change that asked for nothing would hand it over for good.
func (s *Server) handleChangePassword(w http.ResponseWriter, r *http.Request) {
	actor, ok := middleware.ActorFrom(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "error.unauthorized")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, credentialsBodyLimit)
	var in passwordChange
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "error.bad_request")
		return
	}
	if len(in.New) < minPasswordLength {
		writeError(w, http.StatusBadRequest, "error.password_too_short")
		return
	}

	switch err := s.accounts().SetPassword(actor.UserID, in.Current, in.New); {
	case errors.Is(err, account.ErrNotFound):
		writeError(w, http.StatusForbidden, "error.password_wrong")
		return
	case err != nil:
		writeError(w, http.StatusInternalServerError, "error.internal")
		return
	}

	// Every other session of this account ends. Changing a password because you
	// think somebody else has it, and leaving their session alive, is the one
	// thing this must not do.
	if err := s.sessions().DeleteAllFor(actor.UserID); err != nil {
		writeError(w, http.StatusInternalServerError, "error.internal")
		return
	}
	s.startSession(w, r, actor.UserID)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

type profileChange struct {
	DisplayName string `json:"display_name"`
}

// handleChangeProfile changes the name shown beside a reply.
func (s *Server) handleChangeProfile(w http.ResponseWriter, r *http.Request) {
	actor, ok := middleware.ActorFrom(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "error.unauthorized")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, credentialsBodyLimit)
	var in profileChange
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "error.bad_request")
		return
	}
	if len([]rune(strings.TrimSpace(in.DisplayName))) > maxDisplayNameLength {
		writeError(w, http.StatusBadRequest, "error.bad_request")
		return
	}
	if err := s.accounts().SetDisplayName(actor.UserID, in.DisplayName); err != nil {
		writeError(w, http.StatusInternalServerError, "error.internal")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"display_name": strings.TrimSpace(in.DisplayName)})
}

const maxDisplayNameLength = 60

// --- the people who may use this installation -------------------------------

// handleListPeople answers who has an account and what each one may do.
func (s *Server) handleListPeople(w http.ResponseWriter, r *http.Request) {
	if !s.may(w, r, authz.ManageServer, authz.Scope{}) {
		return
	}

	users, err := s.accounts().List()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "error.internal")
		return
	}

	out := make([]map[string]any, 0, len(users))
	for _, u := range users {
		memberships, err := s.accounts().Memberships(u.ID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "error.internal")
			return
		}
		grants := make([]map[string]any, 0, len(memberships))
		for _, m := range memberships {
			grants = append(grants, map[string]any{
				"scope": string(m.Scope.Kind),
				"id":    m.Scope.ID,
				"role":  string(m.Role),
			})
		}
		out = append(out, map[string]any{
			"id":       u.ID,
			"username": u.Username,
			"name":     u.DisplayName,
			"admin":    u.IsAdministrator,
			"disabled": u.Disabled,
			"grants":   grants,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"people": out})
}

type newPerson struct {
	Username    string `json:"username"`
	Password    string `json:"password"`
	DisplayName string `json:"display_name"`
}

// handleCreatePerson adds an account.
//
// There is no public sign-up and there never will be: everybody who can act
// here was put here by somebody who already could.
func (s *Server) handleCreatePerson(w http.ResponseWriter, r *http.Request) {
	if !s.may(w, r, authz.ManageServer, authz.Scope{}) {
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, credentialsBodyLimit)
	var in newPerson
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "error.bad_request")
		return
	}
	if len(strings.TrimSpace(in.Username)) < minUsernameLength {
		writeError(w, http.StatusBadRequest, "error.username_required")
		return
	}
	if len(in.Password) < minPasswordLength {
		writeError(w, http.StatusBadRequest, "error.password_too_short")
		return
	}

	user, err := s.accounts().Create(in.Username, in.Password, in.DisplayName, false, s.now())
	switch {
	case errors.Is(err, account.ErrNameTaken):
		writeError(w, http.StatusConflict, "error.username_taken")
		return
	case err != nil:
		writeError(w, http.StatusInternalServerError, "error.internal")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"id": user.ID, "username": user.Username})
}

type grantChange struct {
	Scope string `json:"scope"`
	ID    int64  `json:"id"`
	OrgID int64  `json:"org_id"`
	// An empty role means take it away, which is one endpoint rather than two
	// that could disagree about what a scope is.
	Role string `json:"role"`
}

// handleGrant gives somebody a role in one place, or takes it away.
func (s *Server) handleGrant(w http.ResponseWriter, r *http.Request) {
	if !s.may(w, r, authz.ManageServer, authz.Scope{}) {
		return
	}

	userID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "error.bad_request")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, credentialsBodyLimit)
	var in grantChange
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "error.bad_request")
		return
	}

	scope, ok := authz.ScopeFrom(in.Scope, in.ID, in.OrgID)
	if !ok {
		writeError(w, http.StatusBadRequest, "error.bad_request")
		return
	}

	if in.Role == "" {
		if err := s.accounts().Revoke(userID, scope); err != nil {
			writeError(w, http.StatusInternalServerError, "error.internal")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
		return
	}

	role, ok := authz.RoleFrom(in.Role)
	if !ok {
		writeError(w, http.StatusBadRequest, "error.bad_request")
		return
	}
	if err := s.accounts().Grant(userID, scope, role, s.now()); err != nil {
		writeError(w, http.StatusInternalServerError, "error.internal")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}
