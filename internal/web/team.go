package web

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"git.codigosh.com/CodigoSH/LastHelp/internal/core/account"
	"git.codigosh.com/CodigoSH/LastHelp/internal/core/authz"
	"git.codigosh.com/CodigoSH/LastHelp/internal/core/item"
	"git.codigosh.com/CodigoSH/LastHelp/internal/core/org"
	"git.codigosh.com/CodigoSH/LastHelp/internal/platform/notify"
	"git.codigosh.com/CodigoSH/LastHelp/internal/web/middleware"
)

// handleRepositoryPanel is everything the panel's page for one repository
// shows, in one answer.
//
// A repository's things used to live in four places: its queue on a screen of
// its own, its settings behind a pencil that nothing was wired to, its
// notification channel on the organisation's page under a picker of
// repositories, and who works on it only in People. This is the one place they
// are asked for.
//
// Whoever may triage it may open it. What they may change is said in the
// answer rather than worked out by the page, because a page that decides for
// itself who may do what is a second copy of the rules.
func (s *Server) handleRepositoryPanel(w http.ResponseWriter, r *http.Request) {
	repo, organisation, ok := s.lookupRepository(w, r)
	if !ok {
		return
	}
	if !s.may(w, r, authz.Triage, authz.Repository(repo.ID, organisation.ID)) {
		return
	}
	actor, _ := middleware.ActorFrom(r)

	counts, err := s.items().Counts([]int64{repo.ID}, "moderation_state")
	if err != nil {
		slog.Error("counting a repository's queue", "error", err)
		writeError(w, http.StatusInternalServerError, "error.internal")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"slug":          repo.Slug,
		"name":          repo.Name,
		"forge_repo":    repo.ForgeRepo,
		"accepts_bugs":  repo.AcceptsBugs,
		"accepts_ideas": repo.AcceptsIdeas,
		// Which channel, and never what is in it: an endpoint that returns a
		// stored token is a way to read one.
		"channel":      channelKind(repo.NotifyConfig),
		"pending":      counts[string(item.Pending)],
		"organization": map[string]any{"slug": organisation.Slug, "name": organisation.Name, "forge": organisation.Forge},
		"can_manage":   actor.Can(authz.ManageOrganization, authz.Organization(organisation.ID)),
		"can_delete":   actor.Can(authz.ManageServer, authz.Scope{}),
	})
}

// handleOrganizationPanel is everything the panel's page for one organisation
// shows: its repositories with what is waiting in each, its forge and whether
// it is connected, and what the reader may change.
//
// The repositories are the ones the reader may triage, not all of them: a
// collaborator on one board opens this page to reach that board, and a list
// of boards they would be refused is a list of doors that do not open.
// Somebody with no board here at all is refused the page.
func (s *Server) handleOrganizationPanel(w http.ResponseWriter, r *http.Request) {
	organisation, ok := s.lookupOrganization(w, r)
	if !ok {
		return
	}
	actor, _ := middleware.ActorFrom(r)
	manage := actor.Can(authz.ManageOrganization, authz.Organization(organisation.ID))

	repos, err := s.orgs().ListRepositories(organisation.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "error.internal")
		return
	}
	rows := make([]map[string]any, 0, len(repos))
	for _, repo := range repos {
		if !actor.Can(authz.Triage, authz.Repository(repo.ID, organisation.ID)) {
			continue
		}
		counts, err := s.items().Counts([]int64{repo.ID}, "moderation_state")
		if err != nil {
			slog.Error("counting a repository's queue", "error", err)
			writeError(w, http.StatusInternalServerError, "error.internal")
			return
		}
		rows = append(rows, map[string]any{
			"slug":          repo.Slug,
			"name":          repo.Name,
			"accepts_bugs":  repo.AcceptsBugs,
			"accepts_ideas": repo.AcceptsIdeas,
			"pending":       counts[string(item.Pending)],
		})
	}
	if !manage && len(rows) == 0 {
		writeError(w, http.StatusForbidden, "error.forbidden")
		return
	}

	about := map[string]any{"slug": organisation.Slug, "name": organisation.Name, "forge": string(organisation.Forge)}
	body := map[string]any{
		"organization": about,
		"repositories": rows,
		"can_manage":   manage,
		"can_delete":   actor.Can(authz.ManageServer, authz.Scope{}),
	}

	// Where the forge is and whether it is connected, for whoever runs it:
	// whether a token and a secret are stored, never what they are.
	if manage && organisation.Forge != org.ForgeNone {
		about["base_url"] = organisation.BaseURL
		about["forge_owner"] = organisation.ForgeOwner
		token, secret, err := s.orgs().Credentials(organisation.ID)
		switch {
		case errors.Is(err, org.ErrNoCredentials):
		case err != nil:
			writeError(w, http.StatusInternalServerError, "error.internal")
			return
		}
		body["connection"] = map[string]any{"token": token != "", "secret": secret != ""}
		if hook, err := s.orgs().HookID(organisation.ID); err == nil && hook != "" {
			about["webhook_path"] = "/hooks/" + hook
		}
	}

	writeJSON(w, http.StatusOK, body)
}

// handleOrganizationsPanel is the panel's list of organisations: the ones the
// reader may open, each with how many of its boards they work on, what is
// waiting in those, its forge and whether it is connected, and what the reader
// is there.
//
// The public list is every organisation by name, which is right for a visitor
// and wrong here: a collaborator was shown organisations whose page refuses
// them, and nobody was shown where the work was waiting until they opened
// each one.
func (s *Server) handleOrganizationsPanel(w http.ResponseWriter, r *http.Request) {
	actor, _ := middleware.ActorFrom(r)
	list, err := s.orgs().ListOrganizations()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "error.internal")
		return
	}

	out := make([]map[string]any, 0, len(list))
	for _, o := range list {
		manage := actor.Can(authz.ManageOrganization, authz.Organization(o.ID))
		repos, err := s.orgs().ListRepositories(o.ID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "error.internal")
			return
		}
		ids := make([]int64, 0, len(repos))
		for _, repo := range repos {
			if actor.Can(authz.Triage, authz.Repository(repo.ID, o.ID)) {
				ids = append(ids, repo.ID)
			}
		}
		if !manage && len(ids) == 0 {
			continue
		}
		pending := 0
		if len(ids) > 0 {
			counts, err := s.items().Counts(ids, "moderation_state")
			if err != nil {
				slog.Error("counting an organisation's queue", "error", err)
				writeError(w, http.StatusInternalServerError, "error.internal")
				return
			}
			pending = counts[string(item.Pending)]
		}

		row := map[string]any{
			"slug":         o.Slug,
			"name":         o.Name,
			"forge":        string(o.Forge),
			"repositories": len(ids),
			"pending":      pending,
			"role":         actor.StandingIn(o.ID),
			"can_manage":   manage,
		}
		// Whether it is connected, to whoever connects it — never with what.
		if manage && o.Forge != org.ForgeNone {
			_, _, err := s.orgs().Credentials(o.ID)
			switch {
			case errors.Is(err, org.ErrNoCredentials):
				row["connected"] = false
			case err != nil:
				writeError(w, http.StatusInternalServerError, "error.internal")
				return
			default:
				row["connected"] = true
			}
		}
		out = append(out, row)
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"organizations": out,
		"can_add":       actor.Can(authz.ManageServer, authz.Scope{}),
		"can_delete":    actor.Can(authz.ManageServer, authz.Scope{}),
	})
}

// channelKind reads which channel a stored configuration is, and nothing else.
// Through the notifier's own reader: a second parser of the same JSON is the
// kind of copy that reads a key the other spells differently.
func channelKind(stored string) string {
	return string(notify.Parse(stored).Kind)
}

// handleTeam says who works in one organisation, and on which of its
// repositories, as whoever runs it sees it.
//
// Its owners and the administrator, and nobody else: a collaborator triages a
// board without needing to know who else can. The accounts that could be added
// come with it, so the page can offer them without asking for the whole list
// of people — which stays the administrator's.
func (s *Server) handleTeam(w http.ResponseWriter, r *http.Request) {
	organisation, ok := s.lookupOrganization(w, r)
	if !ok {
		return
	}
	if !s.may(w, r, authz.ManageOrganization, authz.Organization(organisation.ID)) {
		return
	}

	repos, err := s.orgs().ListRepositories(organisation.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "error.internal")
		return
	}
	names := map[int64]org.Repository{}
	for _, repo := range repos {
		names[repo.ID] = repo
	}

	users, err := s.accounts().List()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "error.internal")
		return
	}

	members := make([]map[string]any, 0)
	accounts := make([]map[string]any, 0, len(users))
	for _, u := range users {
		if !u.Disabled {
			accounts = append(accounts, map[string]any{"username": u.Username})
		}
		memberships, err := s.accounts().Memberships(u.ID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "error.internal")
			return
		}
		roles := make([]map[string]any, 0)
		for _, m := range memberships {
			switch {
			case m.Scope.Kind == authz.ScopeOrganization && m.Scope.ID == organisation.ID:
				roles = append(roles, map[string]any{"role": string(m.Role), "repository": "", "where": organisation.Name})
			case m.Scope.Kind == authz.ScopeRepository:
				if repo, mine := names[m.Scope.ID]; mine {
					roles = append(roles, map[string]any{"role": string(m.Role), "repository": repo.Slug, "where": repo.Name})
				}
			}
		}
		if len(roles) == 0 {
			continue
		}
		members = append(members, map[string]any{
			"username": u.Username,
			"avatar":   u.Avatar,
			"admin":    u.IsAdministrator,
			"roles":    roles,
		})
	}

	writeJSON(w, http.StatusOK, map[string]any{"members": members, "accounts": accounts})
}

type teamChange struct {
	Username   string `json:"username"`
	Repository string `json:"repository"`
	Role       string `json:"role"`
}

// handleTeamChange gives or takes a role inside one organisation.
//
// Whoever runs the organisation may do it, not only the administrator: the
// person who knows who should triage a board is the person who owns it. The
// scope is held to this organisation — a repository is looked up under it, so
// a slug from next door is not found rather than granted — and each role only
// at its own level: owner of the organisation, collaborator on a repository.
// An empty role takes whatever was there.
func (s *Server) handleTeamChange(w http.ResponseWriter, r *http.Request) {
	organisation, ok := s.lookupOrganization(w, r)
	if !ok {
		return
	}
	if !s.may(w, r, authz.ManageOrganization, authz.Organization(organisation.ID)) {
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, credentialsBodyLimit)
	var in teamChange
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "error.bad_request")
		return
	}

	user, err := s.accountByUsername(in.Username)
	switch {
	case errors.Is(err, account.ErrNotFound):
		writeError(w, http.StatusNotFound, "error.not_found")
		return
	case err != nil:
		writeError(w, http.StatusInternalServerError, "error.internal")
		return
	}

	scope := authz.Organization(organisation.ID)
	want := authz.RoleOwner
	if in.Repository != "" {
		repo, err := s.orgs().RepositoryBySlugs(organisation.Slug, in.Repository)
		if err != nil {
			writeError(w, http.StatusNotFound, "error.not_found")
			return
		}
		scope = authz.Repository(repo.ID, organisation.ID)
		want = authz.RoleCollaborator
	}

	if in.Role == "" {
		if err := s.accounts().Revoke(user.ID, scope); err != nil {
			writeError(w, http.StatusInternalServerError, "error.internal")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
		return
	}

	role, known := authz.RoleFrom(in.Role)
	if !known || role != want {
		writeError(w, http.StatusBadRequest, "error.bad_request")
		return
	}
	if err := s.accounts().Grant(user.ID, scope, role, s.now()); err != nil {
		writeError(w, http.StatusInternalServerError, "error.internal")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// accountByUsername finds one account by its name, the way the team section
// names people.
func (s *Server) accountByUsername(username string) (account.User, error) {
	users, err := s.accounts().List()
	if err != nil {
		return account.User{}, err
	}
	want := strings.ToLower(strings.TrimSpace(username))
	for _, u := range users {
		if u.Username == want {
			return u, nil
		}
	}
	return account.User{}, account.ErrNotFound
}
