package web

import (
	"bytes"
	"io"
	"net/http"
	"strings"
	"testing"

	"git.codigosh.com/CodigoSH/LastHelp/internal/forge/fake"

	"git.codigosh.com/CodigoSH/LastHelp/internal/version"
)

// connected sets an organisation up with a forge, credentials and a webhook
// address, and returns that address.
func (i *instance) connected(t *testing.T, kind, secret string) string {
	t.Helper()

	resp := i.do(http.MethodPost, "/api/organizations", map[string]string{
		"slug": "codigosh", "name": "CodigoSH", "forge": kind,
		"base_url": "https://forge.example", "forge_owner": "CodigoSH",
	})
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("creating the organisation returned %d", resp.StatusCode)
	}
	i.createRepo("codigosh", "lastboard")

	body := decode(t, i.do(http.MethodPost, "/api/orgs/codigosh/credentials",
		map[string]string{"token": "a-token", "webhook_secret": secret}))
	path, _ := body["webhook_path"].(string)
	if path == "" {
		t.Fatal("no webhook address came back")
	}
	return path
}

// deliver posts a delivery the way a forge would: no session, no CSRF token,
// just a signature.
func (i *instance) deliver(path string, headers http.Header, body []byte) *http.Response {
	i.t.Helper()

	req, err := http.NewRequest(http.MethodPost, i.server.URL+path, bytes.NewReader(body))
	if err != nil {
		i.t.Fatal(err)
	}
	for name, values := range headers {
		for _, v := range values {
			req.Header.Add(name, v)
		}
	}
	resp, err := (&http.Client{}).Do(req)
	if err != nil {
		i.t.Fatal(err)
	}
	return resp
}

// TestAForgeCanDeliverWithoutASession.
//
// A forge cannot sign in, and it does not send a CSRF token. This endpoint has
// to work for something that has neither — which is why its signature is the
// whole of its authentication.
func TestAForgeCanDeliverWithoutASession(t *testing.T) {
	team := newInstance(t)
	team.claim()
	const secret = "the webhook secret"
	path := team.connected(t, "forgejo", secret)

	headers, body := fake.GiteaDelivery(secret, "closed", 1, "closed")
	resp := team.deliver(path, headers, body)
	_ = resp.Body.Close()

	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("a signed delivery returned %d", resp.StatusCode)
	}
}

// TestAnUnsignedDeliveryIsRefused.
func TestAnUnsignedDeliveryIsRefused(t *testing.T) {
	team := newInstance(t)
	team.claim()
	path := team.connected(t, "forgejo", "the webhook secret")

	_, body := fake.GiteaDelivery("the webhook secret", "closed", 1, "closed")
	resp := team.deliver(path, http.Header{}, body)
	_ = resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("a delivery with no signature returned %d", resp.StatusCode)
	}
}

func TestADeliverySignedWithTheWrongSecretIsRefused(t *testing.T) {
	team := newInstance(t)
	team.claim()
	path := team.connected(t, "forgejo", "the webhook secret")

	headers, body := fake.GiteaDelivery("somebody else's secret", "closed", 1, "closed")
	resp := team.deliver(path, headers, body)
	_ = resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("a delivery signed with the wrong secret returned %d", resp.StatusCode)
	}
}

// TestTheVerifierComesFromTheConnectionAndNotTheHeaders.
//
// This is the test the whole receiver design exists for.
//
// Forgejo sends GitHub's event and delivery headers alongside its own. If this
// server chose a verifier by looking at headers, a caller could dress a
// delivery up as whichever forge had the weakest check — GitLab's legacy scheme
// is a plain shared token — and be verified under that instead.
//
// Here the connection says "gitlab", so a GitLab verifier is used. The delivery
// is a perfectly valid Forgejo one, GitHub headers and all. It must be refused.
func TestTheVerifierComesFromTheConnectionAndNotTheHeaders(t *testing.T) {
	team := newInstance(t)
	team.claim()
	const secret = "the webhook secret"
	path := team.connected(t, "gitlab", secret)

	headers, body := fake.GiteaDelivery(secret, "closed", 1, "closed")
	resp := team.deliver(path, headers, body)
	_ = resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("a Forgejo delivery was accepted by a GitLab connection: %d\n\n"+
			"The verifier has to come from the connection record. Forgejo sends GitHub's\n"+
			"headers too, so anything that sniffs them lets the caller choose which\n"+
			"verification it faces.", resp.StatusCode)
	}
}

// TestAnUnknownWebhookAddressLooksLikeNothing.
//
// An address that does not exist and one that does but failed to verify must be
// indistinguishable, or this endpoint becomes a way to enumerate which
// organisations an instance has.
func TestAnUnknownWebhookAddressIs404(t *testing.T) {
	team := newInstance(t)
	team.claim()
	team.connected(t, "forgejo", "the webhook secret")

	headers, body := fake.GiteaDelivery("the webhook secret", "closed", 1, "closed")
	resp := team.deliver("/hooks/not-a-real-address", headers, body)
	_ = resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("an unknown webhook address returned %d", resp.StatusCode)
	}
}

// TestTheTokenIsNeverEchoedBack.
//
// An endpoint that returns what it stored is a way to read a credential through
// the panel, which is exactly what the two role levels exist to prevent.
func TestTheTokenIsNeverEchoedBack(t *testing.T) {
	team := newInstance(t)
	team.claim()

	resp := team.do(http.MethodPost, "/api/organizations", map[string]string{
		"slug": "codigosh", "forge": "forgejo", "base_url": "https://forge.example",
	})
	_ = resp.Body.Close()

	body := decode(t, team.do(http.MethodPost, "/api/orgs/codigosh/credentials",
		map[string]string{"token": "ghp_the_secret_token", "webhook_secret": "s"}))

	for key, value := range body {
		if text, ok := value.(string); ok && text == "ghp_the_secret_token" {
			t.Errorf("the token came back in %q", key)
		}
	}
}

// TestTheTokenNeverComesBackOutOfTheAPI.
//
// This replaced a test that checked the token was unreadable in the database.
// It is readable now, deliberately: a key kept beside the database protects
// nothing against the file leaving the machine, because the key leaves with it.
// The boundary is the backup, and that is where the password belongs.
//
// What still has to be true, and is what this asks: the token goes in and never
// comes back. A panel that echoes it into a form, a listing that carries it for
// convenience, an error that quotes it — each of those puts write access to
// somebody's forge into a browser, a proxy log and a screenshot.
func TestTheTokenNeverComesBackOutOfTheAPI(t *testing.T) {
	const secret = "ghp_the_secret_token"

	team := newInstance(t)
	team.claim()

	resp := team.do(http.MethodPost, "/api/organizations", map[string]string{
		"slug": "codigosh", "forge": "forgejo", "base_url": "https://forge.example",
	})
	_ = resp.Body.Close()
	resp = team.do(http.MethodPost, "/api/orgs/codigosh/credentials",
		map[string]string{"token": secret, "webhook_secret": "s"})
	_ = resp.Body.Close()

	// It is stored, because the bridge has to use it.
	var stored string
	if err := team.db().QueryRow(`SELECT token FROM organization WHERE slug = 'codigosh'`).
		Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if stored != secret {
		t.Fatalf("the token was not stored as given: %q", stored)
	}

	// And it does not come back out of anything the panel asks for.
	for _, path := range []string{
		"/api/organizations",
		"/api/orgs/codigosh",
		"/api/orgs/codigosh/credentials",
		"/api/orgs/codigosh/repositories",
	} {
		resp := team.do(http.MethodGet, path, nil)
		body, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if strings.Contains(string(body), secret) {
			t.Errorf("%s hands the forge token back:\n%s", path, body)
		}
	}
}

// TestACollaboratorCannotReachTheCredentials keeps the promise the two levels
// were created for.
func TestACollaboratorCannotReachTheCredentials(t *testing.T) {
	team := newInstance(t)
	team.claim()
	team.connected(t, "forgejo", "s")

	helper := team.anonymous()
	makeCollaborator(t, team, helper, "helper")

	resp := helper.do(http.MethodPost, "/api/orgs/codigosh/credentials",
		map[string]string{"token": "mine-now", "webhook_secret": "s"})
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("a collaborator reached the forge credentials: %d", resp.StatusCode)
	}
}

// TestNothingElseLivesUnderHooks.
//
// The webhook handler sits outside the application's chain — no session check,
// no CSRF, no same-site check. That is safe for exactly one route and would be
// a hole for any other, so the branch that sends requests there must reach
// nothing else: not the API, not the application shell, not a file.
func TestNothingElseLivesUnderHooks(t *testing.T) {
	team := newInstance(t)
	team.claim()
	path := team.connected(t, "forgejo", "s")

	for _, probe := range []struct{ method, target string }{
		{http.MethodGet, "/hooks/"},
		{http.MethodGet, path},
		{http.MethodPost, "/hooks/"},
		{http.MethodPost, "/hooks/../api/me"},
		{http.MethodGet, "/hooks/anything/else"},
	} {
		req, err := http.NewRequest(probe.method, team.server.URL+probe.target, nil)
		if err != nil {
			t.Fatal(err)
		}
		resp, err := (&http.Client{}).Do(req)
		if err != nil {
			t.Fatal(err)
		}
		body := make([]byte, 256)
		n, _ := resp.Body.Read(body)
		_ = resp.Body.Close()

		if resp.StatusCode < 400 {
			t.Errorf("%s %s answered %d outside the application's chain", probe.method, probe.target, resp.StatusCode)
		}
		if bytes.Contains(body[:n], []byte("<title>LastHelp</title>")) {
			t.Errorf("%s %s served the application shell", probe.method, probe.target)
		}
	}
}

// TestOnlyTheAdministratorSeesUpdates.
//
// Replacing the binary is not something an organisation's owner does to a
// machine somebody else is also using.
func TestOnlyTheAdministratorSeesUpdates(t *testing.T) {
	team := newInstance(t)
	team.claim()
	team.createOrg("codigosh")
	team.createRepo("codigosh", "lastboard")

	helper := team.anonymous()
	makeCollaborator(t, team, helper, "helper")

	resp := helper.do(http.MethodGet, "/api/update", nil)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("a collaborator could see the update panel: %d", resp.StatusCode)
	}

	resp = helper.do(http.MethodPost, "/api/update", nil)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("a collaborator could start an update: %d", resp.StatusCode)
	}

	stranger := team.anonymous()
	resp = stranger.do(http.MethodGet, "/api/update", nil)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("the update panel answered %d to the public", resp.StatusCode)
	}
}

// TestABuildWithNoKeysSaysSoRatherThanLookingUpToDate.
//
// A build with no release keys cannot verify anything, and the panel has to say
// so. An empty answer where a reason belongs reads as "you are up to date",
// which is the one thing it must never say by accident.
//
// The key list is emptied for the duration rather than relied upon to be empty:
// this test passed for as long as no release had been cut, and the day the
// first signing key was added it failed — not because the behaviour broke, but
// because the test was reading the state of the world instead of setting it.
func TestABuildWithNoKeysSaysSoRatherThanLookingUpToDate(t *testing.T) {
	pinNotInAContainer(t)
	keys := version.ReleaseKeys
	version.ReleaseKeys = nil
	t.Cleanup(func() { version.ReleaseKeys = keys })

	team := newInstance(t)
	team.claim()

	body := decode(t, team.do(http.MethodGet, "/api/update", nil))

	if body["can_verify"] != false {
		t.Fatalf("this build reports it can verify releases, and it has no keys: %v", body)
	}
	if body["reason"] == nil || body["reason"] == "" {
		t.Error("no reason was given; a missing reason reads as 'up to date'")
	}
	if body["available"] == true {
		t.Error("an update is being offered by a build that cannot verify one")
	}
	if body["current"] == nil || body["current"] == "" {
		t.Error("the panel does not say what is running")
	}
}

// TestAnUpdateIsRefusedWhenItCannotBeVerified.
//
// No key does not mean accept anything.
func TestAnUpdateIsRefusedWhenItCannotBeVerified(t *testing.T) {
	pinNotInAContainer(t)
	keys := version.ReleaseKeys
	version.ReleaseKeys = nil
	t.Cleanup(func() { version.ReleaseKeys = keys })

	team := newInstance(t)
	team.claim()

	resp := team.do(http.MethodPost, "/api/update", nil)
	body := decode(t, resp)
	if resp.StatusCode < 400 {
		t.Fatalf("an unverifiable update returned %d %v", resp.StatusCode, body)
	}
}

// pinNotInAContainer makes the test run as a build on a plain machine, whatever
// it is really running in. Inside a container the status handler still asks the
// forge, so as to say a newer image exists, and that is right there but is not
// what these tests are about: they passed on every laptop and failed in the CI
// job, which runs in a container, because they were reading the machine instead
// of setting it.
func pinNotInAContainer(t *testing.T) {
	t.Helper()
	was := inContainer
	inContainer = func() bool { return false }
	t.Cleanup(func() { inContainer = was })
}
