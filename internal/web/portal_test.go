package web

import (
	"bytes"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"git.codigosh.com/CodigoSH/LastHelp/internal/core/account"
	"git.codigosh.com/CodigoSH/LastHelp/internal/core/authz"
	"git.codigosh.com/CodigoSH/LastHelp/internal/core/bridge"
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

// submitWith is submit, with answers to the questions the form asks.
func (i *instance) submitWith(orgSlug, repoSlug, kind, title string, fields map[string]string) (publicID, token string) {
	i.t.Helper()
	base := "/api/orgs/" + orgSlug + "/repos/" + repoSlug

	formResp := i.do(http.MethodGet, base+"/form", nil)
	form := decode(i.t, formResp)
	formToken, _ := form["token"].(string)
	i.tick.advance(5 * time.Second)

	resp := i.do(http.MethodPost, base+"/items",
		map[string]any{"kind": kind, "title": title, "body": "what happened for " + title,
			"fields": fields, "token": formToken})
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

// TestAnAnsweredIdeaShowsItsCountAndNoButton is the same rule seen from the
// page: the number stays, the way to change it goes, and an old page that still
// has the button gets a refusal in words rather than a silent change.
func TestAnAnsweredIdeaShowsItsCountAndNoButton(t *testing.T) {
	team := newInstance(t)
	team.claim()
	team.createOrg("codigosh")
	team.createRepo("codigosh", "lastboard")

	public := team.anonymous()
	id, _ := public.submit("codigosh", "lastboard", "idea", "dark mode")
	resp := team.do(http.MethodPost, "/api/items/"+id+"/moderation", map[string]string{"state": "published"})
	_ = resp.Body.Close()

	resp = public.do(http.MethodPost, "/api/items/"+id+"/votes", nil)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("the first vote returned %d", resp.StatusCode)
	}

	open := decode(t, public.do(http.MethodGet, "/api/items/"+id, nil))
	if open["votable"] != true {
		t.Errorf("an open idea is not votable: %v", open["votable"])
	}

	resp = team.do(http.MethodPost, "/api/items/"+id+"/status", map[string]string{"status": string(item.StatusDone)})
	_ = resp.Body.Close()

	answered := decode(t, public.do(http.MethodGet, "/api/items/"+id, nil))
	if answered["votable"] != false || answered["votes"] != float64(1) {
		t.Errorf("an answered idea says votable=%v votes=%v, want false and 1", answered["votable"], answered["votes"])
	}

	for _, method := range []string{http.MethodPost, http.MethodDelete} {
		resp = public.do(method, "/api/items/"+id+"/votes", nil)
		body := decode(t, resp)
		if resp.StatusCode != http.StatusConflict || body["error"] != "error.voting_closed" {
			t.Errorf("%s on an answered idea returned %d %v", method, resp.StatusCode, body)
		}
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
	helper, err := accounts.Create("helper", "a good long passphrase", false, team.now())
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
		"username": "helper", "password": "a good long passphrase",
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
	outsider, err := accounts.Create("otherowner", "a good long passphrase", false, team.now())
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
		"username": "otherowner", "password": "a good long passphrase",
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

// TestASuggestionSaysWhatTheTeamDecided: somebody about to ask for something the
// team already declined is shown that it was, and how, rather than a bare title
// that reads as an open report they could vote for.
func TestASuggestionSaysWhatTheTeamDecided(t *testing.T) {
	team := newInstance(t)
	team.claim()
	team.createOrg("codigosh")
	team.createRepo("codigosh", "lastboard")
	public := team.anonymous()

	declined, _ := public.submit("codigosh", "lastboard", "idea", "keyboard shortcuts everywhere")
	open, _ := public.submit("codigosh", "lastboard", "idea", "keyboard layout switcher")
	for _, id := range []string{declined, open} {
		resp := team.do(http.MethodPost, "/api/items/"+id+"/moderation", map[string]string{"state": "published"})
		_ = resp.Body.Close()
	}
	resp := team.do(http.MethodPost, "/api/items/"+declined+"/status", map[string]string{"status": string(item.StatusDeclined)})
	_ = resp.Body.Close()

	body := decode(t, public.do(http.MethodGet, "/api/orgs/codigosh/repos/lastboard/similar?title=keyboard", nil))
	found, _ := body["similar"].([]any)
	if len(found) != 2 {
		t.Fatalf("the suggestions are %v, want both reports, the declined one included", body)
	}
	for _, raw := range found {
		entry, _ := raw.(map[string]any)
		switch entry["public_id"] {
		case declined:
			if entry["status"] != string(item.StatusDeclined) || entry["closed"] != true {
				t.Errorf("a declined report is suggested as %v (closed=%v)", entry["status"], entry["closed"])
			}
		case open:
			if entry["status"] != string(item.StatusNew) || entry["closed"] != false {
				t.Errorf("an open report is suggested as %v (closed=%v)", entry["status"], entry["closed"])
			}
		}
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

// TestTheVoterCookieLastsAYear.
//
// It only has to outlast an idea being open, since an answered idea takes no
// more votes. Five years was an identifier kept in a stranger's browser long
// after it stopped doing anything.
func TestTheVoterCookieLastsAYear(t *testing.T) {
	team := newInstance(t)
	team.claim()
	team.createOrg("codigosh")
	team.createRepo("codigosh", "lastboard")

	public := team.anonymous()
	id, _ := public.submit("codigosh", "lastboard", "idea", "dark mode")
	resp := team.do(http.MethodPost, "/api/items/"+id+"/moderation", map[string]string{"state": "published"})
	_ = resp.Body.Close()

	resp = public.do(http.MethodPost, "/api/items/"+id+"/votes", nil)
	_ = resp.Body.Close()
	for _, c := range resp.Cookies() {
		if c.Name == VoterCookie {
			if year := 365 * 24 * 60 * 60; c.MaxAge != year {
				t.Errorf("the voter cookie lasts %d seconds, want a year (%d)", c.MaxAge, year)
			}
			return
		}
	}
	t.Fatal("voting did not set a voter cookie")
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

// TestTheRoadmapIsPublicAndTheControlIsNot.
//
// The board itself is the point of the product, so it is readable by anybody.
// The control that moves an item along it is not, and the page asks the server
// whether to draw it rather than deciding for itself — a page that decides is a
// second implementation of the rules.
func TestTheRoadmapIsPublicAndTheControlIsNot(t *testing.T) {
	team := newInstance(t)
	team.claim()
	team.createOrg("codigosh")
	team.createRepo("codigosh", "lastboard")

	public := team.anonymous()
	id, _ := public.submit("codigosh", "lastboard", "idea", "dark mode")
	resp := team.do(http.MethodPost, "/api/items/"+id+"/moderation", map[string]string{"state": "published"})
	_ = resp.Body.Close()
	resp = team.do(http.MethodPost, "/api/items/"+id+"/status", map[string]string{"status": "planned"})
	_ = resp.Body.Close()

	// Anybody can read it.
	board := decode(t, public.do(http.MethodGet, "/api/orgs/codigosh/repos/lastboard/roadmap", nil))
	columns, _ := board["columns"].([]any)
	if len(columns) != 3 {
		t.Fatalf("the roadmap came back with %d columns; all three are always sent so the reader "+
			"is not left working out which ones they are looking at", len(columns))
	}
	first, _ := columns[0].(map[string]any)
	if got := len(first["items"].([]any)); got != 1 {
		t.Errorf("the planned column has %d items", got)
	}

	// Nobody without a session is told what statuses are available, let alone
	// sets one.
	if _, has := decode(t, public.do(http.MethodGet, "/api/items/"+id, nil))["team"]; has {
		t.Error("the public was handed the team's part of the report")
	}

	resp = public.do(http.MethodPost, "/api/items/"+id+"/status", map[string]string{"status": "done"})
	_ = resp.Body.Close()
	if resp.StatusCode < 400 {
		t.Errorf("the public moved an item along the roadmap: %d", resp.StatusCode)
	}
}

// TestTheRoadmapNeverShowsWhatNobodyHasRead.
//
// The default listing is published only, and the roadmap leans on that. If it
// ever stopped doing so, the public board would be showing submissions no
// person has looked at — which is the one thing the moderation queue exists to
// prevent.
func TestTheRoadmapNeverShowsWhatNobodyHasRead(t *testing.T) {
	team := newInstance(t)
	team.claim()
	team.createOrg("codigosh")
	team.createRepo("codigosh", "lastboard")

	public := team.anonymous()
	waiting, _ := public.submit("codigosh", "lastboard", "idea", "still in the queue")

	// Reach past the API to put it in a roadmap status while it is still
	// waiting: no endpoint allows that, which is the point — this is testing
	// the listing, not the endpoint.
	if _, err := team.db().Exec(`UPDATE item SET status = 'planned' WHERE public_id = ?`, waiting); err != nil {
		t.Fatal(err)
	}

	board := decode(t, public.do(http.MethodGet, "/api/orgs/codigosh/repos/lastboard/roadmap", nil))
	for _, raw := range board["columns"].([]any) {
		column, _ := raw.(map[string]any)
		if got := len(column["items"].([]any)); got != 0 {
			t.Errorf("column %v shows %d items, and nothing has been approved", column["column"], got)
		}
	}
}

// makeCollaborator gives an account triage rights on codigosh/lastboard and
// signs the given client in as them.
func makeCollaborator(t *testing.T, team, client *instance, username string) {
	t.Helper()

	accounts := account.Store{DB: team.db()}
	user, err := accounts.Create(username, "a good long passphrase", false, team.now())
	if err != nil {
		t.Fatal(err)
	}
	var repoID int64
	if err := team.db().QueryRow(`SELECT id FROM repository WHERE slug = 'lastboard'`).Scan(&repoID); err != nil {
		t.Fatal(err)
	}
	if err := accounts.Grant(user.ID, authz.Repository(repoID, 0), authz.RoleCollaborator, team.now()); err != nil {
		t.Fatal(err)
	}

	resp := client.do(http.MethodPost, "/api/session",
		map[string]string{"username": username, "password": "a good long passphrase"})
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("the collaborator could not sign in: %d", resp.StatusCode)
	}
}

// TestACommentReachesThePublicPage is the second half of the promise the page
// makes: "a person will read this before it appears".
//
// Until this passed, the first half happened and the second could not. A
// visitor's comment was stored as Pending, nothing listed it, no endpoint
// published it, and ApproveComment had no caller anywhere in the product — the
// conversation on every report died at the first reply from outside, silently,
// with the page still promising otherwise.
func TestACommentReachesThePublicPage(t *testing.T) {
	team := newInstance(t)
	team.claim()
	team.createOrg("codigosh")
	team.createRepo("codigosh", "lastboard")

	public := team.anonymous()
	id, _ := public.submit("codigosh", "lastboard", "idea", "dark mode")
	resp := team.do(http.MethodPost, "/api/items/"+id+"/moderation", map[string]string{"state": "published"})
	_ = resp.Body.Close()

	resp = public.do(http.MethodPost, "/api/items/"+id+"/comments",
		map[string]string{"body": "it happens on the phone too"})
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("commenting returned %d", resp.StatusCode)
	}

	// Not on the page yet: accepted is not published.
	resp = public.do(http.MethodGet, "/api/items/"+id, nil)
	if got := len(decode(t, resp)["comments"].([]any)); got != 0 {
		t.Fatalf("a comment nobody has read is already public: %d", got)
	}

	// The team finds it in the queue, next to the reports.
	resp = team.do(http.MethodGet, "/api/orgs/codigosh/repos/lastboard/comments", nil)
	waiting, _ := decode(t, resp)["comments"].([]any)
	if len(waiting) != 1 {
		t.Fatalf("the comment queue holds %d", len(waiting))
	}
	entry, _ := waiting[0].(map[string]any)
	on, _ := entry["item"].(map[string]any)
	if on["public_id"] != id {
		t.Errorf("the queue does not say which report the comment is on: %v", entry)
	}

	commentID, ok := entry["id"].(float64)
	if !ok {
		t.Fatalf("the queue entry has no id: %v", entry)
	}

	resp = team.do(http.MethodPost, fmt.Sprintf("/api/comments/%d/moderation", int64(commentID)),
		map[string]string{"state": "published"})
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("publishing the comment returned %d", resp.StatusCode)
	}

	resp = public.do(http.MethodGet, "/api/items/"+id, nil)
	thread, _ := decode(t, resp)["comments"].([]any)
	if len(thread) != 1 {
		t.Fatalf("the published comment is not on the page: %d", len(thread))
	}

	// And it is gone from the queue, rather than waiting to be decided twice.
	resp = team.do(http.MethodGet, "/api/orgs/codigosh/repos/lastboard/comments", nil)
	if got := len(decode(t, resp)["comments"].([]any)); got != 0 {
		t.Errorf("a published comment is still in the queue: %d", got)
	}
}

// TestOnlySomebodyWhoTriagesCanPublishAComment.
//
// The comment queue is a second door into the same room, and a door added later
// is the one left open. The rule is written about the repository the comment
// hangs on, not about the comment — so this signs in as somebody who triages
// the board next door and watches them be refused. The public being refused
// proves nothing here: the route is private, so the session middleware turns
// them away before the rule is ever consulted.
func TestOnlySomebodyWhoTriagesCanPublishAComment(t *testing.T) {
	team := newInstance(t)
	team.claim()
	team.createOrg("codigosh")
	team.createRepo("codigosh", "lastboard")
	team.createRepo("codigosh", "lastdav")

	public := team.anonymous()
	id, _ := public.submit("codigosh", "lastboard", "idea", "dark mode")
	resp := team.do(http.MethodPost, "/api/items/"+id+"/moderation", map[string]string{"state": "published"})
	_ = resp.Body.Close()
	resp = public.do(http.MethodPost, "/api/items/"+id+"/comments", map[string]string{"body": "me too"})
	_ = resp.Body.Close()

	// Somebody who triages the other board, and only it.
	accounts := account.Store{DB: team.db()}
	neighbour, err := accounts.Create("neighbour", "a good long passphrase", false, team.now())
	if err != nil {
		t.Fatal(err)
	}
	var nextDoor int64
	if err := team.db().QueryRow(`SELECT id FROM repository WHERE slug = 'lastdav'`).Scan(&nextDoor); err != nil {
		t.Fatal(err)
	}
	if err := accounts.Grant(neighbour.ID, authz.Repository(nextDoor, 0), authz.RoleCollaborator, team.now()); err != nil {
		t.Fatal(err)
	}

	them := team.anonymous()
	resp = them.do(http.MethodPost, "/api/session",
		map[string]string{"username": "neighbour", "password": "a good long passphrase"})
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("the collaborator could not sign in: %d", resp.StatusCode)
	}

	resp = them.do(http.MethodGet, "/api/orgs/codigosh/repos/lastboard/comments", nil)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("the comment queue of the board next door is readable: %d", resp.StatusCode)
	}

	resp = team.do(http.MethodGet, "/api/orgs/codigosh/repos/lastboard/comments", nil)
	waiting, _ := decode(t, resp)["comments"].([]any)
	entry, _ := waiting[0].(map[string]any)
	commentID, _ := entry["id"].(float64)
	path := fmt.Sprintf("/api/comments/%d/moderation", int64(commentID))

	resp = them.do(http.MethodPost, path, map[string]string{"state": "published"})
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("a comment on the board next door was published from outside it: %d", resp.StatusCode)
	}

	// And the public, who has no session at all, gets no further.
	resp = public.do(http.MethodPost, path, map[string]string{"state": "published"})
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized && resp.StatusCode != http.StatusForbidden {
		t.Errorf("the public could publish its own comment: %d", resp.StatusCode)
	}
}

// TestAReportSaysWhereItCameFrom.
//
// A report is the one page of this product that travels: it arrives in a chat
// as a bare link, opened by somebody who has seen no other screen. It used to
// answer with a title and nothing else — no board, no way onward.
//
// It also carries whether it is published, which is not decoration: the page
// tells a reporter their submission is waiting to be read, and without this it
// had no way to know it no longer was.
func TestAReportSaysWhereItCameFrom(t *testing.T) {
	team := newInstance(t)
	team.claim()
	team.createOrg("codigosh")
	team.createRepo("codigosh", "lastboard")

	public := team.anonymous()
	id, token := public.submit("codigosh", "lastboard", "idea", "dark mode")

	resp := public.do(http.MethodGet, "/api/items/"+id+"?k="+token, nil)
	waiting := decode(t, resp)
	if waiting["moderation"] != string(item.Pending) {
		t.Errorf("a report waiting to be read does not say so: %v", waiting["moderation"])
	}

	resp = team.do(http.MethodPost, "/api/items/"+id+"/moderation", map[string]string{"state": "published"})
	_ = resp.Body.Close()

	resp = public.do(http.MethodGet, "/api/items/"+id, nil)
	body := decode(t, resp)
	if body["moderation"] != string(item.Published) {
		t.Errorf("a published report still reads as waiting: %v", body["moderation"])
	}

	repo, _ := body["repository"].(map[string]any)
	if repo["slug"] != "lastboard" {
		t.Fatalf("the report does not name its board: %v", body["repository"])
	}
	org, _ := repo["organization"].(map[string]any)
	if org["slug"] != "codigosh" {
		t.Errorf("the report does not name the organisation: %v", repo)
	}
}

// TestABugIsAskedWhatMakesItUsable.
//
// The version and the steps existed everywhere except where somebody could
// type them: the table stored them, the export carried them, the forge issue
// printed them under a comment saying a bug without them is work rather than
// information — and the form never asked, so every one was empty in every
// report ever made.
//
// What is checked here is the half that is not the form: what the endpoint
// keeps. The questions belong to the kind, their order is the order they were
// asked in, and the labels are the domain's — the submission used to build a
// field out of every key it was sent and use that key as the label, so the
// sender chose the question as well as the answer.
func TestABugIsAskedWhatMakesItUsable(t *testing.T) {
	team := newInstance(t)
	team.claim()
	team.createOrg("codigosh")
	team.createRepo("codigosh", "lastboard")

	public := team.anonymous()
	id, _ := public.submitWith("codigosh", "lastboard", "bug", "crash on import", map[string]string{
		// Sent in the other order, and with a question nobody asked.
		"steps":           "1. open it\n2. import\n3. it closes",
		"version":         "1.4.2",
		"**Pay me here**": "https://not.a.real.place",
	})
	resp := team.do(http.MethodPost, "/api/items/"+id+"/moderation", map[string]string{"state": "published"})
	_ = resp.Body.Close()

	resp = public.do(http.MethodGet, "/api/items/"+id, nil)
	fields, _ := decode(t, resp)["fields"].([]any)
	if len(fields) != 2 {
		t.Fatalf("the report kept %d answers, not the two it was asked: %v", len(fields), fields)
	}

	first, _ := fields[0].(map[string]any)
	second, _ := fields[1].(map[string]any)
	if first["key"] != "version" || first["label"] != "Version" || first["value"] != "1.4.2" {
		t.Errorf("the first answer is not the first question: %v", first)
	}
	if second["key"] != "steps" || second["label"] != "Steps to reproduce" {
		t.Errorf("the second answer is not the second question: %v", second)
	}
	for _, raw := range fields {
		f, _ := raw.(map[string]any)
		if label, _ := f["label"].(string); strings.Contains(label, "Pay me") {
			t.Errorf("a question the sender wrote was kept: %v", f)
		}
	}
}

// TestAnIdeaIsNotAskedThem.
//
// An idea is the sentence somebody wrote. Asking it which version it happened
// in is a box that means nothing, and every box on a public form is somebody
// deciding not to bother — so the endpoint drops them even when they arrive.
func TestAnIdeaIsNotAskedThem(t *testing.T) {
	team := newInstance(t)
	team.claim()
	team.createOrg("codigosh")
	team.createRepo("codigosh", "lastboard")

	public := team.anonymous()
	id, _ := public.submitWith("codigosh", "lastboard", "idea", "dark mode", map[string]string{
		"version": "1.4.2", "steps": "1. open it",
	})
	resp := team.do(http.MethodPost, "/api/items/"+id+"/moderation", map[string]string{"state": "published"})
	_ = resp.Body.Close()

	resp = public.do(http.MethodGet, "/api/items/"+id, nil)
	if fields, _ := decode(t, resp)["fields"].([]any); len(fields) != 0 {
		t.Errorf("an idea was asked a bug's questions: %v", fields)
	}
}

// TestTheAnswersReachTheForge is the point of asking at all.
//
// A bug arrives on the forge with its version and its steps under the
// description, in the order they were asked — which is what makes it something
// somebody can pick up rather than a sentence they have to go and ask about.
func TestTheAnswersReachTheForge(t *testing.T) {
	team := newInstance(t)
	team.claim()
	team.createOrg("codigosh")
	team.createRepo("codigosh", "lastboard")

	public := team.anonymous()
	id, _ := public.submitWith("codigosh", "lastboard", "bug", "crash on import", map[string]string{
		"version": "1.4.2", "steps": "1. open it\n2. import",
	})

	it, err := (item.Store{DB: team.db()}).ByPublicID(id)
	if err != nil {
		t.Fatal(err)
	}

	body := bridge.IssueBody(it)
	if !strings.Contains(body, "**Version:** 1.4.2") {
		t.Errorf("the issue does not carry the version:\n%s", body)
	}
	if !strings.Contains(body, "**Steps to reproduce:** 1. open it") {
		t.Errorf("the issue does not carry the steps:\n%s", body)
	}
	if strings.Index(body, "**Version:**") > strings.Index(body, "**Steps to reproduce:**") {
		t.Errorf("the answers arrive in the wrong order:\n%s", body)
	}
}

// TestALanguageFollowsTheAccount.
//
// Not the browser. Somebody who chooses a language on a desktop has chosen
// nothing at all if it lives in that browser's storage, and an account created
// by an administrator lands in whatever its first browser asks for — which is
// how a first sign-in arrives in a language nobody picked, with the setting to
// change it written in that language.
func TestALanguageFollowsTheAccount(t *testing.T) {
	team := newInstance(t)
	team.claim()

	resp := team.do(http.MethodPost, "/api/people", map[string]any{
		"username": "greta", "password": "a good long passphrase",
		"display_name": "Greta", "language": "de",
	})
	body := decode(t, resp)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("creating the account returned %d: %v", resp.StatusCode, body)
	}

	them := team.anonymous()
	resp = them.do(http.MethodPost, "/api/session",
		map[string]string{"username": "greta", "password": "a good long passphrase"})
	_ = resp.Body.Close()

	resp = them.do(http.MethodGet, "/api/me", nil)
	if got := decode(t, resp)["language"]; got != "de" {
		t.Errorf("the account does not carry the language it was created with: %v", got)
	}

	// And they can change it themselves.
	resp = them.do(http.MethodPost, "/api/me/language", map[string]string{"language": "ja"})
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("changing the language returned %d", resp.StatusCode)
	}
	resp = them.do(http.MethodGet, "/api/me", nil)
	if got := decode(t, resp)["language"]; got != "ja" {
		t.Errorf("the change did not take: %v", got)
	}

	// A language this build does not ship is refused rather than stored: the
	// value ends up in <html lang>, which is what a screen reader pronounces by.
	resp = them.do(http.MethodPost, "/api/me/language", map[string]string{"language": "klingon"})
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("a language that does not exist was accepted: %d", resp.StatusCode)
	}

	// Empty is not a language: it means "not chosen", and hands the decision
	// back to the browser.
	resp = them.do(http.MethodPost, "/api/me/language", map[string]string{"language": ""})
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("clearing the language was refused: %d", resp.StatusCode)
	}
}

// TestTheServerFactsAreForWhoeverRunsIt.
//
// The About screen reports how long the process has been up, how large the
// database is and what is left on the disk under it. None of that is secret
// exactly, and none of it is the public's business either.
func TestTheServerFactsAreForWhoeverRunsIt(t *testing.T) {
	team := newInstance(t)
	team.claim()

	resp := team.do(http.MethodGet, "/api/server", nil)
	facts := decode(t, resp)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("reading the server facts returned %d", resp.StatusCode)
	}
	for _, key := range []string{"version", "uptime_seconds", "database_bytes", "memory_bytes"} {
		if _, ok := facts[key]; !ok {
			t.Errorf("the About screen has no %s to show: %v", key, facts)
		}
	}
	// The database is on disk and has rows in it, so a zero here means the file
	// was not found rather than that it is empty.
	if size, _ := facts["database_bytes"].(float64); size <= 0 {
		t.Errorf("the database reports %v bytes", facts["database_bytes"])
	}

	// Somebody with a session but no run of the place. The public being turned
	// away proves nothing here: the route is private, so the session middleware
	// answers before the rule is ever consulted.
	accounts := account.Store{DB: team.db()}
	if _, err := accounts.Create("helper", "a good long passphrase", false, team.now()); err != nil {
		t.Fatal(err)
	}
	them := team.anonymous()
	resp = them.do(http.MethodPost, "/api/session",
		map[string]string{"username": "helper", "password": "a good long passphrase"})
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("the helper could not sign in: %d", resp.StatusCode)
	}
	resp = them.do(http.MethodGet, "/api/server", nil)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("somebody who does not run this server can read its disk: %d", resp.StatusCode)
	}

	public := team.anonymous()
	resp = public.do(http.MethodGet, "/api/server", nil)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized && resp.StatusCode != http.StatusForbidden {
		t.Errorf("the free space on the operator's disk is public: %d", resp.StatusCode)
	}
}

// TestALogoTheSizeItIsAllowedToBeGoesThrough.
//
// The endpoint bounds the logo at 96 KiB and bounded the request body at 16,
// so anything over about twelve was cut off before it could be measured and
// came back "the request was not understood" — a true answer to a question
// nobody asked. Two bounds on the same value, in two files, disagreeing.
//
// The sizes here are the real ones on purpose: a test with a 1 KB logo passes
// on both sides of the bug.
func TestALogoTheSizeItIsAllowedToBeGoesThrough(t *testing.T) {
	team := newInstance(t)
	team.claim()

	logo := func(kb int) string {
		return "data:image/png;base64," + strings.Repeat("A", kb*1024)
	}

	resp := team.do(http.MethodPost, "/api/instance",
		map[string]string{"name": "Acme", "logo": logo(80), "accent": ""})
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("a logo of 80 KiB, under the 96 the endpoint allows, returned %d", resp.StatusCode)
	}

	resp = team.do(http.MethodGet, "/api/instance", nil)
	if got, _ := decode(t, resp)["logo"].(string); len(got) < 80*1024 {
		t.Errorf("the logo came back %d bytes shorter than it went in", 80*1024-len(got))
	}

	// And one that really is too large says so, rather than being cut off and
	// reported as unreadable.
	resp = team.do(http.MethodPost, "/api/instance",
		map[string]string{"name": "Acme", "logo": logo(120), "accent": ""})
	body := decode(t, resp)
	if resp.StatusCode != http.StatusBadRequest || body["error"] != "error.image_too_big" {
		t.Errorf("an oversized logo answered %d %v, not that it is too big", resp.StatusCode, body)
	}
}

// TestAFailureLeavesALine.
//
// Forty-six of the seventy-five places that answer "error.internal" logged
// nothing, so a failure gave an operator one sentence on screen — "something
// went wrong here" — and an empty log to send with the report. This is the
// difference between a bug that can be diagnosed and one that has to be
// guessed at, and it was noticed because a real report could not be traced.
func TestAFailureLeavesALine(t *testing.T) {
	var written bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&written, &slog.HandlerOptions{Level: slog.LevelError})))
	t.Cleanup(func() { slog.SetDefault(previous) })

	recorder := httptest.NewRecorder()
	writeError(recorder, http.StatusInternalServerError, "error.internal")

	line := written.String()
	if !strings.Contains(line, "error.internal") {
		t.Errorf("a 500 wrote no line naming the key: %q", line)
	}
	// The caller's file and line, which is the whole point: "something failed"
	// with no location is the state this replaced.
	if !strings.Contains(line, "portal_test.go:") {
		t.Errorf("the line does not say where the failure came from: %q", line)
	}

	// A refusal is not a failure. Logging every 400 would bury the ones that
	// matter under everything a bad request can do.
	written.Reset()
	writeError(httptest.NewRecorder(), http.StatusBadRequest, "error.bad_request")
	if written.Len() != 0 {
		t.Errorf("a 400 wrote to the error log: %q", written.String())
	}
}

// TestTheTeamCanReadWhatTheyHaveToDecideAbout.
//
// A report waiting for review is exactly the one somebody has to read before
// publishing it, and the public page answered "not here" to everybody who was
// not its author — the team included. The queue could be worked through only by
// trusting the title.
//
// The other half of the check matters as much: a stranger still gets "not
// here", and nothing about it says the report exists.
func TestTheTeamCanReadWhatTheyHaveToDecideAbout(t *testing.T) {
	team := newInstance(t)
	team.claim()
	team.createOrg("codigosh")
	team.createRepo("codigosh", "lastboard")

	public := team.anonymous()
	id, _ := public.submit("codigosh", "lastboard", "bug", "crashes on import")

	resp := team.do(http.MethodGet, "/api/items/"+id, nil)
	body := decode(t, resp)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("the team asking for a report waiting for review got %d: %v", resp.StatusCode, body)
	}
	if body["moderation"] != "pending" || body["title"] != "crashes on import" {
		t.Errorf("what came back was %v", body)
	}

	stranger := team.anonymous()
	resp = stranger.do(http.MethodGet, "/api/items/"+id, nil)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("a stranger asking for the same report got %d", resp.StatusCode)
	}
}

// TestEveryResponseCarriesAPolicy.
//
// This was in the plan for the web layer from the start — a policy on every
// response, alongside the rest of the middleware — and it was never written.
// Nothing said so: no document promised it, so no document was lying, and the
// only way to notice an absent header is to ask for it.
//
// What it checks is the shape rather than the string: script-src exists and
// does not allow inline, because that is the directive an injected <script>
// runs under, and the page's own module is loaded by URL. style-src keeps
// 'unsafe-inline' on purpose — an inline style attribute has no nonce, the
// interface sets dozens of them, and a policy written to look strict and break
// the product is worse than the one it replaced.
func TestEveryResponseCarriesAPolicy(t *testing.T) {
	team := newInstance(t)
	team.claim()

	for _, path := range []string{"/", "/api/orgs", "/healthz"} {
		resp := team.do(http.MethodGet, path, nil)
		policy := resp.Header.Get("Content-Security-Policy")
		_ = resp.Body.Close()

		if policy == "" {
			t.Errorf("%s answered with no Content-Security-Policy", path)
			continue
		}
		if !strings.Contains(policy, "script-src 'self'") ||
			strings.Contains(policy, "script-src 'self' 'unsafe-inline'") {
			t.Errorf("%s allows inline script: %q", path, policy)
		}
		if !strings.Contains(policy, "frame-ancestors 'none'") {
			t.Errorf("%s can be framed: %q", path, policy)
		}
		// The one that took work rather than a word: no 'unsafe-inline'
		// anywhere, style included. Every inline style the interface used to
		// write is a class or a rule in the sheet the document adopts.
		if strings.Contains(policy, "unsafe-inline") || strings.Contains(policy, "unsafe-eval") {
			t.Errorf("%s allows inline: %q", path, policy)
		}
		if !strings.Contains(policy, "default-src 'none'") {
			t.Errorf("%s inherits for a directive nobody named: %q", path, policy)
		}
		if !strings.Contains(policy, "img-src 'self' data:") {
			t.Errorf("%s forbids the data URLs avatars are stored as: %q", path, policy)
		}
		if resp.Header.Get("X-Content-Type-Options") != "nosniff" {
			t.Errorf("%s lets the browser guess the type", path)
		}
		if resp.Header.Get("Referrer-Policy") != "no-referrer" {
			t.Errorf("%s leaks the address — which carries a follow token — as a referrer", path)
		}
	}
}

// TestTheWaitingCountStopsAtTheDoor.
//
// The number beside "Reports" is the first thing a collaborator sees, and a
// number is a claim: this much is waiting for you. Counted globally it is wrong
// twice — it sends somebody to a queue that is empty for them, and it tells
// them how busy places they cannot open are.
//
// Two boards, one report waiting on each, and an account that triages one of
// them. The count has to be one.
func TestTheWaitingCountStopsAtTheDoor(t *testing.T) {
	team := newInstance(t)
	team.claim()
	team.createOrg("codigosh")
	team.createRepo("codigosh", "lastboard")
	team.createRepo("codigosh", "lastdav")

	accounts := account.Store{DB: team.db()}
	helper, err := accounts.Create("helper", "a good long passphrase", false, team.now())
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

	public := team.anonymous()
	public.submit("codigosh", "lastboard", "idea", "waiting in my repository")
	public.submit("codigosh", "lastdav", "idea", "waiting next door")

	waiting := func(c *instance, who string) map[string]any {
		t.Helper()
		body := decode(t, c.do(http.MethodGet, "/api/me", nil))
		pending, ok := body["pending"].(map[string]any)
		if !ok {
			t.Fatalf("%s: /api/me did not report what is waiting: %v", who, body["pending"])
		}
		return pending
	}

	// The administrator sees both boards, so both reports.
	if got := waiting(team, "the administrator")["reports"]; got != float64(2) {
		t.Errorf("the administrator was told %v reports are waiting, want 2", got)
	}

	helperClient := team.anonymous()
	resp := helperClient.do(http.MethodPost, "/api/session", map[string]string{
		"username": "helper", "password": "a good long passphrase",
	})
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("the collaborator could not sign in: %d", resp.StatusCode)
	}

	// The collaborator triages one board, so one report — never the one next
	// door, whose existence this number must not reveal either.
	if got := waiting(helperClient, "the collaborator")["reports"]; got != float64(1) {
		t.Errorf("the collaborator was told %v reports are waiting, want 1 — the\n"+
			"board next door has one too, and they cannot open it", got)
	}
}

// TestTheQueueThatSpansBoardsStopsAtTheDoor.
//
// The queue without a board in its address is the one that makes the panel
// usable: five boards used to mean asking five times to find the one with
// something waiting. Spanning boards is exactly what makes it worth checking
// where it stops — a message from a place this account cannot open must not
// appear in it, and neither must the name of that place.
func TestTheQueueThatSpansBoardsStopsAtTheDoor(t *testing.T) {
	team := newInstance(t)
	team.claim()
	team.createOrg("codigosh")
	team.createRepo("codigosh", "lastboard")
	team.createRepo("codigosh", "lastdav")

	accounts := account.Store{DB: team.db()}
	helper, err := accounts.Create("helper", "a good long passphrase", false, team.now())
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

	// A published report on each board, with a message waiting on each.
	public := team.anonymous()
	for _, board := range []struct{ repo, title, message string }{
		{"lastboard", "dark mode", "mine: it happens on the phone too"},
		{"lastdav", "next door", "theirs: this one is not yours to read"},
	} {
		id, _ := public.submit("codigosh", board.repo, "idea", board.title)
		resp := team.do(http.MethodPost, "/api/items/"+id+"/moderation", map[string]string{"state": "published"})
		_ = resp.Body.Close()
		resp = public.do(http.MethodPost, "/api/items/"+id+"/comments", map[string]string{"body": board.message})
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusAccepted {
			t.Fatalf("commenting on %s returned %d", board.repo, resp.StatusCode)
		}
	}

	read := func(c *instance, who string) []any {
		t.Helper()
		body := decode(t, c.do(http.MethodGet, "/api/panel/comments", nil))
		comments, _ := body["comments"].([]any)
		return comments
	}

	// The administrator sees both, each naming its board.
	all := read(team, "the administrator")
	if len(all) != 2 {
		t.Fatalf("the administrator was shown %d messages, want 2", len(all))
	}
	for _, c := range all {
		row, _ := c.(map[string]any)
		board, _ := row["board"].(map[string]any)
		if board["repository"] == nil {
			t.Error("a message did not say which board it came from")
		}
	}

	helperClient := team.anonymous()
	resp := helperClient.do(http.MethodPost, "/api/session", map[string]string{
		"username": "helper", "password": "a good long passphrase",
	})
	_ = resp.Body.Close()

	mine := read(helperClient, "the collaborator")
	if len(mine) != 1 {
		t.Fatalf("the collaborator was shown %d messages, want 1", len(mine))
	}
	row, _ := mine[0].(map[string]any)
	if body, _ := row["body"].(string); !strings.HasPrefix(body, "mine:") {
		t.Errorf("the collaborator was shown the message from next door: %q", body)
	}
	if board, _ := row["board"].(map[string]any); board["repository"] != "lastboard" {
		t.Errorf("the message came back on board %v, want lastboard", board["repository"])
	}
}

// TestTheTeamCanRewriteAReportAndOnlyOnTheirOwnBoard.
//
// Editing somebody else's words is the kind of power that has to sit behind the
// same door as every other decision about a report, not a wider one. So this
// checks the two halves together: that it works at all, and that it stops where
// moderation stops.
func TestTheTeamCanRewriteAReportAndOnlyOnTheirOwnBoard(t *testing.T) {
	team := newInstance(t)
	team.claim()
	team.createOrg("codigosh")
	team.createRepo("codigosh", "lastboard")
	team.createRepo("codigosh", "lastdav")

	public := team.anonymous()
	mine, _ := public.submit("codigosh", "lastboard", "bug", "its broken")
	theirs, _ := public.submit("codigosh", "lastdav", "bug", "next door")

	// The team fixes a title that says nothing.
	resp := team.do(http.MethodPatch, "/api/items/"+mine, map[string]string{
		"title": "Importing a folder crashes on the second run",
		"body":  "Steps: import twice.",
	})
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("the team could not edit a report: %d", resp.StatusCode)
	}

	body := decode(t, team.do(http.MethodGet, "/api/items/"+mine, nil))
	if body["title"] != "Importing a folder crashes on the second run" {
		t.Errorf("the title came back as %v", body["title"])
	}
	if body["body"] != "Steps: import twice." {
		t.Errorf("the description came back as %v", body["body"])
	}

	// An empty title is refused, the same as on the public form.
	resp = team.do(http.MethodPatch, "/api/items/"+mine, map[string]string{"title": "  ", "body": "x"})
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("an empty title was accepted: %d", resp.StatusCode)
	}

	// And a collaborator of one board cannot rewrite the one next door.
	accounts := account.Store{DB: team.db()}
	helper, err := accounts.Create("helper", "a good long passphrase", false, team.now())
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
	helperClient := team.anonymous()
	resp = helperClient.do(http.MethodPost, "/api/session", map[string]string{
		"username": "helper", "password": "a good long passphrase",
	})
	_ = resp.Body.Close()

	resp = helperClient.do(http.MethodPatch, "/api/items/"+theirs, map[string]string{
		"title": "rewritten from next door", "body": "x",
	})
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("a collaborator rewrote a report on a board they do not triage: %d", resp.StatusCode)
	}

	// And nobody without a session at all.
	resp = public.do(http.MethodPatch, "/api/items/"+mine, map[string]string{"title": "x", "body": "y"})
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized && resp.StatusCode != http.StatusForbidden {
		t.Errorf("a stranger rewrote a report: %d", resp.StatusCode)
	}
}

// TestAnAnsweredReportClosesItsConversation: a visitor's comment is refused with
// the same answer once the report is answered, a bug as much as an idea, and the
// page is told so it can take the form away. The reporter and the team keep the
// form for 48 hours, are told when it ends, and then lose it too.
func TestAnAnsweredReportClosesItsConversation(t *testing.T) {
	team := newInstance(t)
	team.claim()
	team.createOrg("codigosh")
	team.createRepo("codigosh", "lastboard")
	public := team.anonymous()

	for _, tc := range []struct{ kind, answer string }{
		{"idea", string(item.StatusDone)},
		{"bug", string(item.StatusClosed)},
	} {
		id, token := public.submit("codigosh", "lastboard", tc.kind, "something "+tc.kind)
		resp := team.do(http.MethodPost, "/api/items/"+id+"/moderation", map[string]string{"state": "published"})
		_ = resp.Body.Close()

		if open := decode(t, public.do(http.MethodGet, "/api/items/"+id, nil)); open["commentable"] != true {
			t.Errorf("an open %s says commentable=%v", tc.kind, open["commentable"])
		}
		resp = public.do(http.MethodPost, "/api/items/"+id+"/comments", map[string]string{"body": "me too"})
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusAccepted {
			t.Fatalf("a comment on an open %s returned %d", tc.kind, resp.StatusCode)
		}

		resp = team.do(http.MethodPost, "/api/items/"+id+"/status", map[string]string{"status": tc.answer})
		_ = resp.Body.Close()
		answered := team.now()

		// A visitor: shut at once.
		if closed := decode(t, public.do(http.MethodGet, "/api/items/"+id, nil)); closed["commentable"] != false || closed["comment_until"] != nil {
			t.Errorf("an answered %s says commentable=%v until=%v to a visitor", tc.kind, closed["commentable"], closed["comment_until"])
		}
		resp = public.do(http.MethodPost, "/api/items/"+id+"/comments", map[string]string{"body": "still happens"})
		body := decode(t, resp)
		if resp.StatusCode != http.StatusConflict || body["error"] != "error.conversation_closed" {
			t.Errorf("a visitor's comment on an answered %s returned %d %v", tc.kind, resp.StatusCode, body)
		}

		// The reporter and the team: open, and told until when.
		until := float64(answered.Add(item.GraceAfterClosing).Unix())
		mine := decode(t, public.do(http.MethodGet, "/api/items/"+id+"?k="+token, nil))
		if mine["commentable"] != true || mine["comment_until"] != until {
			t.Errorf("an answered %s says commentable=%v until=%v to its reporter, want true until %v", tc.kind, mine["commentable"], mine["comment_until"], until)
		}
		theirs := decode(t, team.do(http.MethodGet, "/api/items/"+id, nil))
		if theirs["commentable"] != true || theirs["comment_until"] != until {
			t.Errorf("an answered %s says commentable=%v until=%v to the team", tc.kind, theirs["commentable"], theirs["comment_until"])
		}
		resp = public.do(http.MethodPost, "/api/items/"+id+"/comments", map[string]string{"body": "it is fixed, thanks", "token": token})
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusAccepted {
			t.Errorf("the reporter's comment inside the window on an answered %s returned %d", tc.kind, resp.StatusCode)
		}
		resp = team.do(http.MethodPost, "/api/items/"+id+"/reply", map[string]string{"body": "one more thing"})
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusCreated {
			t.Errorf("the team's reply inside the window on an answered %s returned %d", tc.kind, resp.StatusCode)
		}

		// And once it is over, for everybody.
		team.tick.advance(item.GraceAfterClosing + time.Minute)
		for who, send := range map[string]func() *http.Response{
			"the reporter": func() *http.Response {
				return public.do(http.MethodPost, "/api/items/"+id+"/comments", map[string]string{"body": "late", "token": token})
			},
			"the team": func() *http.Response {
				return team.do(http.MethodPost, "/api/items/"+id+"/reply", map[string]string{"body": "late"})
			},
		} {
			resp := send()
			body := decode(t, resp)
			if resp.StatusCode != http.StatusConflict || body["error"] != "error.conversation_closed" {
				t.Errorf("%s writing on an answered %s after the window returned %d %v", who, tc.kind, resp.StatusCode, body)
			}
		}
		late := decode(t, public.do(http.MethodGet, "/api/items/"+id+"?k="+token, nil))
		if late["commentable"] != false || late["comment_until"] != nil {
			t.Errorf("the reporter is still offered a form after the window: %v %v", late["commentable"], late["comment_until"])
		}
	}
}

// TestTheFrontPageCardCountsWhatTheBoardPageCounts: the number beside each kind
// on a board's card is what the two tabs of the page behind it add up to. Two
// paths computing one magnitude are how a screen ends up contradicting itself,
// so what is asserted is the relation, not either figure.
func TestTheFrontPageCardCountsWhatTheBoardPageCounts(t *testing.T) {
	team := newInstance(t)
	team.claim()
	team.createOrg("codigosh")
	team.createRepo("codigosh", "lastboard")
	public := team.anonymous()

	publish := func(kind, title string) string {
		id, _ := public.submit("codigosh", "lastboard", kind, title)
		resp := team.do(http.MethodPost, "/api/items/"+id+"/moderation", map[string]string{"state": "published"})
		_ = resp.Body.Close()
		return id
	}
	publish("idea", "dark mode")
	publish("idea", "export")
	answered := publish("idea", "tags")
	publish("bug", "crash")
	closedBug := publish("bug", "freeze")
	public.submit("codigosh", "lastboard", "bug", "still waiting") // never published

	for id, answer := range map[string]string{answered: string(item.StatusDone), closedBug: string(item.StatusClosed)} {
		resp := team.do(http.MethodPost, "/api/items/"+id+"/status", map[string]string{"status": answer})
		_ = resp.Body.Close()
	}

	card := map[string]any{}
	content, _ := decode(t, public.do(http.MethodGet, "/api/landing", nil))["content"].(map[string]any)
	for _, rows := range content {
		list, _ := rows.([]any)
		for _, row := range list {
			if b, _ := row.(map[string]any); b["slug"] == "lastboard" && b["org"] == "codigosh" {
				card = b
			}
		}
	}
	if card["total_ideas"] != float64(3) || card["total_bugs"] != float64(2) {
		t.Fatalf("the card says %v ideas and %v bugs, want 3 and 2 (the answered ones count; one that was never published does not)",
			card["total_ideas"], card["total_bugs"])
	}

	for kind, key := range map[string]string{"idea": "total_ideas", "bug": "total_bugs"} {
		page := decode(t, public.do(http.MethodGet, "/api/orgs/codigosh/repos/lastboard/items?kind="+kind, nil))
		counts, _ := page["counts"].(map[string]any)
		open, _ := counts["open"].(float64)
		closed, _ := counts["closed"].(float64)
		if open+closed != card[key] {
			t.Errorf("the card says %v %ss and the board's two tabs add up to %v", card[key], kind, open+closed)
		}
	}
}

// TestAReportAtEveryLimitInJapaneseIsTaken.
//
// The limits on the words are in characters and the body of a request is in
// bytes, and a language that takes three or four bytes a letter put a report
// at its limits well past 64 KiB: it was refused as a malformed request, with
// nothing to say why. Sent through the real handler with every field at its
// limit, because the model alone passing says nothing about the wire.
func TestAReportAtEveryLimitInJapaneseIsTaken(t *testing.T) {
	team := newInstance(t)
	team.claim()
	team.createOrg("codigosh")
	team.createRepo("codigosh", "lastboard")
	public := team.anonymous()

	ja := func(n int) string { return strings.Repeat("ぜ", n) }
	base := "/api/orgs/codigosh/repos/lastboard"
	form := decode(t, public.do(http.MethodGet, base+"/form", nil))
	public.tick.advance(5 * time.Second)

	resp := public.do(http.MethodPost, base+"/items", map[string]any{
		"kind": "bug", "title": ja(200), "body": ja(20_000), "token": form["token"],
		"fields": map[string]string{"version": ja(4_000), "steps": ja(4_000)},
	})
	body := decode(t, resp)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("a Japanese report at every limit answered %d %v", resp.StatusCode, body)
	}
}
