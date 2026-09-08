package web

import (
	"fmt"
	"net/http"
	"net/http/cookiejar"
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
	helper, err := accounts.Create("helper", "a good long passphrase", "Helper", false, team.now())
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
	outsider, err := accounts.Create("otherowner", "a good long passphrase", "Other", false, team.now())
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

	// Nobody without a session may ask what statuses are available, let alone
	// set one.
	resp = public.do(http.MethodGet, "/api/items/"+id+"/statuses", nil)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("the status list answered %d to the public", resp.StatusCode)
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
	user, err := accounts.Create(username, "a good long passphrase", "Helper", false, team.now())
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
	neighbour, err := accounts.Create("neighbour", "a good long passphrase", "Neighbour", false, team.now())
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
