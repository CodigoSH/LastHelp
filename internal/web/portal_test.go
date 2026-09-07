package web

import (
	"net/http"
	"net/http/cookiejar"
	"testing"
	"time"

	"git.codigosh.com/CodigoSH/LastHelp/internal/core/account"
	"git.codigosh.com/CodigoSH/LastHelp/internal/core/authz"
	"git.codigosh.com/CodigoSH/LastHelp/internal/core/item"
)

// anonymous is a second client on the same instance with no session: the public.
func (i *instance) anonymous() *instance {
	i.t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		i.t.Fatal(err)
	}
	return &instance{t: i.t, server: i.server, client: &http.Client{Jar: jar},
		dbHandle: i.dbHandle, clock: i.clock, tick: i.tick}
}

func (i *instance) createOrg(slug string) {
	i.t.Helper()
	resp := i.do(http.MethodPost, "/api/organizations", map[string]string{"slug": slug, "name": slug})
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusCreated {
		i.t.Fatalf("creating organisation %s returned %d", slug, resp.StatusCode)
	}
}

func (i *instance) createRepo(orgSlug, repoSlug string) {
	i.t.Helper()
	resp := i.do(http.MethodPost, "/api/orgs/"+orgSlug+"/repositories",
		map[string]any{"slug": repoSlug, "name": repoSlug})
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusCreated {
		i.t.Fatalf("creating repository %s/%s returned %d", orgSlug, repoSlug, resp.StatusCode)
	}
}

// submit does what the page does: fetch the form, take a moment to fill it in,
// then send it with the token.
func (i *instance) submit(orgSlug, repoSlug, kind, title string) (publicID, token string) {
	i.t.Helper()
	base := "/api/orgs/" + orgSlug + "/repos/" + repoSlug

	formResp := i.do(http.MethodGet, base+"/form", nil)
	form := decode(i.t, formResp)
	formToken, _ := form["token"].(string)

	// Long enough that it was typed rather than posted.
	i.tick.advance(5 * time.Second)

	resp := i.do(http.MethodPost, base+"/items",
		map[string]any{"kind": kind, "title": title, "body": "what happened for " + title,
			"token": formToken})
	body := decode(i.t, resp)
	if resp.StatusCode != http.StatusCreated {
		i.t.Fatalf("submitting returned %d: %v", resp.StatusCode, body)
	}
	id, _ := body["public_id"].(string)
	tok, _ := body["follow_token"].(string)
	return id, tok
}

// TestTheWholeJourney is the product in one test: somebody reports without an
// account, it waits, the team approves it, and then it is public and votable.
func TestTheWholeJourney(t *testing.T) {
	team := newInstance(t)
	team.claim()
	team.createOrg("codigosh")
	team.createRepo("codigosh", "lastboard")

	public := team.anonymous()
	id, token := public.submit("codigosh", "lastboard", "idea", "dark mode")
	if id == "" || token == "" {
		t.Fatal("no public id or follow link came back")
	}

	// Nothing waiting is public.
	resp := public.do(http.MethodGet, "/api/orgs/codigosh/repos/lastboard/items", nil)
	if got := len(decode(t, resp)["items"].([]any)); got != 0 {
		t.Fatalf("the public board shows %d items before anybody approved anything", got)
	}

	// A stranger cannot see it either, not even by its address.
	stranger := team.anonymous()
	resp = stranger.do(http.MethodGet, "/api/items/"+id, nil)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("a waiting item is visible to a stranger: %d", resp.StatusCode)
	}

	// The reporter can, with the secret link.
	resp = public.do(http.MethodGet, "/api/items/"+id+"?k="+token, nil)
	mine := decode(t, resp)
	if resp.StatusCode != http.StatusOK || mine["mine"] != true {
		t.Fatalf("the reporter cannot see their own report: %d %v", resp.StatusCode, mine)
	}

	// The team approves it.
	resp = team.do(http.MethodPost, "/api/items/"+id+"/moderation", map[string]string{"state": "published"})
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("approving returned %d", resp.StatusCode)
	}

	// Now it is public, and votable.
	resp = stranger.do(http.MethodGet, "/api/orgs/codigosh/repos/lastboard/items", nil)
	if got := len(decode(t, resp)["items"].([]any)); got != 1 {
		t.Fatalf("the public board shows %d items after approval", got)
	}

	resp = stranger.do(http.MethodPost, "/api/items/"+id+"/votes", nil)
	voted := decode(t, resp)
	if resp.StatusCode != http.StatusOK || voted["votes"] != float64(1) || voted["voted"] != true {
		t.Fatalf("voting returned %d %v", resp.StatusCode, voted)
	}

	// The same browser voting again still counts once.
	resp = stranger.do(http.MethodPost, "/api/items/"+id+"/votes", nil)
	again := decode(t, resp)
	if again["votes"] != float64(1) {
		t.Errorf("a second vote from the same browser counted: %v", again)
	}

	// A different browser is a different voter.
	other := team.anonymous()
	resp = other.do(http.MethodPost, "/api/items/"+id+"/votes", nil)
	third := decode(t, resp)
	if third["votes"] != float64(2) {
		t.Errorf("a second browser did not count: %v", third)
	}
}

func TestABugCannotBeVoted(t *testing.T) {
	team := newInstance(t)
	team.claim()
	team.createOrg("codigosh")
	team.createRepo("codigosh", "lastboard")

	public := team.anonymous()
	id, _ := public.submit("codigosh", "lastboard", "bug", "crash on import")
	resp := team.do(http.MethodPost, "/api/items/"+id+"/moderation", map[string]string{"state": "published"})
	_ = resp.Body.Close()

	resp = public.do(http.MethodPost, "/api/items/"+id+"/votes", nil)
	body := decode(t, resp)
	if resp.StatusCode != http.StatusBadRequest || body["error"] != "error.not_votable" {
		t.Errorf("voting a bug returned %d %v — two hundred people agreeing that something "+
			"crashes does not make it crash harder", resp.StatusCode, body)
	}
}

// TestACollaboratorCannotTouchTheRepositoryNextDoor.
//
// This is the isolation the two levels of roles exist for, checked from
// outside: not "authz would refuse", but "the HTTP endpoint refuses".
func TestACollaboratorCannotTouchTheRepositoryNextDoor(t *testing.T) {
	team := newInstance(t)
	team.claim()
	team.createOrg("codigosh")
	team.createRepo("codigosh", "lastboard")
	team.createRepo("codigosh", "lastdav")

	// Somebody who triages only lastboard.
	accounts := account.Store{DB: team.db()}
	helper, err := accounts.Create("helper@example.com", "a good long passphrase", "Helper", false, team.now())
	if err != nil {
		t.Fatal(err)
	}
	var boardID int64
	if err := team.db().QueryRow(`SELECT id FROM repository WHERE slug = 'lastboard'`).Scan(&boardID); err != nil {
		t.Fatal(err)
	}
	if err := accounts.Grant(helper.ID, authz.Repository(boardID, 0), authz.RoleCollaborator, team.now()); err != nil {
		t.Fatal(err)
	}

	// An item on each board.
	public := team.anonymous()
	mine, _ := public.submit("codigosh", "lastboard", "idea", "in my repository")
	theirs, _ := public.submit("codigosh", "lastdav", "idea", "next door")

	helperClient := team.anonymous()
	resp := helperClient.do(http.MethodPost, "/api/session", map[string]string{
		"email": "helper@example.com", "password": "a good long passphrase",
	})
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("the collaborator could not sign in: %d", resp.StatusCode)
	}

	// Their own: allowed.
	resp = helperClient.do(http.MethodPost, "/api/items/"+mine+"/moderation", map[string]string{"state": "published"})
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("the collaborator could not moderate their own repository: %d", resp.StatusCode)
	}

	// Next door: refused.
	resp = helperClient.do(http.MethodPost, "/api/items/"+theirs+"/moderation", map[string]string{"state": "published"})
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("the collaborator moderated the repository next door: %d", resp.StatusCode)
	}

	// And cannot register repositories at all.
	resp = helperClient.do(http.MethodPost, "/api/orgs/codigosh/repositories", map[string]string{"slug": "sneaky"})
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("the collaborator created a repository: %d", resp.StatusCode)
	}
}

// TestAnOwnerOfOneOrganizationHasNothingInAnother.
func TestAnOwnerOfOneOrganizationHasNothingInAnother(t *testing.T) {
	team := newInstance(t)
	team.claim()
	team.createOrg("codigosh")
	team.createOrg("jellyfin")
	team.createRepo("jellyfin", "server")

	accounts := account.Store{DB: team.db()}
	outsider, err := accounts.Create("owner@other.example", "a good long passphrase", "Other", false, team.now())
	if err != nil {
		t.Fatal(err)
	}
	var codigoshID int64
	if err := team.db().QueryRow(`SELECT id FROM organization WHERE slug = 'codigosh'`).Scan(&codigoshID); err != nil {
		t.Fatal(err)
	}
	if err := accounts.Grant(outsider.ID, authz.Organization(codigoshID), authz.RoleOwner, team.now()); err != nil {
		t.Fatal(err)
	}

	client := team.anonymous()
	resp := client.do(http.MethodPost, "/api/session", map[string]string{
		"email": "owner@other.example", "password": "a good long passphrase",
	})
	_ = resp.Body.Close()

	// Their own organisation: allowed.
	resp = client.do(http.MethodPost, "/api/orgs/codigosh/repositories", map[string]string{"slug": "lastboard"})
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Errorf("an owner could not register a repository in their own organisation: %d", resp.StatusCode)
	}

	// Somebody else's: refused.
	resp = client.do(http.MethodPost, "/api/orgs/jellyfin/repositories", map[string]string{"slug": "sneaky"})
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("an owner of one organisation registered a repository in another: %d", resp.StatusCode)
	}

	// And creating organisations is the server's business, not theirs.
	resp = client.do(http.MethodPost, "/api/organizations", map[string]string{"slug": "invented"})
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("a non-administrator created an organisation: %d", resp.StatusCode)
	}
}

// TestTheQueueIsBehindTheDoor.
func TestTheQueueIsBehindTheDoor(t *testing.T) {
	team := newInstance(t)
	team.claim()
	team.createOrg("codigosh")
	team.createRepo("codigosh", "lastboard")

	public := team.anonymous()
	public.submit("codigosh", "lastboard", "idea", "something waiting")

	resp := public.do(http.MethodGet, "/api/orgs/codigosh/repos/lastboard/queue", nil)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("the moderation queue answered %d to the public", resp.StatusCode)
	}

	resp = team.do(http.MethodGet, "/api/orgs/codigosh/repos/lastboard/queue", nil)
	body := decode(t, resp)
	if resp.StatusCode != http.StatusOK || len(body["items"].([]any)) != 1 {
		t.Errorf("the team's queue returned %d %v", resp.StatusCode, body)
	}
}

// TestSimilarItemsAreOfferedWhileTyping.
func TestSimilarItemsAreOfferedWhileTyping(t *testing.T) {
	team := newInstance(t)
	team.claim()
	team.createOrg("codigosh")
	team.createRepo("codigosh", "lastboard")

	public := team.anonymous()
	id, _ := public.submit("codigosh", "lastboard", "idea", "dark mode for the dashboard")
	resp := team.do(http.MethodPost, "/api/items/"+id+"/moderation", map[string]string{"state": "published"})
	_ = resp.Body.Close()

	resp = public.do(http.MethodGet, "/api/orgs/codigosh/repos/lastboard/similar?title=dark+mode", nil)
	body := decode(t, resp)
	similar, _ := body["similar"].([]any)
	if len(similar) != 1 {
		t.Fatalf("looking for a near-duplicate found %d: %v", len(similar), body)
	}
}

// TestAKindTheBoardDoesNotAcceptIsRefused.
func TestAKindTheBoardDoesNotAcceptIsRefused(t *testing.T) {
	team := newInstance(t)
	team.claim()
	team.createOrg("codigosh")

	resp := team.do(http.MethodPost, "/api/orgs/codigosh/repositories", map[string]any{
		"slug": "ideas-only", "accepts_bugs": false, "accepts_ideas": true,
	})
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("creating the board returned %d", resp.StatusCode)
	}

	public := team.anonymous()
	form := decode(t, public.do(http.MethodGet, "/api/orgs/codigosh/repos/ideas-only/form", nil))
	public.tick.advance(5 * time.Second)

	resp = public.do(http.MethodPost, "/api/orgs/codigosh/repos/ideas-only/items",
		map[string]any{"kind": "bug", "title": "a bug", "token": form["token"]})
	body := decode(t, resp)
	if resp.StatusCode != http.StatusBadRequest || body["error"] != "error.kind_not_accepted" {
		t.Errorf("a bug on an ideas-only board returned %d %v", resp.StatusCode, body)
	}
}

// TestAStatusFromTheOtherKindIsRefusedAtTheEdge.
func TestAStatusFromTheOtherKindIsRefusedAtTheEdge(t *testing.T) {
	team := newInstance(t)
	team.claim()
	team.createOrg("codigosh")
	team.createRepo("codigosh", "lastboard")

	public := team.anonymous()
	id, _ := public.submit("codigosh", "lastboard", "bug", "crash")

	resp := team.do(http.MethodPost, "/api/items/"+id+"/status",
		map[string]string{"status": string(item.StatusPlanned)})
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("a bug was marked planned over HTTP: %d", resp.StatusCode)
	}

	resp = team.do(http.MethodPost, "/api/items/"+id+"/status",
		map[string]string{"status": string(item.StatusConfirmed)})
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("a bug could not be confirmed: %d", resp.StatusCode)
	}
}

// TestAVoterCookieCannotBeForged.
//
// Without a signature, one person votes as a thousand by editing a cookie —
// which is cheaper than clearing it a thousand times.
func TestAVoterCookieCannotBeForged(t *testing.T) {
	team := newInstance(t)
	team.claim()
	team.createOrg("codigosh")
	team.createRepo("codigosh", "lastboard")

	public := team.anonymous()
	id, _ := public.submit("codigosh", "lastboard", "idea", "dark mode")
	resp := team.do(http.MethodPost, "/api/items/"+id+"/moderation", map[string]string{"state": "published"})
	_ = resp.Body.Close()

	// Vote once with a real cookie.
	resp = public.do(http.MethodPost, "/api/items/"+id+"/votes", nil)
	_ = resp.Body.Close()

	// Now try a made-up voter identity.
	forger := team.anonymous()
	req, err := http.NewRequest(http.MethodPost, forger.server.URL+"/api/items/"+id+"/votes", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	req.Header.Set("X-CSRF-Token", forger.csrf())
	req.AddCookie(&http.Cookie{Name: VoterCookie, Value: "invented.signature"})
	resp, err = forger.client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body := decode(t, resp)

	// The forged identity is discarded and a fresh signed one is minted, so the
	// vote still happens — but as a new voter, not as one they chose.
	if body["votes"] != float64(2) {
		t.Errorf("votes came back %v; the forged cookie should have been discarded and replaced", body["votes"])
	}
	for _, c := range resp.Cookies() {
		if c.Name == VoterCookie && c.Value == "invented.signature" {
			t.Error("the server kept the forged voter identity")
		}
	}
}
