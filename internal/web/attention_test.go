package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"git.codigosh.com/CodigoSH/LastHelp/internal/core/outbox"
	"git.codigosh.com/CodigoSH/LastHelp/internal/platform/config"
)

func (i *instance) attentionItems() []attentionItem {
	i.t.Helper()
	resp := i.do(http.MethodGet, "/api/attention", nil)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		i.t.Fatalf("GET /api/attention = %d", resp.StatusCode)
	}
	var out struct{ Items []attentionItem }
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		i.t.Fatal(err)
	}
	return out.Items
}

func (i *instance) attentionHas(key string) bool {
	i.t.Helper()
	for _, it := range i.attentionItems() {
		if it.Key == key {
			return true
		}
	}
	return false
}

// What the server gave up sending is listed, with the last reason given, so
// the administrator knows which credential or channel to look at.
func TestAttentionListsWhatTheServerGaveUpOn(t *testing.T) {
	i := newInstance(t)
	i.claim()
	if i.attentionHas("jobs") {
		t.Fatal("jobs listed with nothing abandoned")
	}
	if _, err := i.db().Exec(`INSERT INTO outbox (kind, payload, attempts, next_attempt_at, last_error, created_at)
		VALUES ('issue', '{}', ?, 0, 'gave up: 401 Unauthorized', 0)`, outbox.MaxAttempts); err != nil {
		t.Fatal(err)
	}
	for _, it := range i.attentionItems() {
		if it.Key == "jobs" {
			if it.Params["count"] != float64(1) || it.Params["last_error"] != "gave up: 401 Unauthorized" {
				t.Errorf("jobs item says %+v", it.Params)
			}
			return
		}
	}
	t.Error("an abandoned job is not listed")
}

// Without a public address, an organisation with a forge gets issues with no
// link back; one without a forge is not affected and nothing is said.
func TestAttentionAsksForAPublicAddressOnlyWithAForge(t *testing.T) {
	i := newInstance(t)
	i.claim()
	if _, err := i.db().Exec(`INSERT INTO organization (slug, name, forge_kind, base_url, forge_owner, created_at)
		VALUES ('plain', 'Plain', 'none', '', '', 0)`); err != nil {
		t.Fatal(err)
	}
	if i.attentionHas("public_url") {
		t.Error("asked for a public address with no forge connected")
	}
	if _, err := i.db().Exec(`INSERT INTO organization (slug, name, forge_kind, base_url, forge_owner, created_at)
		VALUES ('acme', 'Acme', 'gitea', 'https://git.example.org', 'acme', 0)`); err != nil {
		t.Fatal(err)
	}
	if !i.attentionHas("public_url") {
		t.Error("an organisation with a forge and no public address is not flagged")
	}
}

// Plain HTTP is noticed from another machine, not from this one.
func TestPlainHTTPFromAnotherMachineIsNoticed(t *testing.T) {
	seen := func(peer string) bool {
		s := &Server{cfg: config.Config{}}
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.RemoteAddr = peer
		s.noticeInsecure(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})).ServeHTTP(httptest.NewRecorder(), r)
		return s.insecureSeen.Load()
	}
	if !seen("192.168.1.30:50000") {
		t.Error("plain HTTP from the LAN was not noticed")
	}
	if seen("127.0.0.1:50000") {
		t.Error("plain HTTP from this machine was noticed")
	}
}

// The list is the administrator's; anybody else is refused.
func TestAttentionIsTheAdministrators(t *testing.T) {
	i := newInstance(t)
	i.claim()
	grantOnBoard(t, i, "ana", "")
	ana := i.signInAs("ana")
	if got := status(t, ana.do(http.MethodGet, "/api/attention", nil)); got != http.StatusForbidden {
		t.Errorf("a team member reading the attention list: %d, want 403", got)
	}
}

// About's dialog gets how the server is set up, and never a secret.
func TestServerInfoSaysHowItIsSetUp(t *testing.T) {
	i := newInstance(t)
	i.claim()
	body := decode(t, i.do(http.MethodGet, "/api/server", nil))
	cfg, ok := body["config"].(map[string]any)
	if !ok {
		t.Fatalf("no config in %v", body)
	}
	for _, k := range []string{"trust_proxy", "proxy_hops", "public_url", "mail", "offline", "https"} {
		if _, ok := cfg[k]; !ok {
			t.Errorf("config has no %q", k)
		}
	}
	if _, leaked := body["proxy_unread"]; leaked {
		t.Error("the proxy notice is said in two places; it belongs to the attention list")
	}
}

// The disk is said at the lines its bar changes colour at, and not before.
func TestTheDiskIsSaidWhenItIsFilling(t *testing.T) {
	for _, c := range []struct {
		free, total uint64
		tone        string
	}{
		{30, 100, ""}, {26, 100, ""}, {25, 100, "warn"}, {11, 100, "warn"}, {10, 100, "bad"}, {0, 100, "bad"}, {0, 0, ""}, {5, 4, ""},
	} {
		item, said := diskAttention(c.free, c.total)
		switch {
		case c.tone == "" && said:
			t.Errorf("%d free of %d: said %q, want nothing", c.free, c.total, item.Tone)
		case c.tone != "" && (!said || item.Tone != c.tone):
			t.Errorf("%d free of %d: said=%v %q, want %q", c.free, c.total, said, item.Tone, c.tone)
		}
	}
}
