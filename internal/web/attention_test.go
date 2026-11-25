package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"git.codigosh.com/CodigoSH/LastHelp/internal/core/outbox"
	"git.codigosh.com/CodigoSH/LastHelp/internal/web/middleware"
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

// sessionOf is the cookie this client was signed in with.
func (i *instance) sessionOf() *http.Cookie {
	i.t.Helper()
	u, _ := http.NewRequest(http.MethodGet, i.server.URL, nil)
	for _, c := range i.client.Jar.Cookies(u.URL) {
		if c.Name == middleware.SessionCookie {
			return c
		}
	}
	i.t.Fatal("this client is not signed in")
	return nil
}

// Plain HTTP is noticed when somebody signed in uses the server so from another machine. Not from
// this one, and not a request nobody is behind (a proxy's health probe, a check of the server by its
// own address), which an administrator on HTTPS was being told about.
func TestPlainHTTPIsNoticedWhenSomebodySignedInUsesIt(t *testing.T) {
	i, srv := newPushInstance(t)
	i.claim()
	session := i.sessionOf()
	request := func(peer string, signedIn bool, forwardedProto string) {
		r := httptest.NewRequest(http.MethodGet, "/api/status", nil)
		r.RemoteAddr = peer
		r.Header.Set("Sec-Fetch-Site", "same-origin")
		if signedIn {
			r.AddCookie(session)
		}
		if forwardedProto != "" {
			r.Header.Set("X-Forwarded-Proto", forwardedProto)
		}
		srv.ServeHTTP(httptest.NewRecorder(), r)
	}

	request("192.168.1.30:50000", false, "")
	if srv.insecureSeen.Load() {
		t.Fatal("a request with nobody signed in behind it was noticed as somebody using plain HTTP")
	}
	request("127.0.0.1:50000", true, "")
	if srv.insecureSeen.Load() {
		t.Fatal("somebody signed in from this machine was noticed")
	}

	// Behind a proxy that is declared, its word is what the visitor used: https, wss on a WebSocket
	// (Traefik writes that one), and the first of a list.
	srv.cfg.TrustProxy = true
	for _, proto := range []string{"https", "wss", "https, http"} {
		request("192.168.1.30:50000", true, proto)
		if srv.insecureSeen.Load() {
			t.Fatalf("somebody signed in over HTTPS through a proxy (%q) was noticed as using plain HTTP", proto)
		}
	}

	request("192.168.1.30:50000", true, "http")
	if !srv.insecureSeen.Load() {
		t.Error("somebody signed in on plain HTTP from the LAN was not noticed")
	}
}

// While a proxy has announced itself and nobody declared it, the server cannot tell what the visitor
// used, and it does not say they used nothing: the notice of the proxy says what to do, and the other
// is judged again once it is done.
func TestPlainHTTPIsNotSaidWhileAnUndeclaredProxyIsInFront(t *testing.T) {
	i, srv := newPushInstance(t)
	i.claim()
	has := func() bool {
		for _, it := range srv.attention() {
			if it.Key == "https" {
				return true
			}
		}
		return false
	}
	srv.insecureSeen.Store(true)
	if !has() {
		t.Fatal("plain HTTP, with no proxy in the picture, is not said")
	}
	srv.forwardedUnread.Store(true)
	if has() {
		t.Error("plain HTTP is said although a proxy in front was noticed and not declared")
	}
	srv.cfg.TrustProxy = true
	if !has() {
		t.Error("plain HTTP is not said once the proxy is declared")
	}
}

// What the administrator is told to do about a setting depends on how LastHelp is installed, so the
// server says which it is.
func TestTheProxyNoticeSaysHowLastHelpIsInstalled(t *testing.T) {
	i, srv := newPushInstance(t)
	i.claim()
	srv.forwardedUnread.Store(true)
	var proxy *attentionItem
	for _, it := range srv.attention() {
		if it.Key == "proxy" {
			it := it
			proxy = &it
		}
	}
	if proxy == nil {
		t.Fatal("an undeclared proxy is not listed")
	}
	if got := proxy.Params["install"]; got != installKind() || (got != "container" && got != "native") {
		t.Errorf("the proxy item says install=%v, this process is %q", got, installKind())
	}
}

// A proxy writes the word of the connection it received, and it is not always "https".
func TestTheProxysProtocolIsReadForWhatItSays(t *testing.T) {
	for value, want := range map[string]bool{
		"https": true, "wss": true, "HTTPS": true, "https, http": true, " wss ,http": true,
		"http": false, "ws": false, "http, https": false, "": false, "javascript": false,
	} {
		if got := secureProto(value); got != want {
			t.Errorf("%q read as secure=%v, want %v", value, got, want)
		}
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
