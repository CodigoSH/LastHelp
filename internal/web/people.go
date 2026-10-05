package web

import (
	"encoding/json"
	"errors"
	"log/slog"
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
	Username string `json:"username"`
}

// handleChangeProfile renames the account: the name it signs in with and the
// one its replies carry, which are the same name.
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
	username, ok := s.rename(w, actor.UserID, in.Username)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"username": username})
}

type languageChange struct {
	Language string `json:"language"`
}

// handleChangeLanguage stores the language this account reads in.
//
// On the account rather than in the browser, so it follows the person: chosen
// on a desktop and there on the laptop. Empty is allowed and means "not
// chosen", which hands the decision back to the browser.
func (s *Server) handleChangeLanguage(w http.ResponseWriter, r *http.Request) {
	actor, ok := middleware.ActorFrom(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "error.unauthorized")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, credentialsBodyLimit)
	var in languageChange
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "error.bad_request")
		return
	}
	if in.Language != "" && !knownLocale(in.Language) {
		writeError(w, http.StatusBadRequest, "error.bad_request")
		return
	}
	if err := s.accounts().SetLanguage(actor.UserID, in.Language); err != nil {
		writeError(w, http.StatusInternalServerError, "error.internal")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"language": in.Language})
}

const maxUsernameLength = 60

// rename is the one way a username changes, for the account itself and for an
// administrator changing somebody else's: the same bounds and the same answer
// when the name is already somebody's.
func (s *Server) rename(w http.ResponseWriter, userID int64, username string) (string, bool) {
	username = strings.ToLower(strings.TrimSpace(username))
	if n := len([]rune(username)); n < minUsernameLength || n > maxUsernameLength {
		writeError(w, http.StatusBadRequest, "error.username_required")
		return "", false
	}
	switch err := s.accounts().Rename(userID, username); {
	case errors.Is(err, account.ErrNameTaken):
		writeError(w, http.StatusConflict, "error.username_taken")
		return "", false
	case errors.Is(err, account.ErrBadUsername):
		writeError(w, http.StatusBadRequest, "error.username_chars")
		return "", false
	case err != nil:
		writeError(w, http.StatusInternalServerError, "error.internal")
		return "", false
	}
	return username, true
}

const maxAvatarBytes = 96 << 10

type avatarChange struct {
	Avatar string `json:"avatar"`
}

// handleChangeAvatar stores the picture beside your name, or clears it.
func (s *Server) handleChangeAvatar(w http.ResponseWriter, r *http.Request) {
	actor, ok := middleware.ActorFrom(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "error.unauthorized")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxAvatarBytes+(8<<10))
	var in avatarChange
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		// Same as the logo: a body cut off at the bound is an oversized
		// picture, and saying "the request was not understood" sends somebody
		// looking for a problem with their browser.
		if tooLarge(err) {
			writeError(w, http.StatusBadRequest, "error.image_too_big")
			return
		}
		writeError(w, http.StatusBadRequest, "error.bad_request")
		return
	}
	picture := strings.TrimSpace(in.Avatar)
	if picture != "" && (!strings.HasPrefix(picture, "data:image/") || len(picture) > maxAvatarBytes) {
		writeError(w, http.StatusBadRequest, "error.image_too_big")
		return
	}
	if err := s.accounts().SetAvatar(actor.UserID, picture); err != nil {
		writeError(w, http.StatusInternalServerError, "error.internal")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"avatar": picture})
}

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
			// The place, spelled the way the screen spells it. A row number
			// would make the browser look it up again, and it has no way to.
			grant := map[string]any{"scope": string(m.Scope.Kind), "role": string(m.Role)}
			switch m.Scope.Kind {
			case authz.ScopeOrganization:
				if o, err := s.orgs().OrganizationByID(m.Scope.ID); err == nil {
					grant["organization"] = o.Slug
					grant["where"] = o.Name
				}
			case authz.ScopeRepository:
				if repo, err := s.orgs().RepositoryByID(m.Scope.ID); err == nil {
					grant["repository"] = repo.Slug
					grant["where"] = repo.Name
					if o, err := s.orgs().OrganizationByID(m.Scope.OrgID); err == nil {
						grant["organization"] = o.Slug
					}
				}
			}
			grants = append(grants, grant)
		}
		out = append(out, map[string]any{
			"id":       u.ID,
			"username": u.Username,
			"admin":    u.IsAdministrator,
			"disabled": u.Disabled,
			"avatar":   u.Avatar,
			"grants":   grants,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"people": out})
}

type newPerson struct {
	Username string `json:"username"`
	Password string `json:"password"`
	// The language this person will read in. Chosen by whoever creates the
	// account, because the alternative is somebody's first sign-in landing in a
	// language they did not pick and having to find the setting in it.
	Language string `json:"language"`
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

	if in.Language != "" && !knownLocale(in.Language) {
		writeError(w, http.StatusBadRequest, "error.bad_request")
		return
	}

	user, err := s.accounts().Create(in.Username, in.Password, false, s.now())
	switch {
	case errors.Is(err, account.ErrNameTaken):
		writeError(w, http.StatusConflict, "error.username_taken")
		return
	case errors.Is(err, account.ErrBadUsername):
		writeError(w, http.StatusBadRequest, "error.username_chars")
		return
	case err != nil:
		writeError(w, http.StatusInternalServerError, "error.internal")
		return
	}
	if in.Language != "" {
		if err := s.accounts().SetLanguage(user.ID, in.Language); err != nil {
			slog.Error("storing the language of a new account", "error", err)
		}
	}
	writeJSON(w, http.StatusCreated, map[string]any{"id": user.ID, "username": user.Username})
}

// grantChange names a place the way every other endpoint names it: by slug.
//
// Not by identifier. The rest of this API addresses an organisation as
// "codigosh" and a repository as "codigosh/lasthelp", and an endpoint that took
// a row number instead would be the one place the browser had to know what a
// row number is.
type grantChange struct {
	Organization string `json:"organization"`
	Repository   string `json:"repository"`
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

	organization, err := s.orgs().OrganizationBySlug(in.Organization)
	if err != nil {
		writeError(w, http.StatusNotFound, "error.not_found")
		return
	}

	scope := authz.Organization(organization.ID)
	if in.Repository != "" {
		repository, err := s.orgs().RepositoryBySlugs(in.Organization, in.Repository)
		if err != nil {
			writeError(w, http.StatusNotFound, "error.not_found")
			return
		}
		scope = authz.Repository(repository.ID, organization.ID)
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
