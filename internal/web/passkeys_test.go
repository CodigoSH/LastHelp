package web

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"git.codigosh.com/CodigoSH/LastHelp/internal/core/account"
	"git.codigosh.com/CodigoSH/LastHelp/internal/core/webauthn"
	"git.codigosh.com/CodigoSH/LastHelp/internal/core/webauthn/softkey"
	"git.codigosh.com/CodigoSH/LastHelp/internal/web/middleware"
)

// A passkey belongs to a name, and the test server listens on an IP address:
// these requests say they were made to localhost on its port, as a browser
// opening http://localhost:PORT would.
func (i *instance) atLocalhost(method, path string, body any) *http.Response {
	i.t.Helper()
	// The CSRF cookie comes with the first answer: a page has always loaded once
	// before it posts anything.
	if i.csrf() == "" {
		_ = statusOf(i, http.MethodGet, "/api/status", nil)
	}
	payload, err := json.Marshal(body)
	if err != nil {
		i.t.Fatal(err)
	}
	req, err := http.NewRequest(method, i.server.URL+path, bytes.NewReader(payload))
	if err != nil {
		i.t.Fatal(err)
	}
	req.Host = i.localhost()
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	if token := i.csrf(); token != "" {
		req.Header.Set(middleware.CSRFHeader, token)
	}
	// With a Host other than the URL's the client sends no cookies from its jar,
	// and keeps none it is given: both are done here by hand, under the URL.
	for _, c := range i.client.Jar.Cookies(req.URL) {
		req.AddCookie(c)
	}
	resp, err := i.client.Do(req)
	if err != nil {
		i.t.Fatal(err)
	}
	i.client.Jar.SetCookies(req.URL, resp.Cookies())
	return resp
}

func (i *instance) localhost() string {
	u, _ := url.Parse(i.server.URL)
	return "localhost:" + u.Port()
}

func (i *instance) origin() string { return "http://" + i.localhost() }

const presentAndVerified = softkey.UserPresent | softkey.UserVerified

// addPasskey makes a passkey for the account i is signed in as, the way a
// browser and an authenticator do it.
func (i *instance) addPasskey(key *softkey.Key) {
	i.t.Helper()
	resp := i.atLocalhost(http.MethodPost, "/api/me/passkeys/register/begin", nil)
	if resp.StatusCode != http.StatusOK {
		i.t.Fatalf("register begin: %d", resp.StatusCode)
	}
	options := decode(i.t, resp)
	challenge, _ := webauthn.B64.DecodeString(options["challenge"].(string))
	handle, _ := webauthn.B64.DecodeString(options["user"].(map[string]any)["id"].(string))
	key.UserHandle = handle
	resp = i.atLocalhost(http.MethodPost, "/api/me/passkeys/register/finish", map[string]string{
		"name":              "Firefox · Linux",
		"clientDataJSON":    webauthn.B64.EncodeToString(softkey.ClientData(webauthn.TypeCreate, challenge, i.origin())),
		"attestationObject": webauthn.B64.EncodeToString(key.Create("localhost", presentAndVerified)),
	})
	if resp.StatusCode != http.StatusCreated {
		i.t.Fatalf("register finish: %d %s", resp.StatusCode, body(i.t, resp))
	}
	_ = resp.Body.Close()
}

// passkeyAnswer is what a browser sends back for a sign-in challenge.
func (i *instance) passkeyAnswer(key *softkey.Key, flags byte) map[string]string {
	i.t.Helper()
	resp := i.atLocalhost(http.MethodPost, "/api/session/passkey/begin", nil)
	if resp.StatusCode != http.StatusOK {
		i.t.Fatalf("sign-in begin: %d", resp.StatusCode)
	}
	challenge, _ := webauthn.B64.DecodeString(decode(i.t, resp)["challenge"].(string))
	clientData := softkey.ClientData(webauthn.TypeGet, challenge, i.origin())
	authData, sig := key.Get("localhost", flags, clientData)
	return map[string]string{
		"credentialId":      webauthn.B64.EncodeToString(key.CredentialID),
		"clientDataJSON":    webauthn.B64.EncodeToString(clientData),
		"authenticatorData": webauthn.B64.EncodeToString(authData),
		"signature":         webauthn.B64.EncodeToString(sig),
		"userHandle":        webauthn.B64.EncodeToString(key.UserHandle),
	}
}

func (i *instance) finishPasskey(answer map[string]string) int {
	resp := i.atLocalhost(http.MethodPost, "/api/session/passkey/finish", answer)
	_ = resp.Body.Close()
	return resp.StatusCode
}

// A passkey made while signed in signs in with nothing typed, both kinds of key,
// and each answer is good once.
func TestAPasskeySignsInWithNothingTyped(t *testing.T) {
	for _, alg := range []int64{webauthn.AlgES256, webauthn.AlgEdDSA} {
		i := newInstance(t)
		i.claim()
		key := softkey.New(alg)
		i.addPasskey(key)

		browser := i.anonymous()
		answer := browser.passkeyAnswer(key, presentAndVerified)
		if got := browser.finishPasskey(answer); got != http.StatusOK {
			t.Fatalf("alg %d: sign-in: %d", alg, got)
		}
		if got := statusOf(browser, http.MethodGet, "/api/me", nil); got != http.StatusOK {
			t.Errorf("alg %d: not signed in after the passkey: %d", alg, got)
		}
		if got := i.anonymous().finishPasskey(answer); got == http.StatusOK {
			t.Errorf("alg %d: the same answer signed in twice", alg)
		}
	}
}

// A passkey used without the person verifying themselves is refused: it stands
// for the password and the second step together.
func TestAPasskeyWithoutVerificationIsRefused(t *testing.T) {
	i := newInstance(t)
	i.claim()
	key := softkey.New(webauthn.AlgES256)
	i.addPasskey(key)
	b := i.anonymous()
	if got := b.finishPasskey(b.passkeyAnswer(key, softkey.UserPresent)); got != http.StatusUnauthorized {
		t.Errorf("present but not verified: %d", got)
	}
}

// A counter that goes back means the passkey was copied.
func TestACounterThatGoesBackIsRefused(t *testing.T) {
	i := newInstance(t)
	i.claim()
	key := softkey.New(webauthn.AlgES256)
	key.Count = 5
	i.addPasskey(key)
	b := i.anonymous()
	if got := b.finishPasskey(b.passkeyAnswer(key, presentAndVerified)); got != http.StatusOK {
		t.Fatalf("a counting passkey: %d", got)
	}
	key.Count = 2
	c := i.anonymous()
	if got := c.finishPasskey(c.passkeyAnswer(key, presentAndVerified)); got != http.StatusUnauthorized {
		t.Errorf("a counter that went back: %d", got)
	}
}

// A passkey of somebody else's, or another site's, signs nobody in.
func TestAPasskeyFromElsewhereIsRefused(t *testing.T) {
	i := newInstance(t)
	i.claim()
	key := softkey.New(webauthn.AlgES256)
	i.addPasskey(key)

	stranger := softkey.New(webauthn.AlgES256)
	stranger.UserHandle = key.UserHandle
	b := i.anonymous()
	if got := b.finishPasskey(b.passkeyAnswer(stranger, presentAndVerified)); got != http.StatusUnauthorized {
		t.Errorf("an unknown passkey: %d", got)
	}

	// The passkey's id with somebody else's key: everything checks but the signature.
	forger := softkey.New(webauthn.AlgES256)
	forger.CredentialID = key.CredentialID
	forger.UserHandle = key.UserHandle
	f := i.anonymous()
	if got := f.finishPasskey(f.passkeyAnswer(forger, presentAndVerified)); got != http.StatusUnauthorized {
		t.Errorf("a signature by another key: %d", got)
	}

	// Signed for another site: the browser's origin is right, the authenticator's is not.
	c := i.anonymous()
	resp := c.atLocalhost(http.MethodPost, "/api/session/passkey/begin", nil)
	challenge, _ := webauthn.B64.DecodeString(decode(t, resp)["challenge"].(string))
	clientData := softkey.ClientData(webauthn.TypeGet, challenge, c.origin())
	authData, sig := key.Get("evil.example", presentAndVerified, clientData)
	if got := c.finishPasskey(map[string]string{
		"credentialId":      webauthn.B64.EncodeToString(key.CredentialID),
		"clientDataJSON":    webauthn.B64.EncodeToString(clientData),
		"authenticatorData": webauthn.B64.EncodeToString(authData),
		"signature":         webauthn.B64.EncodeToString(sig),
	}); got != http.StatusUnauthorized {
		t.Errorf("signed for another site: %d", got)
	}
}

// A passkey belongs to a name: never to an IP address, even over https, and
// over plain http only to localhost.
func TestTheSiteOfAPasskey(t *testing.T) {
	s := &Server{}
	for _, c := range []struct {
		host   string
		tls    bool
		want   string
		refuse bool
	}{
		{host: "help.example.org", tls: true, want: "help.example.org"},
		{host: "Help.Example.org:8443", tls: true, want: "help.example.org"},
		{host: "localhost:8080", want: "localhost"},
		{host: "app.localhost", want: "app.localhost"},
		{host: "help.example.org", refuse: true},
		{host: "192.168.1.5", tls: true, refuse: true},
		{host: "[2001:db8::1]:443", tls: true, refuse: true},
	} {
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.Host = c.host
		if c.tls {
			r.TLS = &tls.ConnectionState{}
		}
		rp, _, err := s.relyingParty(r)
		if c.refuse {
			if err == nil {
				t.Errorf("%s (tls %v): accepted as %q", c.host, c.tls, rp)
			}
			continue
		}
		if err != nil || rp != c.want {
			t.Errorf("%s (tls %v): %q, %v; want %q", c.host, c.tls, rp, err, c.want)
		}
	}
}

// Over an IP address there are no passkeys, and the server says so.
func TestNoPasskeysOnAnIPAddress(t *testing.T) {
	i := newInstance(t)
	i.claim()
	if got := statusOf(i, http.MethodPost, "/api/me/passkeys/register/begin", nil); got != http.StatusConflict {
		t.Errorf("register on an IP address: %d", got)
	}
	resp := i.do(http.MethodGet, "/api/me/passkeys", nil)
	if got := decode(t, resp); got["available"] != false {
		t.Errorf("listed as available on an IP address: %v", got)
	}
}

// A passkey sign-in asks for no code even when the account has a second step,
// and a disabled account is not signed in by its passkey.
func TestAPasskeySkipsTheSecondStepButNotADisabledAccount(t *testing.T) {
	i := newInstance(t)
	i.claim()
	i.turnOnTwoStep()
	key := softkey.New(webauthn.AlgEdDSA)
	i.addPasskey(key)

	b := i.anonymous()
	if got := b.finishPasskey(b.passkeyAnswer(key, presentAndVerified)); got != http.StatusOK {
		t.Fatalf("a passkey for an account with a second step: %d", got)
	}

	var ownerID int64
	if err := i.db().QueryRow(`SELECT id FROM app_user WHERE username = 'owner'`).Scan(&ownerID); err != nil {
		t.Fatal(err)
	}
	if err := (account.Store{DB: i.db()}).SetDisabled(ownerID, true); err != nil {
		t.Fatal(err)
	}
	c := i.anonymous()
	if got := c.finishPasskey(c.passkeyAnswer(key, presentAndVerified)); got == http.StatusOK {
		t.Error("a disabled account signed in with its passkey")
	}
}

// One account cannot remove another's passkey.
func TestAPasskeyIsRemovedOnlyByItsAccount(t *testing.T) {
	i := newInstance(t)
	i.claim()
	i.addPasskey(softkey.New(webauthn.AlgES256))
	var id int64
	if err := i.db().QueryRow(`SELECT id FROM passkey`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	if _, err := (account.Store{DB: i.db()}).Create("member", ownerPassword, false, i.now()); err != nil {
		t.Fatal(err)
	}
	member := i.anonymous()
	if got := statusOf(member, http.MethodPost, "/api/session", map[string]string{"username": "member", "password": ownerPassword}); got != http.StatusOK {
		t.Fatalf("member sign-in: %d", got)
	}
	if got := statusOf(member, http.MethodDelete, "/api/me/passkeys/"+itoa(id), nil); got != http.StatusNotFound {
		t.Errorf("another account's passkey: %d", got)
	}
	if got := statusOf(i, http.MethodDelete, "/api/me/passkeys/"+itoa(id), nil); got != http.StatusNoContent {
		t.Errorf("its own: %d", got)
	}
}
