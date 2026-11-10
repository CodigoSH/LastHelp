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

	// Open or closed, and for the closed ones what was done about them, the way the
	// public board splits them. Asked of published reports only: a report turned
	// down or still waiting has the status "new" and is not open in any sense that
	// matters to somebody counting what is left to do, so the question is refused
	// where it would have counted them and the moderation filter is the way to them.
	state, outcome := query.Get("state"), query.Get("outcome")
	if (state != "" && state != "open" && state != "closed") ||
		(outcome != "" && (state != "closed" || (outcome != "done" && outcome != "undone"))) {
		writeError(w, http.StatusBadRequest, "error.bad_request")
		return
	}

	// The order: newest first unless asked, the longest-waiting first, or the most voted.
	order := query.Get("sort")
	if order != "" && order != "votes" && order != "oldest" {
		writeError(w, http.StatusBadRequest, "error.bad_request")
		return
	}

	// What nobody on the team has answered. It means published and not finished,
	// as the overview counts it: a report closed without a word is a different
	// question from one still waiting for one.
	reply := query.Get("reply")
	if reply != "" && reply != "none" {
		writeError(w, http.StatusBadRequest, "error.bad_request")
		return
	}
	if reply == "none" && state == "" {
		state = "open"
	}

	base := item.Filter{
		RepoIDs:    ids,
		Kind:       item.Kind(query.Get("kind")),
		Search:     query.Get("q"),
		SortByVote: order == "votes",
		SortOldest: order == "oldest",
		Unanswered: reply == "none",
	}
	// narrowed is the list as asked for one choice of state and outcome, and
	// whether that choice can hold anything at all given the other filters. The
	// list and the counts on the switches both come from here, so a switch says
	// how many rows pressing it would show.
	narrowed := func(state, outcome string) (item.Filter, bool) {
		f := base
		// Several of each may be asked at once, comma-separated: "completed" is both ways of being done,
		// so every status named stands for what it is alike to.
		var wanted []item.Status
		seenStatus := map[item.Status]bool{}
		for _, raw := range splitList(query.Get("status")) {
			for _, st := range item.Alike(item.Status(raw)) {
				if !seenStatus[st] {
					seenStatus[st] = true
					wanted = append(wanted, st)
				}
			}
		}
		var moderations []item.Moderation
		for _, raw := range splitList(query.Get("moderation")) {
			moderations = append(moderations, item.Moderation(raw))
		}
		f.AnyModeration = len(moderations) == 0
		f.Moderations = moderations
		if state == "" {
			f.Statuses = wanted
			return f, true
		}
		// Open and closed are of what is published: asking for states that leave it out leaves nothing.
		if len(moderations) > 0 {
			published := false
			for _, m := range moderations {
				if m == item.Published {
					published = true
				}
			}
			if !published {
				return f, false
			}
		}
		f.Moderations, f.AnyModeration, f.Moderation = nil, false, item.Published
		allowed := item.OpenStatuses()
		switch {
		case outcome == "done":
			allowed = item.DeliveredStatuses()
		case outcome == "undone":
			allowed = item.TurnedDownStatuses()
		case state == "closed":
			allowed = item.ClosedStatuses()
		}
		if len(wanted) == 0 {
			f.Statuses = allowed
			return f, true
		}
		// The statuses picked have to be of this side's, or nothing is left.
		var kept []item.Status
		for _, st := range allowed {
			if seenStatus[st] {
				kept = append(kept, st)
			}
		}
		if len(kept) == 0 {
			return f, false
		}
		f.Statuses = kept
		return f, true
	}

	filter, holds := narrowed(state, outcome)
	filter.Limit, filter.Offset = intParam(query.Get("limit"), 50), intParam(query.Get("offset"), 0)

	var list []item.Item
	if len(ids) > 0 && holds {
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

	// What each switch of open and closed would show, under every other filter.
	// Not scope-wide like the counts above, which exist to hide an option that
	// finds nothing: these are numbers on a switch, and what a switch says is what
	// pressing it draws.
	states := map[string]int{}
	for name, ask := range map[string][2]string{
		"all": {"", ""}, "open": {"open", ""}, "closed": {"closed", ""},
		"done": {"closed", "done"}, "undone": {"closed", "undone"},
	} {
		f, possible := narrowed(ask[0], ask[1])
		if !possible || len(ids) == 0 {
			states[name] = 0
			continue
		}
		if states[name], err = s.items().Tally(f); err != nil {
			slog.Error("counting reports", "error", err)
			writeError(w, http.StatusInternalServerError, "error.internal")
			return
		}
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
			"state":      states,
		},
	})
}

// splitList reads a comma-separated list of values from an address: no blanks, none twice.
func splitList(raw string) []string {
	var out []string
	seen := map[string]bool{}
	for _, v := range strings.Split(raw, ",") {
		v = strings.TrimSpace(v)
		if v != "" && !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	return out
}
