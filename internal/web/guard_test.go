package web

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"git.codigosh.com/CodigoSH/LastHelp/internal/core/guard"
)

func boardWithGuard(t *testing.T) (*instance, *instance) {
	t.Helper()
	team := newInstance(t)
	team.claim()
	team.createOrg("codigosh")
	team.createRepo("codigosh", "lastboard")
	return team, team.anonymous()
}

const boardAPI = "/api/orgs/codigosh/repos/lastboard"

func (i *instance) formToken() string {
	i.t.Helper()
	body := decode(i.t, i.do(http.MethodGet, boardAPI+"/form", nil))
	token, _ := body["token"].(string)
	return token
}

// TestAPostStraightAtTheEndpointIsRefused.
//
// Most automated submission never loads the page. This is the layer that costs
// nothing to write and everything to work around, and it is checked here at the
// endpoint rather than only in the guard's own tests: a layer that exists and is
// not wired in protects nothing.
func TestAPostStraightAtTheEndpointIsRefused(t *testing.T) {
	_, public := boardWithGuard(t)

	resp := public.do(http.MethodPost, boardAPI+"/items",
		map[string]any{"kind": "idea", "title": "buy watches"})
	body := decode(t, resp)

	if resp.StatusCode != http.StatusBadRequest || body["error"] != "error.form_stale" {
		t.Fatalf("a submission with no form token returned %d %v", resp.StatusCode, body)
	}
}

func TestAFormFilledInFasterThanAPersonTypesIsRefused(t *testing.T) {
	_, public := boardWithGuard(t)
	token := public.formToken()
	// No advance: the submission arrives in the same instant the form was
	// fetched.
	resp := public.do(http.MethodPost, boardAPI+"/items",
		map[string]any{"kind": "idea", "title": "instant", "token": token})
	body := decode(t, resp)

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("an instant submission returned %d %v", resp.StatusCode, body)
	}
}

func TestOneFormSubmitsOnce(t *testing.T) {
	_, public := boardWithGuard(t)
	token := public.formToken()
	public.tick.advance(5 * time.Second)

	send := func(title string) int {
		resp := public.do(http.MethodPost, boardAPI+"/items",
			map[string]any{"kind": "idea", "title": title, "body": title, "token": token})
		_ = resp.Body.Close()
		return resp.StatusCode
	}

	if got := send("the first one"); got != http.StatusCreated {
		t.Fatalf("the first submission returned %d", got)
	}
	if got := send("the second one"); got == http.StatusCreated {
		t.Error("the same form token submitted twice; one page load would be a licence to flood")
	}
}

// TestTheHoneypotAnswersAsIfItWorked.
//
// Telling a script which check caught it is telling it what to change. The
// answer is indistinguishable from success, and nothing is stored.
func TestTheHoneypotAnswersAsIfItWorked(t *testing.T) {
	team, public := boardWithGuard(t)
	token := public.formToken()
	public.tick.advance(5 * time.Second)

	resp := public.do(http.MethodPost, boardAPI+"/items", map[string]any{
		"kind": "idea", "title": "buy watches", "token": token,
		"website": "http://spam.example.com",
	})
	if resp.StatusCode != http.StatusCreated {
		t.Errorf("the honeypot answered %d, which tells a script exactly what it did wrong", resp.StatusCode)
	}
	_ = resp.Body.Close()

	queue := decode(t, team.do(http.MethodGet, boardAPI+"/queue", nil))
	if got := len(queue["items"].([]any)); got != 0 {
		t.Errorf("%d items reached the queue from a filled honeypot", got)
	}
}

// TestTheSameParagraphTwiceIsRefused is the copy-paste flood, at the endpoint.
func TestTheSameParagraphTwiceIsRefused(t *testing.T) {
	_, public := boardWithGuard(t)

	send := func(title, body string) (int, map[string]any) {
		token := public.formToken()
		public.tick.advance(5 * time.Second)
		resp := public.do(http.MethodPost, boardAPI+"/items",
			map[string]any{"kind": "idea", "title": title, "body": body, "token": token})
		return resp.StatusCode, decode(t, resp)
	}

	if got, body := send("Buy cheap watches", "visit our site"); got != http.StatusCreated {
		t.Fatalf("the first one returned %d %v", got, body)
	}
	got, body := send("buy   CHEAP watches", "visit\nour site")
	if got == http.StatusCreated {
		t.Error("the same text with different case and wrapping got through a second time")
	}
	if body["error"] != "error.already_sent" {
		t.Errorf("refused with %v", body["error"])
	}
}

func TestAnAdvertIsRefused(t *testing.T) {
	_, public := boardWithGuard(t)
	token := public.formToken()
	public.tick.advance(5 * time.Second)

	resp := public.do(http.MethodPost, boardAPI+"/items", map[string]any{
		"kind": "idea", "title": "great deals",
		"body":  strings.Repeat("visit https://example.com now ", 6),
		"token": token,
	})
	body := decode(t, resp)
	if resp.StatusCode == http.StatusCreated || body["error"] != "error.too_many_links" {
		t.Errorf("six links returned %d %v", resp.StatusCode, body)
	}
}

// TestTheBreakerClosesTheBoard.
//
// The layer that protects attention rather than data: past a point, refusing
// for a while beats handing somebody a queue they will not open.
func TestTheBreakerClosesTheBoard(t *testing.T) {
	_, public := boardWithGuard(t)

	// Fill the hour directly: the point under test is the refusal, not the
	// thousand HTTP round trips it would take to get there.
	for i := 0; i < guard.HourlyLimit; i++ {
		if _, err := public.db().Exec(`INSERT INTO item
			(public_id, repo_id, kind, title, body, status, moderation_state, body_hash, follow_token_hash, created_at)
			VALUES (?, 1, 'idea', 'flood', '', 'new', 'pending', ?, 'x', ?)`,
			"flood-"+itoa(int64(i)), "hash-"+itoa(int64(i)), public.now().Unix()); err != nil {
			t.Fatal(err)
		}
	}

	form := decode(t, public.do(http.MethodGet, boardAPI+"/form", nil))
	if form["accepting"] != false {
		t.Error("the form still says the board is accepting after an hour's flood")
	}

	token, _ := form["token"].(string)
	public.tick.advance(5 * time.Second)
	resp := public.do(http.MethodPost, boardAPI+"/items",
		map[string]any{"kind": "idea", "title": "one more", "body": "one more", "token": token})
	body := decode(t, resp)
	if resp.StatusCode != http.StatusServiceUnavailable || body["error"] != "error.board_closed" {
		t.Errorf("submitting to a flooded board returned %d %v", resp.StatusCode, body)
	}
}

// TestRefusalsAreCounted.
//
// Without the count there is no way to know whether any of this is working. A
// layer that never refuses anything is a layer to delete; one that refuses
// nearly everything is turning away people who did nothing wrong — and those
// people do not complain, they leave.
func TestRefusalsAreCounted(t *testing.T) {
	_, public := boardWithGuard(t)

	resp := public.do(http.MethodPost, boardAPI+"/items",
		map[string]any{"kind": "idea", "title": "no token here"})
	_ = resp.Body.Close()

	var value string
	if err := public.db().QueryRow(`SELECT value FROM settings WHERE key LIKE 'refused.%.form_token'`).
		Scan(&value); err != nil {
		t.Fatalf("no refusal was counted: %v", err)
	}
	if value != "1" {
		t.Errorf("the counter says %q", value)
	}
}

// TestAGenuineSubmissionStillGetsThrough is the other half of every test above.
//
// Six layers is six chances to refuse somebody with a real problem, and the
// people a filter wrongly refuses are the ones who never say so.
func TestAGenuineSubmissionStillGetsThrough(t *testing.T) {
	_, public := boardWithGuard(t)
	token := public.formToken()
	public.tick.advance(12 * time.Second)

	resp := public.do(http.MethodPost, boardAPI+"/items", map[string]any{
		"kind":  "bug",
		"title": "crashes when I import a folder of photos",
		"body":  "It closes with no message. Log here: https://paste.example.com/abc",
		"email": "someone@example.com",
		"token": token,
	})
	body := decode(t, resp)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("an ordinary bug report was refused: %d %v", resp.StatusCode, body)
	}
	if body["follow_token"] == "" {
		t.Error("no follow link came back")
	}
}
