package web

import (
	"net/http"
	"testing"
)

// TestAWrittenMessageIsQueuedForWhoHasToKnow.
//
// A comment waits for somebody to read it and a reply is something the reporter
// asked to hear about; both used to leave no trace in the queue, so nobody was
// told. The handlers only queue, and this reads the queue: delivery has its own
// tests in the bridge.
func TestAWrittenMessageIsQueuedForWhoHasToKnow(t *testing.T) {
	team := newInstance(t)
	team.claim()
	team.createOrg("codigosh")
	team.createRepo("codigosh", "lastboard")
	published, _ := team.anonymous().submit("codigosh", "lastboard", "idea", "dark mode")
	_ = status(t, team.do(http.MethodPost, "/api/items/"+published+"/moderation", map[string]string{"state": "published"}))

	count := func(kind, what string) int {
		t.Helper()
		var n int
		if err := team.db().QueryRow(`SELECT count(*) FROM outbox WHERE kind = ? AND payload LIKE ?`,
			kind, "%"+what+"%").Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}

	if got := status(t, team.anonymous().do(http.MethodPost, "/api/items/"+published+"/comments",
		map[string]string{"body": "same here"})); got != http.StatusAccepted {
		t.Fatalf("a visitor's comment answered %d", got)
	}
	if got := count("announce", "comment waiting for review"); got != 1 {
		t.Errorf("a comment waiting for review queued %d announcements, want 1", got)
	}

	if got := status(t, team.do(http.MethodPost, "/api/items/"+published+"/reply",
		map[string]string{"body": "looking at it"})); got != http.StatusCreated {
		t.Fatalf("a reply answered %d", got)
	}
	if got := count("tell_reporter", "has a new reply"); got != 1 {
		t.Errorf("a team reply queued %d reporter notices, want 1", got)
	}
	if got := count("announce", "comment waiting for review"); got != 1 {
		t.Errorf("a team reply announced itself to the team: %d announcements, want still 1", got)
	}
}
