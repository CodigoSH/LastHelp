package web

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"git.codigosh.com/CodigoSH/LastHelp/internal/core/account"
	"git.codigosh.com/CodigoSH/LastHelp/internal/core/authz"
	"git.codigosh.com/CodigoSH/LastHelp/internal/core/landing"
)

// frontPage is the page as whoever is asking sees it.
func frontPage(t *testing.T, who *instance, query string) map[string]any {
	t.Helper()
	return decode(t, who.do(http.MethodGet, "/api/landing"+query, nil))
}

func bandIDs(body map[string]any) []string {
	var ids []string
	bands, _ := body["bands"].([]any)
	for _, b := range bands {
		m, _ := b.(map[string]any)
		ids = append(ids, m["id"].(string))
	}
	return ids
}

// words is a one-band page with one paragraph.
func words(body string) map[string]any {
	return map[string]any{"bands": []map[string]any{{
		"id": "b", "columns": "1",
		"cells": [][]map[string]any{{{"id": "t", "kind": "text", "body": body}}},
	}}}
}

// TestComposingTheFrontPage.
//
// The front page is the one screen a stranger sees, so the halves are checked
// together: that anybody may read it, that what is being edited is never what
// a visitor is shown, and that only an administrator may change it.
func TestComposingTheFrontPage(t *testing.T) {
	i := newInstance(t)
	i.claim()
	visitor := i.anonymous()

	// Before anybody composes anything, it is the default page, band by band.
	if got := strings.Join(bandIDs(frontPage(t, visitor, "")), " "); got != "top where activity how" {
		t.Fatalf("an untouched installation offered %q", got)
	}

	// A draft is kept and not shown.
	if got := status(t, i.do(http.MethodPut, "/api/landing/draft", words("<p>We read everything.</p>"))); got != http.StatusOK {
		t.Fatalf("the administrator could not keep a draft: %d", got)
	}
	if got := strings.Join(bandIDs(frontPage(t, visitor, "")), " "); got != "top where activity how" {
		t.Errorf("a visitor was shown the draft: %q", got)
	}
	if got := strings.Join(bandIDs(frontPage(t, visitor, "?draft=1")), " "); got != "top where activity how" {
		t.Errorf("a visitor asking for the draft was shown it: %q", got)
	}
	// Nor is the administrator, outside the editor: looking at their own
	// page, they see what everybody else sees.
	if got := strings.Join(bandIDs(frontPage(t, i, "")), " "); got != "top where activity how" {
		t.Errorf("the administrator was shown the draft outside the editor: %q", got)
	}
	editing := frontPage(t, i, "?draft=1")
	if got := strings.Join(bandIDs(editing), " "); got != "b" || editing["has_draft"] != true {
		t.Errorf("the administrator was not given the draft: %q %v", got, editing["has_draft"])
	}

	// A page nothing can draw is refused, and the draft is left as it was.
	bad := map[string]any{"bands": []map[string]any{{"id": "b", "columns": "1",
		"cells": [][]map[string]any{{{"id": "x", "kind": "iframe"}}}}}}
	if got := status(t, i.do(http.MethodPut, "/api/landing/draft", bad)); got != http.StatusBadRequest {
		t.Errorf("a module nothing can draw was accepted: %d", got)
	}
	if got := strings.Join(bandIDs(frontPage(t, i, "?draft=1")), " "); got != "b" {
		t.Errorf("a refused page changed the draft: %q", got)
	}

	// Published, it is what everybody sees, and there is no draft any more.
	if got := status(t, i.do(http.MethodPost, "/api/landing/publish", nil)); got != http.StatusOK {
		t.Fatalf("publishing: %d", got)
	}
	shown := frontPage(t, visitor, "")
	if got := strings.Join(bandIDs(shown), " "); got != "b" {
		t.Errorf("a visitor was not shown the published page: %q", got)
	}
	if frontPage(t, i, "")["has_draft"] != false {
		t.Error("a draft was left behind after publishing")
	}

	// Discarding a draft puts nothing back, because nothing was touched.
	_ = status(t, i.do(http.MethodPut, "/api/landing/draft", words("<p>Second thoughts.</p>")))
	if got := status(t, i.do(http.MethodDelete, "/api/landing/draft", nil)); got != http.StatusOK {
		t.Fatalf("discarding: %d", got)
	}
	if frontPage(t, i, "")["has_draft"] != false {
		t.Error("a discarded draft is still there")
	}
	if got := strings.Join(bandIDs(frontPage(t, visitor, "")), " "); got != "b" {
		t.Errorf("discarding a draft changed the published page: %q", got)
	}

	// A stranger cannot touch any of it; this one is stopped at the door.
	for _, call := range [][2]string{
		{http.MethodPut, "/api/landing/draft"}, {http.MethodPost, "/api/landing/publish"},
		{http.MethodDelete, "/api/landing/draft"}, {http.MethodPost, "/api/landing/restore"},
	} {
		got := status(t, visitor.do(call[0], call[1], words("<p>x</p>")))
		if got != http.StatusUnauthorized && got != http.StatusForbidden {
			t.Errorf("a stranger reached %s %s: %d", call[0], call[1], got)
		}
	}

	// And neither can somebody who IS signed in and is not an administrator:
	// the front page belongs to whoever runs the server.
	accounts := account.Store{DB: i.db()}
	helper, err := accounts.Create("helper", "a good long passphrase", "Helper", false, i.now())
	if err != nil {
		t.Fatal(err)
	}
	var boardID int64
	i.createOrg("codigosh")
	i.createRepo("codigosh", "lastboard")
	if err := i.db().QueryRow(`SELECT id FROM repository WHERE slug = 'lastboard'`).Scan(&boardID); err != nil {
		t.Fatal(err)
	}
	if err := accounts.Grant(helper.ID, authz.Repository(boardID, 0), authz.RoleCollaborator, i.now()); err != nil {
		t.Fatal(err)
	}
	helperClient := i.anonymous()
	_ = status(t, helperClient.do(http.MethodPost, "/api/session", map[string]string{
		"username": "helper", "password": "a good long passphrase",
	}))
	for _, call := range [][2]string{
		{http.MethodPut, "/api/landing/draft"}, {http.MethodPost, "/api/landing/publish"},
	} {
		if got := status(t, helperClient.do(call[0], call[1], words("<p>x</p>"))); got != http.StatusForbidden {
			t.Errorf("a collaborator reached %s %s: %d", call[0], call[1], got)
		}
	}
	if _, told := frontPage(t, helperClient, "")["has_draft"]; told {
		t.Error("a collaborator was told whether there is a draft")
	}
}

// TestRestoringTheDefaultStoresNoPage.
//
// The default page is read fresh, never stored, so a better one in a later
// release reaches everybody who never changed theirs. An operator who restores
// it and publishes is one of them again.
func TestRestoringTheDefaultStoresNoPage(t *testing.T) {
	i := newInstance(t)
	i.claim()
	_ = status(t, i.do(http.MethodPut, "/api/landing/draft", words("<p>Mine.</p>")))
	_ = status(t, i.do(http.MethodPost, "/api/landing/publish", nil))

	if got := status(t, i.do(http.MethodPost, "/api/landing/restore", nil)); got != http.StatusOK {
		t.Fatalf("restoring the default: %d", got)
	}
	if got := strings.Join(bandIDs(frontPage(t, i.anonymous(), "")), " "); got != "b" {
		t.Errorf("restoring changed the published page before it was published: %q", got)
	}
	_ = status(t, i.do(http.MethodPost, "/api/landing/publish", nil))

	var stored string
	if err := i.db().QueryRow(`SELECT value FROM settings WHERE key = 'landing_layout'`).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if stored != "" {
		t.Errorf("the default page was stored as a page: %.60s", stored)
	}
	if got := strings.Join(bandIDs(frontPage(t, i.anonymous(), "")), " "); got != "top where activity how" {
		t.Errorf("after restoring, a visitor sees %q", got)
	}
}

// TestAReportsModuleShowsWhatItIsAskedFor.
//
// Proved here rather than in the browser pass: that journey reaches the front
// page with one published idea and no published bug, so a list looks identical
// whether it filters or not.
func TestAReportsModuleShowsWhatItIsAskedFor(t *testing.T) {
	i := newInstance(t)
	i.claim()
	i.createOrg("acme")
	i.createRepo("acme", "atlas")

	idea, _ := i.submit("acme", "atlas", "idea", "dark mode for the dashboard")
	planned, _ := i.submit("acme", "atlas", "idea", "export to csv")
	bug, _ := i.submit("acme", "atlas", "bug", "the sidebar forgets its width")
	for _, id := range []string{idea, planned, bug} {
		if got := status(t, i.do(http.MethodPost, "/api/items/"+id+"/moderation", map[string]string{"state": "published"})); got != http.StatusOK {
			t.Fatalf("publishing %s: %d", id, got)
		}
	}
	if got := status(t, i.do(http.MethodPost, "/api/items/"+planned+"/status", map[string]string{"status": "planned"})); got != http.StatusOK {
		t.Fatalf("planning: %d", got)
	}
	if got := status(t, i.do(http.MethodPost, "/api/items/"+bug+"/status", map[string]string{"status": "closed"})); got != http.StatusOK {
		t.Fatalf("closing: %d", got)
	}

	list := func(id, show string) map[string]any {
		return map[string]any{"id": id, "kind": "reports", "show": show}
	}
	composed := map[string]any{"bands": []map[string]any{{"id": "b", "columns": "1-1-1-1",
		"cells": [][]map[string]any{
			{list("voted", "most_voted")}, {list("done", "recently_done")},
			{list("latest", "latest")}, {list("planned", "planned")},
		}}}}
	_ = status(t, i.do(http.MethodPut, "/api/landing/draft", composed))
	if got := status(t, i.do(http.MethodPost, "/api/landing/publish", nil)); got != http.StatusOK {
		t.Fatalf("publishing the page: %d", got)
	}

	content, _ := frontPage(t, i.anonymous(), "")["content"].(map[string]any)
	titles := func(id string) string {
		rows, _ := content[id].([]any)
		var out []string
		for _, r := range rows {
			m, _ := r.(map[string]any)
			out = append(out, m["title"].(string))
		}
		return strings.Join(out, ", ")
	}
	if got := titles("voted"); strings.Contains(got, "sidebar") || got == "" {
		t.Errorf("most voted carried %q: ideas only", got)
	}
	if got := titles("done"); got != "the sidebar forgets its width" {
		t.Errorf("recently resolved carried %q", got)
	}
	if got := titles("planned"); got != "export to csv" {
		t.Errorf("planned carried %q", got)
	}
	if got := titles("latest"); strings.Count(got, ",") != 2 {
		t.Errorf("latest carried %q, want all three", got)
	}
}

// TestAStrangerIsHandedThePageAndNothingElse.
//
// The board list travels with the page for whoever is going to compose it, and
// for nobody else: it is the picker in the editor, not something a visitor has
// any use for. Nor is whether a draft exists.
func TestAStrangerIsHandedThePageAndNothingElse(t *testing.T) {
	i := newInstance(t)
	i.claim()
	body := frontPage(t, i.anonymous(), "")
	if _, carried := body["boards"]; carried {
		t.Errorf("a stranger was handed the board list: %v", body["boards"])
	}
	if _, carried := body["has_draft"]; carried {
		t.Errorf("a stranger was told whether there is a draft")
	}
}

// TestAPictureOnlyTheDraftUsesIsKept.
//
// A picture dropped into the draft is about to be published, and one only the
// published page uses is still on the screen. Neither may be swept; one nothing
// uses any more is.
func TestAPictureOnlyTheDraftUsesIsKept(t *testing.T) {
	i := newInstance(t)
	i.claim()
	webp := append([]byte("RIFF\x00\x00\x00\x00WEBPVP8 "), make([]byte, 32)...)
	name, err := landing.StoreImage(i.db(), webp, i.now())
	if err != nil {
		t.Fatal(err)
	}
	withPicture := map[string]any{"bands": []map[string]any{{"id": "b", "columns": "1",
		"cells": [][]map[string]any{{{"id": "p", "kind": "image", "image": name, "alt": "The dashboard"}}}}}}
	stored := func() int {
		var n int
		_ = i.db().QueryRow(`SELECT count(*) FROM landing_image`).Scan(&n)
		return n
	}

	if got := status(t, i.do(http.MethodPut, "/api/landing/draft", withPicture)); got != http.StatusOK {
		t.Fatalf("keeping a draft with a picture %q: %d", name, got)
	}
	if stored() != 1 {
		t.Fatal("a picture the draft uses was swept")
	}
	if got := status(t, i.do(http.MethodPost, "/api/landing/publish", nil)); got != http.StatusOK {
		t.Fatalf("publishing the picture: %d", got)
	}
	_ = status(t, i.do(http.MethodPut, "/api/landing/draft", words("<p>No picture.</p>")))
	if stored() != 1 {
		t.Fatal("a picture the published page shows was swept")
	}
	_ = status(t, i.do(http.MethodPost, "/api/landing/publish", nil))
	if stored() != 0 {
		t.Error("a picture no page uses was kept")
	}
}

// TestTheFrontPageSearchFindsWhatIsPublished.
//
// A visitor finds a report before writing it again — and only a published one:
// the front page must never be where something unreviewed appears.
func TestTheFrontPageSearchFindsWhatIsPublished(t *testing.T) {
	i := newInstance(t)
	i.claim()
	i.createOrg("acme")
	i.createRepo("acme", "atlas")
	shown, _ := i.submit("acme", "atlas", "idea", "dark mode for the dashboard")
	_, _ = i.submit("acme", "atlas", "idea", "dark mode for the editor, still waiting")
	if got := status(t, i.do(http.MethodPost, "/api/items/"+shown+"/moderation", map[string]string{"state": "published"})); got != http.StatusOK {
		t.Fatalf("publishing: %d", got)
	}

	body := decode(t, i.anonymous().do(http.MethodGet, "/api/search?q=dar", nil))
	items, _ := body["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("the search found %d reports, want the one published: %v", len(items), items)
	}
	if hit, _ := items[0].(map[string]any); hit["public_id"] != shown || hit["board"] != "atlas" {
		t.Errorf("the search found %v", hit)
	}
	if boards, _ := body["boards"].([]any); len(boards) != 1 {
		t.Errorf("the search did not say where a new report goes: %v", body["boards"])
	}

	// One character asks nothing — and "d" would match: the last word is read
	// as the start of one, so this answers only because of the minimum.
	short := decode(t, i.anonymous().do(http.MethodGet, "/api/search?q=d", nil))
	if items, _ := short["items"].([]any); len(items) != 0 {
		t.Errorf("one character searched: %v", items)
	}
}

// TestThePageHeadCarriesThePageSettings.
//
// A link preview is built by a program that never runs the page's script, so
// the title and description have to be in the HTML the server sends — and the
// operator's words go into attributes, so a quote must not end one.
func TestThePageHeadCarriesThePageSettings(t *testing.T) {
	i := newInstance(t)
	i.claim()

	shell := func() string {
		t.Helper()
		resp := i.anonymous().do(http.MethodGet, "/", nil)
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		return string(body)
	}

	// Nothing chosen: the installation's own name.
	if got := shell(); !strings.Contains(got, "<title>LastHelp</title>") || strings.Contains(got, `name="description"`) {
		t.Fatalf("an untouched installation's head:\n%s", got)
	}

	page := frontPage(t, i, "")
	page["settings"] = map[string]any{"title": `Acme "support"`, "description": "Bugs & ideas <here>"}
	if got := status(t, i.do(http.MethodPut, "/api/landing/draft", page)); got != http.StatusOK {
		t.Fatalf("saving the draft: %d", got)
	}
	// A draft is not what a link shows.
	if got := shell(); strings.Contains(got, "Acme") {
		t.Fatalf("the draft's title reached the head before publishing")
	}
	if got := status(t, i.do(http.MethodPost, "/api/landing/publish", nil)); got != http.StatusOK {
		t.Fatalf("publishing: %d", got)
	}
	got := shell()
	for _, want := range []string{
		"<title>Acme &#34;support&#34;</title>",
		`<meta property="og:title" content="Acme &#34;support&#34;" />`,
		`<meta name="description" content="Bugs &amp; ideas &lt;here&gt;" />`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("the head lacks %s:\n%s", want, got)
		}
	}
	if strings.Contains(got, "og:image") {
		t.Errorf("a picture was announced with no picture and no public address")
	}
}
