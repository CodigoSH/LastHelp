package web

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"git.codigosh.com/CodigoSH/LastHelp/internal/core/account"
)

type listedSession struct {
	ID        string `json:"id"`
	Current   bool   `json:"current"`
	UserAgent string `json:"user_agent"`
	IP        string `json:"ip"`
}

func (i *instance) sessionsList() []listedSession {
	i.t.Helper()
	var out struct {
		Sessions []listedSession `json:"sessions"`
	}
	resp := i.do(http.MethodGet, "/api/sessions", nil)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		i.t.Fatalf("listing sessions: %d", resp.StatusCode)
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		i.t.Fatal(err)
	}
	return out.Sessions
}

// currentOf is the id of the session the client is using, as its own list says.
func currentOf(t *testing.T, list []listedSession) string {
	t.Helper()
	id := ""
	for _, s := range list {
		if s.Current {
			if id != "" {
				t.Fatalf("two sessions are marked current: %+v", list)
			}
			id = s.ID
		}
	}
	if id == "" {
		t.Fatalf("no session is marked current: %+v", list)
	}
	return id
}

// An account sees each browser it is signed in on, with what the browser and
// the network said, and which one is asking.
func TestTheAccountSeesWhereItIsSignedIn(t *testing.T) {
	i := newInstance(t)
	i.claim()
	phone := i.signInAs("owner")

	list := phone.sessionsList()
	if len(list) != 2 {
		t.Fatalf("listed %d sessions, want 2: %+v", len(list), list)
	}
	here := currentOf(t, list)
	var mine listedSession
	for _, s := range list {
		if s.ID == here {
			mine = s
		}
	}
	if mine.UserAgent != "Go-http-client/1.1" || mine.IP != "127.0.0.1" {
		t.Errorf("the session signed in through the route says %+v, want the client's browser and address", mine)
	}
	if other := currentOf(t, i.sessionsList()); other == here {
		t.Error("both clients are told they are the same session")
	}
}

// A session that does not know its browser — one opened before the list
// existed — learns it on its next use, not an hour later; and after that it
// follows a new address once an hour.
func TestASessionLearnsItsBrowserWhenNextUsed(t *testing.T) {
	i := newInstance(t)
	i.claim() // written without a browser or an address, as a session from before is
	if s := i.sessionsList()[0]; s.UserAgent != "Go-http-client/1.1" || s.IP != "127.0.0.1" {
		t.Errorf("on its first use the session says %+v, want the client's browser and address", s)
	}
	if _, err := i.db().Exec(`UPDATE session SET ip = '192.0.2.1'`); err != nil {
		t.Fatal(err)
	}
	_ = i.sessionsList()
	if s := i.sessionsList()[0]; s.IP != "192.0.2.1" {
		t.Errorf("within the hour the address became %q, want it left alone", s.IP)
	}
	i.tick.advance(2 * time.Hour)
	if s := i.sessionsList()[0]; s.IP != "127.0.0.1" {
		t.Errorf("after an hour the address is %q, want the one it came from", s.IP)
	}
}

// Closing one of the others signs that browser out, and nobody else; the one
// asking is not closed this way, and another account's is not found.
func TestClosingASessionSignsThatBrowserOut(t *testing.T) {
	i := newInstance(t)
	i.claim()
	phone := i.signInAs("owner")
	phoneID := currentOf(t, phone.sessionsList())
	here := currentOf(t, i.sessionsList())

	if _, err := (account.Store{DB: i.db()}).Create("ana", ownerPassword, false, i.now()); err != nil {
		t.Fatal(err)
	}
	ana := i.signInAs("ana")
	if got := status(t, ana.do(http.MethodDelete, "/api/sessions/"+phoneID, nil)); got != http.StatusNotFound {
		t.Errorf("another account closing the owner's session: %d, want 404", got)
	}
	if got := status(t, phone.do(http.MethodGet, "/api/me", nil)); got != http.StatusOK {
		t.Fatalf("the phone after another account tried to close it: %d", got)
	}
	if got := status(t, i.do(http.MethodDelete, "/api/sessions/"+here, nil)); got != http.StatusBadRequest {
		t.Errorf("closing the session asking: %d, want 400", got)
	}

	if got := status(t, i.do(http.MethodDelete, "/api/sessions/"+phoneID, nil)); got != http.StatusOK {
		t.Fatalf("closing the phone: %d", got)
	}
	if got := status(t, phone.do(http.MethodGet, "/api/me", nil)); got != http.StatusUnauthorized {
		t.Errorf("the phone after being closed: %d, want 401", got)
	}
	for who, c := range map[string]*instance{"this browser": i, "ana": ana} {
		if got := status(t, c.do(http.MethodGet, "/api/me", nil)); got != http.StatusOK {
			t.Errorf("%s after the phone was closed: %d", who, got)
		}
	}
}

// "Close the others" signs out every other browser of the account and keeps
// this one, and other accounts, signed in.
func TestClosingTheOtherSessions(t *testing.T) {
	i := newInstance(t)
	i.claim()
	phone := i.signInAs("owner")
	laptop := i.signInAs("owner")
	if _, err := (account.Store{DB: i.db()}).Create("ana", ownerPassword, false, i.now()); err != nil {
		t.Fatal(err)
	}
	ana := i.signInAs("ana")

	resp := i.do(http.MethodPost, "/api/sessions/close-others", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("closing the others: %d", resp.StatusCode)
	}
	if got := decode(t, resp)["closed"]; got != float64(2) {
		t.Errorf("closed %v, want 2", got)
	}
	for who, c := range map[string]*instance{"the phone": phone, "the laptop": laptop} {
		if got := status(t, c.do(http.MethodGet, "/api/me", nil)); got != http.StatusUnauthorized {
			t.Errorf("%s after closing the others: %d, want 401", who, got)
		}
	}
	for who, c := range map[string]*instance{"this browser": i, "ana": ana} {
		if got := status(t, c.do(http.MethodGet, "/api/me", nil)); got != http.StatusOK {
			t.Errorf("%s after closing the others: %d", who, got)
		}
	}
	if n := len(i.sessionsList()); n != 1 {
		t.Errorf("%d sessions are left, want this one", n)
	}
}

// Nobody who is not signed in can list or close anything.
func TestSessionsAreForTheSignedIn(t *testing.T) {
	i := newInstance(t)
	i.claim()
	public := i.anonymous()
	for _, r := range []struct{ method, path string }{
		{http.MethodGet, "/api/sessions"},
		{http.MethodDelete, "/api/sessions/0123456789abcdef"},
		{http.MethodPost, "/api/sessions/close-others"},
	} {
		if got := status(t, public.do(r.method, r.path, nil)); got != http.StatusUnauthorized {
			t.Errorf("%s %s without a session: %d, want 401", r.method, r.path, got)
		}
	}
}
