package web

import (
	"net/http"
	"strconv"
	"testing"

	"git.codigosh.com/CodigoSH/LastHelp/internal/core/item"
)

// TestThePublicListSplitsOpenFromClosedAndSaysHowMany.
//
// A visitor chooses between what still waits and what has an answer, and the
// list says how many there are on each side and in all — so a board with more
// reports than one page says it is showing some of them, rather than stopping
// at fifty without a word, which is what it did.
func TestThePublicListSplitsOpenFromClosedAndSaysHowMany(t *testing.T) {
	team := newInstance(t)
	team.claim()
	team.createOrg("codigosh")
	team.createRepo("codigosh", "lastboard")

	store := item.Store{DB: team.db()}
	public := team.anonymous()
	// 25 ideas, the first 9 answered; and 2 bugs, one of them fixed.
	for i := 0; i < 25; i++ {
		id, _ := public.submit("codigosh", "lastboard", "idea", "idea number "+strconv.Itoa(i))
		it, err := store.ByPublicID(id)
		if err != nil {
			t.Fatal(err)
		}
		if err := store.Approve(it.ID, team.now()); err != nil {
			t.Fatal(err)
		}
		if i < 9 {
			if err := store.SetStatus(it.ID, item.StatusDone, team.now()); err != nil {
				t.Fatal(err)
			}
		}
	}
	for i, st := range []item.Status{item.StatusClosed, item.StatusConfirmed} {
		id, _ := public.submit("codigosh", "lastboard", "bug", "bug number "+strconv.Itoa(i))
		it, _ := store.ByPublicID(id)
		_ = store.Approve(it.ID, team.now())
		_ = store.SetStatus(it.ID, st, team.now())
	}

	list := func(query string) (items []any, total float64, counts map[string]any) {
		t.Helper()
		body := decode(t, public.do(http.MethodGet, "/api/orgs/codigosh/repos/lastboard/items?"+query, nil))
		items, _ = body["items"].([]any)
		total, _ = body["total"].(float64)
		counts, _ = body["counts"].(map[string]any)
		return
	}

	items, total, counts := list("state=open&limit=10")
	if len(items) != 10 || total != 17 {
		t.Errorf("open: %d rows of %v, want 10 of 17", len(items), total)
	}
	if counts["open"] != float64(17) || counts["closed"] != float64(10) {
		t.Errorf("counts %v, want 17 open and 10 closed", counts)
	}
	for _, raw := range items {
		if st := item.Status(raw.(map[string]any)["status"].(string)); st.Closes() {
			t.Errorf("an open list carried %s", st)
		}
	}

	// The counts follow the kind and the search, not the side chosen.
	_, total, counts = list("state=closed&kind=bug")
	if total != 1 || counts["open"] != float64(1) || counts["closed"] != float64(1) {
		t.Errorf("closed bugs: total %v, counts %v, want 1, and 1 each side", total, counts)
	}

	// A second page is the next ten, none repeated.
	first, _, _ := list("state=open&limit=10")
	second, _, _ := list("state=open&limit=10&offset=10")
	seen := map[string]bool{}
	for _, raw := range append(first, second...) {
		id := raw.(map[string]any)["public_id"].(string)
		if seen[id] {
			t.Errorf("%s came back on both pages", id)
		}
		seen[id] = true
	}
	if len(second) != 7 || len(seen) != 17 {
		t.Errorf("pages of %d and %d covered %d, want 10, 7 and 17", len(first), len(second), len(seen))
	}

	// A side nobody defined is refused, not read as both.
	if got := status(t, public.do(http.MethodGet, "/api/orgs/codigosh/repos/lastboard/items?state=archived", nil)); got != http.StatusBadRequest {
		t.Errorf("an unknown state answered %d", got)
	}
}
