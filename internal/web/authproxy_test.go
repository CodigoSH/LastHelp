package web

import (
	"bytes"
	"net/http"
	"net/netip"
	"testing"

	"git.codigosh.com/CodigoSH/LastHelp/internal/core/account"
	"git.codigosh.com/CodigoSH/LastHelp/internal/platform/config"
	"git.codigosh.com/CodigoSH/LastHelp/internal/web/middleware"
)

// The test server is reached from 127.0.0.1, so that is "the proxy".
func behindProxy(trusted string, logout string) func(*config.Config) {
	return func(c *config.Config) {
		c.AuthProxy = config.AuthProxy{
			Header:    "Remote-User",
			Trusted:   []netip.Prefix{netip.MustParsePrefix(trusted)},
			LogoutURL: logout,
		}
	}
}

// asProxied sends a request as the proxy forwards it: with the header naming somebody.
func (i *instance) asProxied(method, path, name string) *http.Response {
	i.t.Helper()
	req, err := http.NewRequest(method, i.server.URL+path, bytes.NewReader(nil))
	if err != nil {
		i.t.Fatal(err)
	}
	req.Header.Set("Remote-User", name)
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	if token := i.csrf(); token != "" {
		req.Header.Set(middleware.CSRFHeader, token)
	}
	resp, err := i.client.Do(req)
	if err != nil {
		i.t.Fatal(err)
	}
	return resp
}

func meOf(t *testing.T, c *instance, name string) (int, string) {
	t.Helper()
	resp := c.asProxied(http.MethodGet, "/api/me", name)
	if resp.StatusCode != http.StatusOK {
		_ = resp.Body.Close()
		return resp.StatusCode, ""
	}
	who, _ := decode(t, resp)["username"].(string)
	return http.StatusOK, who
}

// The proxy's word opens a session for an account that exists, with nothing typed.
func TestTheProxySignsInAnAccountThatExists(t *testing.T) {
	i := newInstanceWith(t, behindProxy("127.0.0.1/32", ""))
	i.claim()
	browser := i.anonymous()
	if code, who := meOf(t, browser, "owner"); code != http.StatusOK || who != "owner" {
		t.Fatalf("signed in through the proxy: %d %q", code, who)
	}
	// It is an ordinary session: the next request needs no header.
	if got := statusOf(browser, http.MethodGet, "/api/me", nil); got != http.StatusOK {
		t.Errorf("the session the proxy opened did not last: %d", got)
	}
	var sessions int
	if err := i.db().QueryRow(`SELECT COUNT(*) FROM session`).Scan(&sessions); err != nil || sessions < 2 {
		t.Errorf("no stored session for it: %d %v", sessions, err)
	}
}

// From anywhere but the proxy's addresses the header is just something a caller typed.
func TestTheHeaderCountsOnlyFromTheProxy(t *testing.T) {
	i := newInstanceWith(t, behindProxy("10.0.0.1/32", ""))
	i.claim()
	if code, _ := meOf(t, i.anonymous(), "owner"); code != http.StatusUnauthorized {
		t.Errorf("the header from an address that is not the proxy: %d", code)
	}
}

// It never makes an account, and a disabled one stays out.
func TestTheProxyMakesNoAccountAndOpensNoDisabledOne(t *testing.T) {
	i := newInstanceWith(t, behindProxy("127.0.0.1/32", ""))
	i.claim()
	if code, _ := meOf(t, i.anonymous(), "stranger"); code != http.StatusUnauthorized {
		t.Errorf("a name with no account: %d", code)
	}
	var n int
	if err := i.db().QueryRow(`SELECT COUNT(*) FROM app_user WHERE username = 'stranger'`).Scan(&n); err != nil || n != 0 {
		t.Errorf("an account was made for the proxy's name: %d %v", n, err)
	}
	accounts := account.Store{DB: i.db()}
	member, err := accounts.Create("member", ownerPassword, false, i.now())
	if err != nil {
		t.Fatal(err)
	}
	if err := accounts.SetDisabled(member.ID, true); err != nil {
		t.Fatal(err)
	}
	if code, _ := meOf(t, i.anonymous(), "member"); code != http.StatusUnauthorized {
		t.Errorf("a disabled account: %d", code)
	}
	// Not signed in, and not a session waiting either.
	if err := i.db().QueryRow(`SELECT COUNT(*) FROM session WHERE user_id = ?`, member.ID).Scan(&n); err != nil || n != 0 {
		t.Errorf("a session was opened for a disabled account: %d %v", n, err)
	}
}

// A write never opens a session: its token belongs to the session it came with.
func TestAWriteOpensNoSession(t *testing.T) {
	i := newInstanceWith(t, behindProxy("127.0.0.1/32", ""))
	i.claim()
	browser := i.anonymous()
	_ = statusOf(browser, http.MethodGet, "/api/status", nil) // the CSRF cookie, as a page load gives it
	resp := browser.asProxied(http.MethodPost, "/api/me/two-step/setup", "owner")
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("a write through the proxy with no session: %d", resp.StatusCode)
	}
}

// When the proxy names somebody else, the browser's old session is not used.
func TestTheProxysNameWinsOverAnotherSession(t *testing.T) {
	i := newInstanceWith(t, behindProxy("127.0.0.1/32", ""))
	i.claim()
	if _, err := (account.Store{DB: i.db()}).Create("member", ownerPassword, false, i.now()); err != nil {
		t.Fatal(err)
	}
	browser := i.anonymous()
	if code, who := meOf(t, browser, "member"); code != http.StatusOK || who != "member" {
		t.Fatalf("first as member: %d %q", code, who)
	}
	if code, who := meOf(t, browser, "owner"); code != http.StatusOK || who != "owner" {
		t.Errorf("the proxy now names owner, the page says %d %q", code, who)
	}
}

// Signing out sends the browser on to the proxy's own sign-out, when there is one.
func TestSigningOutGoesOnToTheProxy(t *testing.T) {
	i := newInstanceWith(t, behindProxy("127.0.0.1/32", "https://auth.example.com/logout"))
	i.claim()
	browser := i.anonymous()
	_, _ = meOf(t, browser, "owner")
	resp := browser.asProxied(http.MethodDelete, "/api/session", "owner")
	if resp.StatusCode != http.StatusOK || decode(t, resp)["logout_url"] != "https://auth.example.com/logout" {
		t.Errorf("sign-out through the proxy: %d", resp.StatusCode)
	}

	plain := newInstance(t)
	plain.claim()
	if got := statusOf(plain, http.MethodDelete, "/api/session", nil); got != http.StatusNoContent {
		t.Errorf("sign-out with no proxy: %d", got)
	}
}
