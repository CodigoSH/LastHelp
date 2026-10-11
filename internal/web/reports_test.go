package web

import (
	"net/http"
	"sort"
	"testing"

	"git.codigosh.com/CodigoSH/LastHelp/internal/core/item"
)

// TestTheReportsScreenCanBeNarrowedToOpenAndClosed: the switches, the way the
// public board has them, over the published reports only, with counts that are what
// the list then holds and a refusal for a question that makes no sense.
func TestTheReportsScreenCanBeNarrowedToOpenAndClosed(t *testing.T) {
	admin, _, collaborator := teamSetup(t)
	public := admin.anonymous()

	make := func(board, kind, title string, moderation, status string) {
		org, repo, _ := splitBoard(board)
		id, _ := public.submit(org, repo, kind, title)
		if moderation != "" {
			resp := admin.do(http.MethodPost, "/api/items/"+id+"/moderation", map[string]string{"state": moderation})
			_ = resp.Body.Close()
		}
		if status != "" {
			resp := admin.do(http.MethodPost, "/api/items/"+id+"/status", map[string]string{"status": status})
			_ = resp.Body.Close()
		}
	}
	make("acme/atlas", "idea", "shipped idea", "published", string(item.StatusDone))
	make("acme/atlas", "idea", "declined idea", "published", string(item.StatusDeclined))
	make("acme/atlas", "idea", "open idea", "published", "")
	make("acme/atlas", "idea", "turned down idea", "rejected", "")
	make("acme/atlas", "idea", "waiting idea", "", "")
	make("acme/atlas", "bug", "fixed bug", "published", string(item.StatusClosed))
	make("acme/atlas", "bug", "unreproducible bug", "published", string(item.StatusNotReproducible))
	make("other/elsewhere", "bug", "somebody else's closed bug", "published", string(item.StatusClosed))

	ask := func(who *instance, query string) (titles []string, counts map[string]any, code int) {
		resp := who.do(http.MethodGet, "/api/reports?"+query, nil)
		body := decode(t, resp)
		if resp.StatusCode != http.StatusOK {
			return nil, nil, resp.StatusCode
		}
		for _, raw := range body["items"].([]any) {
			titles = append(titles, raw.(map[string]any)["title"].(string))
		}
		sort.Strings(titles)
		counts, _ = body["counts"].(map[string]any)["state"].(map[string]any)
		return titles, counts, resp.StatusCode
	}
	same := func(label string, got []string, want ...string) {
		t.Helper()
		sort.Strings(want)
		if len(got) != len(want) {
			t.Errorf("%s: %v, want %v", label, got, want)
			return
		}
		for i := range got {
			if got[i] != want[i] {
				t.Errorf("%s: %v, want %v", label, got, want)
				return
			}
		}
	}

	open, counts, _ := ask(admin, "state=open&board=acme/atlas")
	same("open", open, "open idea")
	closed, _, _ := ask(admin, "state=closed&board=acme/atlas")
	same("closed", closed, "shipped idea", "declined idea", "fixed bug", "unreproducible bug")
	done, _, _ := ask(admin, "state=closed&outcome=done&board=acme/atlas")
	same("done", done, "shipped idea", "fixed bug")
	undone, _, _ := ask(admin, "state=closed&outcome=undone&board=acme/atlas")
	same("not done", undone, "declined idea", "unreproducible bug")

	// What a switch says is what pressing it draws, under the filters in force.
	want := map[string]float64{"all": 7, "open": 1, "closed": 4, "done": 2, "undone": 2}
	for name, n := range want {
		if counts[name] != n {
			t.Errorf("counts under the board: %s is %v, want %v (%v)", name, counts[name], n, counts)
		}
	}
	_, ideas, _ := ask(admin, "kind=idea&board=acme/atlas")
	for name, n := range map[string]float64{"all": 5, "open": 1, "closed": 2, "done": 1, "undone": 1} {
		if ideas[name] != n {
			t.Errorf("counts for ideas: %s is %v, want %v (%v)", name, ideas[name], n, ideas)
		}
	}

	// A status that is not of the side chosen leaves nothing; one that is, narrows it.
	none, _, _ := ask(admin, "state=open&status=done&board=acme/atlas")
	same("an open side with a delivered status", none)
	one, _, _ := ask(admin, "state=closed&status=declined&board=acme/atlas")
	same("closed and declined", one, "declined idea")
	// Open and closed are of what is published: turned down is reached by moderation.
	turned, _, _ := ask(admin, "state=open&moderation=rejected&board=acme/atlas")
	same("open and rejected", turned)
	rejected, _, _ := ask(admin, "moderation=rejected&board=acme/atlas")
	same("rejected, without a side", rejected, "turned down idea")

	// The questions that make no sense are refused rather than read as "both".
	for _, query := range []string{"state=maybe", "outcome=done", "state=open&outcome=done", "state=closed&outcome=perhaps"} {
		if _, _, code := ask(admin, query); code != http.StatusBadRequest {
			t.Errorf("%q answered %d, want 400", query, code)
		}
	}

	// A collaborator counts and sees only the boards they work on.
	theirs, theirCounts, _ := ask(collaborator, "state=closed")
	same("a collaborator's closed", theirs, "shipped idea", "declined idea", "fixed bug", "unreproducible bug")
	if theirCounts["closed"] != float64(4) {
		t.Errorf("a collaborator is counted %v closed, want only their board's four", theirCounts["closed"])
	}
}

// cut splits "org/repo".
func splitBoard(board string) (string, string, bool) {
	for i := 0; i < len(board); i++ {
		if board[i] == '/' {
			return board[:i], board[i+1:], true
		}
	}
	return board, "", false
}
