package web

import (
	"log/slog"
	"net/http"
	"strings"

	"git.codigosh.com/CodigoSH/LastHelp/internal/core/authz"
	"git.codigosh.com/CodigoSH/LastHelp/internal/core/item"
	"git.codigosh.com/CodigoSH/LastHelp/internal/core/org"
	"git.codigosh.com/CodigoSH/LastHelp/internal/web/middleware"
)

// board is one repository with enough of its organisation to be named and
// linked. The panel's listing crosses organisations, so a bare repository is
// not identifiable: two of them can be called "Web".
type board struct {
	repo org.Repository
	org  org.Organization
}

// scopedBoards is every board this actor may triage.
//
// The decision is asked of authz once per board rather than reimplemented from
// the memberships here. An owner triages a whole organisation and a
// collaborator one repository, and that rule lives in exactly one place — which
// is the rule an architecture test enforces.
func (s *Server) scopedBoards(actor authz.Actor) ([]board, error) {
	organizations, err := s.orgs().ListOrganizations()
	if err != nil {
		return nil, err
	}

	var out []board
	for _, o := range organizations {
		repos, err := s.orgs().ListRepositories(o.ID)
		if err != nil {
			return nil, err
		}
		for _, r := range repos {
			if actor.Can(authz.Triage, authz.Repository(r.ID, o.ID)) {
				out = append(out, board{repo: r, org: o})
			}
		}
	}
	return out, nil
}

func describeBoard(b board) map[string]any {
	return map[string]any{
		"org":          b.org.Slug,
		"repo":         b.repo.Slug,
		"name":         b.repo.Name,
		"organization": b.org.Name,
	}
}

// handleReports is the panel's own listing: everything this account may triage,
// wherever it lives, with the filters the work actually needs.
//
// It exists because the only listings were per repository and per state: the
// queue of one board, or the public page of one board. Somebody who looks after
// four repositories had no screen that answered "what is waiting for me", and
// no way at all to see a report that had been published — which is most of
// them, and the ones people write in asking about.
func (s *Server) handleReports(w http.ResponseWriter, r *http.Request) {
	actor, ok := middleware.ActorFrom(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "error.unauthorized")
		return
	}
	// The door is a role anywhere; what it opens is decided per board below, so
	// an account with one repository sees one repository.
	if !s.may(w, r, authz.ViewPanel, authz.Scope{}) {
		return
	}

	boards, err := s.scopedBoards(actor)
	if err != nil {
		slog.Error("listing the boards somebody may triage", "error", err)
		writeError(w, http.StatusInternalServerError, "error.internal")
		return
	}

	query := r.URL.Query()

	// The board filter is intersected with what they may see rather than
	// trusted: a slug in the query string is a request, not a permission.
	chosen := strings.TrimSpace(query.Get("board"))
	ids := make([]int64, 0, len(boards))
	byID := map[int64]board{}
	for _, b := range boards {
		byID[b.repo.ID] = b
		if chosen == "" || chosen == b.org.Slug+"/"+b.repo.Slug {
			ids = append(ids, b.repo.ID)
		}
	}

	filter := item.Filter{
		RepoIDs:       ids,
		Kind:          item.Kind(query.Get("kind")),
		Status:        item.Status(query.Get("status")),
		Moderation:    item.Moderation(query.Get("moderation")),
		AnyModeration: query.Get("moderation") == "",
		Search:        query.Get("q"),
		SortByVote:    query.Get("sort") == "votes",
		Limit:         intParam(query.Get("limit"), 50),
		Offset:        intParam(query.Get("offset"), 0),
	}

	var list []item.Item
	if len(ids) > 0 {
		if list, err = s.items().List(filter); err != nil {
			slog.Error("listing reports", "error", err)
			writeError(w, http.StatusInternalServerError, "error.internal")
			return
		}
	}

	rows := make([]map[string]any, 0, len(list))
	for _, it := range list {
		entry := s.describeItem(it, "")
		entry["moderation"] = string(it.Moderation)
		entry["body"] = it.Body
		if b, found := byID[it.RepoID]; found {
			entry["board"] = describeBoard(b)
		}
		rows = append(rows, entry)
	}

	// The counts are over everything in scope, not over what the current filter
	// left: a tab that says how many are waiting has to keep saying it while
	// you are looking at something else.
	all := make([]int64, 0, len(boards))
	for _, b := range boards {
		all = append(all, b.repo.ID)
	}
	moderation, err := s.items().Counts(all, "moderation_state")
	if err != nil {
		slog.Error("counting reports", "error", err)
		writeError(w, http.StatusInternalServerError, "error.internal")
		return
	}
	statuses, err := s.items().Counts(all, "status")
	if err != nil {
		slog.Error("counting reports", "error", err)
		writeError(w, http.StatusInternalServerError, "error.internal")
		return
	}
	kinds, err := s.items().Counts(all, "kind")
	if err != nil {
		slog.Error("counting reports", "error", err)
		writeError(w, http.StatusInternalServerError, "error.internal")
		return
	}

	described := make([]map[string]any, 0, len(boards))
	for _, b := range boards {
		described = append(described, describeBoard(b))
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"items":  rows,
		"boards": described,
		"counts": map[string]any{
			"moderation": moderation,
			"status":     statuses,
			"kind":       kinds,
		},
	})
}
