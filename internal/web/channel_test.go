package web

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// receiver is a notification service that remembers what reached it.
type receiver struct {
	srv  *httptest.Server
	mu   sync.Mutex
	auth []string
	hits int
}

func newReceiver(t *testing.T, code int) *receiver {
	t.Helper()
	r := &receiver{}
	r.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		_, _ = io.Copy(io.Discard, req.Body)
		r.mu.Lock()
		r.hits++
		r.auth = append(r.auth, req.Header.Get("Authorization"))
		r.mu.Unlock()
		w.WriteHeader(code)
	}))
	t.Cleanup(r.srv.Close)
	return r
}

func (r *receiver) seen() (int, []string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.hits, append([]string(nil), r.auth...)
}

func body(t *testing.T, resp *http.Response) string {
	t.Helper()
	raw, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	return string(raw)
}

// TestSavingAChannelWithoutATokenKeepsTheOneStored: the form shows the address
// again and never the token, so saving it back has to leave the token alone.
func TestSavingAChannelWithoutATokenKeepsTheOneStored(t *testing.T) {
	admin, _, _ := teamSetup(t)
	const path = "/api/orgs/acme/repos/atlas/channel"

	admin.do(http.MethodPost, path, map[string]string{"kind": "ntfy", "url": "https://ntfy.example/a", "token": "tk_kept"}).Body.Close()
	if got := status(t, admin.do(http.MethodPost, path, map[string]string{"kind": "ntfy", "url": "https://ntfy.example/b"})); got != http.StatusOK {
		t.Fatalf("saving without a token: %d", got)
	}
	panel := body(t, admin.do(http.MethodGet, "/api/orgs/acme/repos/atlas/panel", nil))
	if !strings.Contains(panel, `"channel_has_token":true`) || !strings.Contains(panel, "ntfy.example/b") {
		t.Errorf("the token was lost or the address not saved: %s", panel)
	}

	// Asking for it to go removes it.
	admin.do(http.MethodPost, path, map[string]any{"kind": "ntfy", "url": "https://ntfy.example/b", "clear_token": true}).Body.Close()
	panel = body(t, admin.do(http.MethodGet, "/api/orgs/acme/repos/atlas/panel", nil))
	if !strings.Contains(panel, `"channel_has_token":false`) {
		t.Errorf("the token was not removed: %s", panel)
	}

	// Another kind never inherits the token of the one before.
	admin.do(http.MethodPost, path, map[string]string{"kind": "gotify", "url": "https://g.example", "token": "tk_gotify"}).Body.Close()
	admin.do(http.MethodPost, path, map[string]string{"kind": "ntfy", "url": "https://ntfy.example/c"}).Body.Close()
	panel = body(t, admin.do(http.MethodGet, "/api/orgs/acme/repos/atlas/panel", nil))
	if !strings.Contains(panel, `"channel_has_token":false`) {
		t.Errorf("a token crossed from one kind of channel to another: %s", panel)
	}
}

// TestTheTestButtonSendsWithTheStoredToken, and says why when it cannot.
func TestTheTestButtonSendsWithTheStoredToken(t *testing.T) {
	admin, _, collaborator := teamSetup(t)
	const path = "/api/orgs/acme/repos/atlas/channel"
	ok := newReceiver(t, http.StatusOK)

	admin.do(http.MethodPost, path, map[string]string{"kind": "ntfy", "url": ok.srv.URL, "token": "tk_stored"}).Body.Close()
	// The form is sent as it was shown: with the address and no token.
	resp := admin.do(http.MethodPost, path+"/test", map[string]string{"kind": "ntfy", "url": ok.srv.URL})
	if got := resp.StatusCode; got != http.StatusOK {
		t.Fatalf("the test did not go out: %d %s", got, body(t, resp))
	}
	hits, auth := ok.seen()
	if hits != 1 || len(auth) != 1 || !strings.Contains(auth[0], "tk_stored") {
		t.Errorf("the test did not carry the stored token: hits=%d auth=%v", hits, auth)
	}

	if got := status(t, collaborator.do(http.MethodPost, path+"/test", map[string]string{"kind": "ntfy", "url": ok.srv.URL})); got != http.StatusForbidden {
		t.Errorf("a collaborator sent a test: %d", got)
	}

	denied := newReceiver(t, http.StatusUnauthorized)
	resp = admin.do(http.MethodPost, path+"/test", map[string]string{"kind": "ntfy", "url": denied.srv.URL})
	if resp.StatusCode == http.StatusOK || !strings.Contains(body(t, resp), "error.channel_denied") {
		t.Errorf("a refused token did not say it was refused")
	}
}

// TestTheServerChannelIsTheAdministratorsAndABoardChoosesIt.
func TestTheServerChannelIsTheAdministratorsAndABoardChoosesIt(t *testing.T) {
	admin, owner, _ := teamSetup(t)
	recv := newReceiver(t, http.StatusOK)

	for _, who := range []*instance{owner} {
		if got := status(t, who.do(http.MethodGet, "/api/notifications", nil)); got != http.StatusForbidden {
			t.Errorf("a non-administrator read the server's channel: %d", got)
		}
		if got := status(t, who.do(http.MethodPut, "/api/notifications", map[string]string{"kind": "ntfy", "url": recv.srv.URL})); got != http.StatusForbidden {
			t.Errorf("a non-administrator set the server's channel: %d", got)
		}
	}

	// A board cannot pick it while there is none.
	if got := status(t, admin.do(http.MethodPost, "/api/orgs/acme/repos/atlas/channel/test", map[string]string{"kind": "server"})); got != http.StatusUnprocessableEntity {
		t.Errorf("a test through an unset server channel: %d", got)
	}

	if got := status(t, admin.do(http.MethodPut, "/api/notifications", map[string]string{"kind": "ntfy", "url": recv.srv.URL, "token": "tk_server"})); got != http.StatusOK {
		t.Fatalf("setting the server's channel: %d", got)
	}
	got := body(t, admin.do(http.MethodGet, "/api/notifications", nil))
	if strings.Contains(got, "tk_server") || !strings.Contains(got, `"channel_has_token":true`) || !strings.Contains(got, recv.srv.URL) {
		t.Errorf("the server's channel read back wrongly: %s", got)
	}
	// It cannot be told to be itself.
	if got := status(t, admin.do(http.MethodPut, "/api/notifications", map[string]string{"kind": "server"})); got == http.StatusOK {
		t.Errorf("the server's channel was set to itself")
	}

	// The board that chose it sends through it, with the server's token.
	admin.do(http.MethodPost, "/api/orgs/acme/repos/atlas/channel", map[string]string{"kind": "server"}).Body.Close()
	resp := admin.do(http.MethodPost, "/api/orgs/acme/repos/atlas/channel/test", map[string]string{"kind": "server"})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("the test through the server's channel: %d %s", resp.StatusCode, body(t, resp))
	}
	hits, auth := recv.seen()
	if hits != 1 || !strings.Contains(auth[0], "tk_server") {
		t.Errorf("the board did not go out through the server's channel: hits=%d auth=%v", hits, auth)
	}
	panel := body(t, admin.do(http.MethodGet, "/api/orgs/acme/repos/atlas/panel", nil))
	if !strings.Contains(panel, `"configured":true`) {
		t.Errorf("the page does not say the server has a channel: %s", panel)
	}
}
