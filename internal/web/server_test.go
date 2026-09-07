package web

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"git.codigosh.com/CodigoSH/LastHelp/internal/platform/config"
	"git.codigosh.com/CodigoSH/LastHelp/internal/platform/database"
	"git.codigosh.com/CodigoSH/LastHelp/internal/web/middleware"
)

// instance is a whole server on a real socket with its own database, which is
// the only way to check the things that only exist at this level: cookies,
// headers, and whether a route is actually behind the door.
type instance struct {
	t        *testing.T
	server   *httptest.Server
	client   *http.Client
	dbHandle *sql.DB
	clock    func() time.Time
}

func newInstance(t *testing.T) *instance {
	t.Helper()

	db, err := database.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	srv, err := New(config.Config{}, db)
	if err != nil {
		t.Fatal(err)
	}

	httpSrv := httptest.NewServer(srv)
	t.Cleanup(httpSrv.Close)

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	return &instance{
		t:        t,
		server:   httpSrv,
		client:   &http.Client{Jar: jar},
		dbHandle: db,
		clock:    srv.now,
	}
}

// db and now let a test set up state the API has no route for yet — granting
// somebody a role, for instance — without reaching for a second database
// connection with its own view of the file.
func (i *instance) db() *sql.DB    { return i.dbHandle }
func (i *instance) now() time.Time { return i.clock() }

// do sends a request the way the page would: same-origin, with the CSRF token
// echoed back from the cookie.
func (i *instance) do(method, path string, body any) *http.Response {
	i.t.Helper()

	var payload []byte
	if body != nil {
		var err error
		payload, err = json.Marshal(body)
		if err != nil {
			i.t.Fatal(err)
		}
	}

	req, err := http.NewRequest(method, i.server.URL+path, bytes.NewReader(payload))
	if err != nil {
		i.t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
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

func (i *instance) csrf() string {
	u, _ := http.NewRequest(http.MethodGet, i.server.URL, nil)
	for _, c := range i.client.Jar.Cookies(u.URL) {
		if c.Name == middleware.CSRFCookie {
			return c.Value
		}
	}
	// The first request has no cookie yet; a GET is what mints one.
	resp, err := i.client.Get(i.server.URL + "/api/status")
	if err != nil {
		i.t.Fatal(err)
	}
	_ = resp.Body.Close()
	for _, c := range i.client.Jar.Cookies(u.URL) {
		if c.Name == middleware.CSRFCookie {
			return c.Value
		}
	}
	return ""
}

func (i *instance) claim() {
	i.t.Helper()
	resp := i.do(http.MethodPost, "/api/setup", map[string]string{
		"email": "owner@example.com", "password": "a good long passphrase", "display_name": "Owner",
	})
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusCreated {
		i.t.Fatalf("setup returned %d", resp.StatusCode)
	}
}

func decode(t *testing.T, resp *http.Response) map[string]any {
	t.Helper()
	defer func() { _ = resp.Body.Close() }()
	var out map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decoding the response: %v", err)
	}
	return out
}

func TestAFreshInstanceAsksToBeSetUp(t *testing.T) {
	i := newInstance(t)

	resp, err := i.client.Get(i.server.URL + "/api/status")
	if err != nil {
		t.Fatal(err)
	}
	if got := decode(t, resp)["needs_setup"]; got != true {
		t.Errorf("needs_setup is %v on a fresh instance", got)
	}

	i.claim()

	resp, err = i.client.Get(i.server.URL + "/api/status")
	if err != nil {
		t.Fatal(err)
	}
	if got := decode(t, resp)["needs_setup"]; got != false {
		t.Errorf("needs_setup is %v after setup", got)
	}
}

// TestSetupCanOnlyHappenOnce.
//
// Without this, anybody who finds the instance before its owner does becomes
// its administrator.
func TestSetupCanOnlyHappenOnce(t *testing.T) {
	i := newInstance(t)
	i.claim()

	resp := i.do(http.MethodPost, "/api/setup", map[string]string{
		"email": "someone@example.com", "password": "another long passphrase",
	})
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("a second setup returned %d, want 403", resp.StatusCode)
	}
}

// TestThePrivateRoutesAreActuallyPrivate.
//
// Being wrapped in RequireActor is the only thing that protects a route, and
// this is that claim checked from outside rather than read in the routing table.
func TestThePrivateRoutesAreActuallyPrivate(t *testing.T) {
	i := newInstance(t)
	i.claim()

	// A client with no cookies at all.
	stranger := &http.Client{}
	for _, route := range []struct{ method, path string }{
		{http.MethodGet, "/api/me"},
		{http.MethodDelete, "/api/session"},
	} {
		req, err := http.NewRequest(route.method, i.server.URL+route.path, nil)
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Sec-Fetch-Site", "same-origin")
		resp, err := stranger.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()

		// 401 for the GET; the DELETE is refused earlier, by CSRF, which is
		// also a refusal. What must never happen is 2xx.
		if resp.StatusCode < 400 {
			t.Errorf("%s %s answered %d to somebody with no session",
				route.method, route.path, resp.StatusCode)
		}
	}
}

func TestSigningInAndOut(t *testing.T) {
	i := newInstance(t)
	i.claim()

	// Setup already signed us in.
	resp := i.do(http.MethodGet, "/api/me", nil)
	me := decode(t, resp)
	if me["admin"] != true {
		t.Fatalf("the first account is not an administrator: %v", me)
	}

	resp = i.do(http.MethodDelete, "/api/session", nil)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("signing out returned %d", resp.StatusCode)
	}

	resp = i.do(http.MethodGet, "/api/me", nil)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("still signed in after signing out: %d", resp.StatusCode)
	}

	resp = i.do(http.MethodPost, "/api/session", map[string]string{
		"email": "owner@example.com", "password": "a good long passphrase",
	})
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("signing back in returned %d", resp.StatusCode)
	}
}

func TestAWrongPasswordAndAnUnknownAddressAnswerTheSame(t *testing.T) {
	i := newInstance(t)
	i.claim()

	wrong := i.do(http.MethodPost, "/api/session", map[string]string{
		"email": "owner@example.com", "password": "not the passphrase",
	})
	unknown := i.do(http.MethodPost, "/api/session", map[string]string{
		"email": "nobody@example.com", "password": "not the passphrase",
	})

	wrongBody := decode(t, wrong)
	unknownBody := decode(t, unknown)
	if wrong.StatusCode != unknown.StatusCode || wrongBody["error"] != unknownBody["error"] {
		t.Errorf("the two answers differ: %d/%v vs %d/%v — that difference tells somebody which addresses have accounts",
			wrong.StatusCode, wrongBody, unknown.StatusCode, unknownBody)
	}
}

// TestAWriteWithoutTheCSRFTokenIsRefused.
//
// The cookie rides along on any request the browser makes. The header does not:
// only a page on this origin can read the token and echo it back.
func TestAWriteWithoutTheCSRFTokenIsRefused(t *testing.T) {
	i := newInstance(t)
	i.claim()

	req, err := http.NewRequest(http.MethodDelete, i.server.URL+"/api/session", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Sec-Fetch-Site", "same-origin") // the header is what is missing
	resp, err := i.client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()

	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("a write with the session cookie but no CSRF token returned %d, want 403", resp.StatusCode)
	}
}

// TestAWriteFromAnotherSiteIsRefusedBeforeAnythingElse.
//
// Sec-Fetch-Site is written by the browser and a page cannot forge it, so a
// cross-site write is refused before any token is even looked at.
func TestAWriteFromAnotherSiteIsRefusedBeforeAnythingElse(t *testing.T) {
	i := newInstance(t)
	i.claim()

	req, err := http.NewRequest(http.MethodDelete, i.server.URL+"/api/session", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Sec-Fetch-Site", "cross-site")
	req.Header.Set(middleware.CSRFHeader, i.csrf()) // even with a valid token
	resp, err := i.client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()

	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("a cross-site write returned %d, want 403", resp.StatusCode)
	}
}

func TestTheSessionCookieIsNotReadableByScripts(t *testing.T) {
	i := newInstance(t)
	i.claim()

	resp := i.do(http.MethodPost, "/api/session", map[string]string{
		"email": "owner@example.com", "password": "a good long passphrase",
	})
	defer func() { _ = resp.Body.Close() }()

	for _, c := range resp.Cookies() {
		if c.Name != middleware.SessionCookie {
			continue
		}
		if !c.HttpOnly {
			t.Error("the session cookie is not HttpOnly: any injected script can read it")
		}
		if c.SameSite != http.SameSiteLaxMode && c.SameSite != http.SameSiteStrictMode {
			t.Errorf("the session cookie has SameSite %v: it would ride along on cross-site requests", c.SameSite)
		}
		return
	}
	t.Fatal("no session cookie was set")
}

// TestAShortPasswordIsRefused keeps the floor from quietly disappearing.
func TestAShortPasswordIsRefused(t *testing.T) {
	i := newInstance(t)
	resp := i.do(http.MethodPost, "/api/setup", map[string]string{
		"email": "owner@example.com", "password": "short",
	})
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("a five-character password was accepted with %d", resp.StatusCode)
	}
}

// TestAnUnknownAPIPathIs404AndNotTheApplicationShell.
//
// The static handler falls through to index.html so the frontend can route
// client-side. If that fall-through covered /api/ too, every unknown API path
// would answer 200 with the shell — which is how this house once served the
// application to people with no session at all.
func TestAnUnknownAPIPathIs404AndNotTheApplicationShell(t *testing.T) {
	i := newInstance(t)

	resp, err := i.client.Get(i.server.URL + "/api/does-not-exist")
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("an unknown API path returned %d, want 404", resp.StatusCode)
	}
}

func TestTheHealthCheckStaysPublic(t *testing.T) {
	i := newInstance(t)
	resp, err := i.client.Get(i.server.URL + "/healthz")
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("the health check answered %d; a container would decide the server is dead", resp.StatusCode)
	}
}
