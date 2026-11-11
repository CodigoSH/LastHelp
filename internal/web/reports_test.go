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
	make("acme/atlas", "bug", "unreproducible bug", "published", string(item.StatusDeclined))
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
	askOrdered := func(who *instance, query string) (titles []string, counts map[string]any, code int) {
		resp := who.do(http.MethodGet, "/api/reports?"+query, nil)
		body := decode(t, resp)
		for _, raw := range body["items"].([]any) {
			titles = append(titles, raw.(map[string]any)["title"].(string))
		}
		return titles, nil, resp.StatusCode
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
	// Completed is one word for both kinds too: it finds the idea that shipped and the bug
	// that was fixed, and nothing the team turned down.
	completed, _, _ := ask(admin, "state=closed&status=done&board=acme/atlas")
	same("closed and completed", completed, "shipped idea", "fixed bug")
	// And on "all", where no side was chosen: the same two, not only the ideas.
	anySide, _, _ := ask(admin, "status=done&board=acme/atlas")
	same("completed on every side", anySide, "shipped idea", "fixed bug")
	viaFixed, _, _ := ask(admin, "state=closed&status=closed&board=acme/atlas")
	same("closed and fixed", viaFixed, "shipped idea", "fixed bug")
	// Declined is one word for both kinds now, so it finds a declined bug too.
	same("closed and declined", one, "declined idea", "unreproducible bug")
	// Open and closed are of what is published: turned down is reached by moderation.
	turned, _, _ := ask(admin, "state=open&moderation=rejected&board=acme/atlas")
	same("open and rejected", turned)
	rejected, _, _ := ask(admin, "moderation=rejected&board=acme/atlas")
	same("rejected, without a side", rejected, "turned down idea")

	// Several statuses and several states of moderation at once, comma-separated.
	many, _, _ := ask(admin, "status=done,declined&board=acme/atlas")
	same("completed or declined", many, "shipped idea", "fixed bug", "declined idea", "unreproducible bug")
	onOpenSide, _, _ := ask(admin, "state=open&status=declined,new&board=acme/atlas")
	same("declined or new on the open side", onOpenSide, "open idea")
	waitingOrTurnedDown, _, _ := ask(admin, "moderation=pending,rejected&board=acme/atlas")
	same("pending or rejected", waitingOrTurnedDown, "waiting idea", "turned down idea")
	both, _, _ := ask(admin, "state=closed&moderation=published,rejected&board=acme/atlas")
	same("a side with published among the states", both, "shipped idea", "declined idea", "fixed bug", "unreproducible bug")
	neither, _, _ := ask(admin, "state=closed&moderation=pending,rejected&board=acme/atlas")
	same("a side without published among the states", neither)
	// The order: newest first by default, the longest-waiting first when asked, and nothing else.
	newest, _, _ := askOrdered(admin, "board=acme/atlas")
	oldest, _, _ := askOrdered(admin, "board=acme/atlas&sort=oldest")
	if len(newest) < 2 || len(oldest) != len(newest) {
		t.Fatalf("order: %v and %v", newest, oldest)
	}
	for i := range newest {
		if newest[i] != oldest[len(oldest)-1-i] {
			t.Errorf("oldest first should be the newest list reversed: %v and %v", newest, oldest)
			break
		}
	}
	if _, _, code := ask(admin, "sort=sideways"); code != http.StatusBadRequest {
		t.Errorf("an order that does not exist answered %d, want 400", code)
	}

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

// TestSeveralReportsCanBeDecidedAtOnce: publishing, rejecting or marking as spam many reports in one request, each one asked
// about on its own — a report of a board the actor does not work on is skipped and counted, and nothing else is touched.
func TestSeveralReportsCanBeDecidedAtOnce(t *testing.T) {
	admin, _, collaborator := teamSetup(t)
	public := admin.anonymous()

	idsOf := func(board string, titles ...string) []string {
		org, repo, _ := splitBoard(board)
		var ids []string
		for _, title := range titles {
			id, _ := public.submit(org, repo, "idea", title)
			ids = append(ids, id)
		}
		return ids
	}
	mine := idsOf("acme/atlas", "one", "two", "three")
	foreign := idsOf("other/elsewhere", "not theirs")

	moderationOf := func(id string) string {
		resp := admin.do(http.MethodGet, "/api/items/"+id, nil)
		return decode(t, resp)["moderation"].(string)
	}
	bulk := func(who *instance, state string, ids ...string) (map[string]any, int) {
		resp := who.do(http.MethodPost, "/api/reports/bulk", map[string]any{"ids": ids, "state": state})
		code := resp.StatusCode
		return decode(t, resp), code
	}

	// A collaborator decides what they work on and skips what they do not.
	got, code := bulk(collaborator, "published", append(append([]string{}, mine...), foreign...)...)
	if code != http.StatusOK || got["done"] != float64(3) || got["skipped"] != float64(1) || got["failed"] != float64(0) {
		t.Fatalf("a mix of their board and another: %d %v", code, got)
	}
	for _, id := range mine {
		if moderationOf(id) != "published" {
			t.Errorf("%s is %s, want published", id, moderationOf(id))
		}
	}
	if moderationOf(foreign[0]) != "pending" {
		t.Errorf("a report of a board they do not work on moved to %s", moderationOf(foreign[0]))
	}

	// The administrator can decide everything, and rejecting and spam are decisions of the same kind.
	if got, _ := bulk(admin, "rejected", mine[0], mine[1]); got["done"] != float64(2) {
		t.Errorf("rejecting two: %v", got)
	}
	if got, _ := bulk(admin, "spam", mine[2]); got["done"] != float64(1) || moderationOf(mine[2]) != "spam" {
		t.Errorf("marking one as spam: %v, now %s", got, moderationOf(mine[2]))
	}

	// What makes no sense is refused, and nothing is guessed.
	for name, body := range map[string]map[string]any{
		"an unknown state": {"ids": mine, "state": "pending"},
		"no reports":       {"ids": []string{}, "state": "published"},
		"too many":         {"ids": manyIDs(101), "state": "published"},
	} {
		resp := admin.do(http.MethodPost, "/api/reports/bulk", body)
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("%s answered %d, want 400", name, resp.StatusCode)
		}
		_ = resp.Body.Close()
	}
	// A visitor cannot.
	if resp := public.do(http.MethodPost, "/api/reports/bulk", map[string]any{"ids": mine, "state": "published"}); resp.StatusCode == http.StatusOK {
		t.Error("an anonymous visitor decided reports")
	}
}

func manyIDs(n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = "x" + string(rune('a'+i%26)) + string(rune('a'+(i/26)%26))
	}
	return out
}

// TestTheReportsScreenCanBeNarrowedToHowOldReportsAre: the date filter, by name, and a refusal for an age that does not exist.
func TestTheReportsScreenCanBeNarrowedToHowOldReportsAre(t *testing.T) {
	admin, _, _ := teamSetup(t)
	public := admin.anonymous()
	org, repo, _ := splitBoard("acme/atlas")
	id, _ := public.submit(org, repo, "idea", "made just now")
	resp := admin.do(http.MethodPost, "/api/items/"+id+"/moderation", map[string]string{"state": "published"})
	_ = resp.Body.Close()

	count := func(query string) int {
		resp := admin.do(http.MethodGet, "/api/reports?"+query, nil)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("%s answered %d", query, resp.StatusCode)
		}
		return len(decode(t, resp)["items"].([]any))
	}
	for _, query := range []string{"age=day", "age=week", "age=month"} {
		if count(query+"&board=acme/atlas") != 1 {
			t.Errorf("%s did not find what was made just now", query)
		}
	}
	if count("age=older&board=acme/atlas") != 0 {
		t.Error("what was made just now is older than a month")
	}
	if resp := admin.do(http.MethodGet, "/api/reports?age=yesterday", nil); resp.StatusCode != http.StatusBadRequest {
		t.Errorf("an age that does not exist answered %d, want 400", resp.StatusCode)
	}
}
