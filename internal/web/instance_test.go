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
// together: that anybody may read it, that what the administrator saves is
// what a visitor is shown from then on, and that only an administrator may
// change it.
func TestComposingTheFrontPage(t *testing.T) {
	i := newInstance(t)
	i.claim()
	visitor := i.anonymous()

	// Before anybody composes anything, it is the default page, band by band.
	if got := strings.Join(bandIDs(frontPage(t, visitor, "")), " "); got != "top where activity how" {
		t.Fatalf("an untouched installation offered %q", got)
	}

	// Saved, it is what everybody sees — bands and the foot of the page alike.
	// There is no second copy to publish: the editor edits the page itself, the
	// way the dashboard is edited, and leaving it changes nothing.
	page := words("<p>We read everything.</p>")
	page["settings"] = map[string]any{"footer_text": "Made in Lisbon", "footer_hide_credit": true}
	if got := status(t, i.do(http.MethodPut, "/api/landing", page)); got != http.StatusOK {
		t.Fatalf("the administrator could not save the page: %d", got)
	}
	shown := frontPage(t, visitor, "")
	if got := strings.Join(bandIDs(shown), " "); got != "b" {
		t.Errorf("a visitor was not shown the saved page: %q", got)
	}
	foot, _ := shown["settings"].(map[string]any)
	if foot["footer_text"] != "Made in Lisbon" || foot["footer_hide_credit"] != true {
		t.Errorf("a visitor was not shown the saved foot of the page: %v", foot)
	}

	// A page nothing can draw is refused, and the page is left as it was.
	bad := map[string]any{"bands": []map[string]any{{"id": "b", "columns": "1",
		"cells": [][]map[string]any{{{"id": "x", "kind": "iframe"}}}}}}
	if got := status(t, i.do(http.MethodPut, "/api/landing", bad)); got != http.StatusBadRequest {
		t.Errorf("a module nothing can draw was accepted: %d", got)
	}
	if got := strings.Join(bandIDs(frontPage(t, visitor, "")), " "); got != "b" {
		t.Errorf("a refused page changed the page: %q", got)
	}

	// A stranger cannot touch any of it; this one is stopped at the door.
	for _, call := range [][2]string{
		{http.MethodPut, "/api/landing"}, {http.MethodPost, "/api/landing/restore"},
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
		{http.MethodPut, "/api/landing"}, {http.MethodPost, "/api/landing/restore"},
	} {
		if got := status(t, helperClient.do(call[0], call[1], words("<p>x</p>"))); got != http.StatusForbidden {
			t.Errorf("a collaborator reached %s %s: %d", call[0], call[1], got)
		}
	}
	if got := strings.Join(bandIDs(frontPage(t, visitor, "")), " "); got != "b" {
		t.Errorf("a refused caller changed the page: %q", got)
	}
}

// TestRestoringTheDefaultStoresNoPage.
//
// The default page is read fresh, never stored, so a better one in a later
// release reaches everybody who never changed theirs. An operator who restores
// it is one of them again — and so is one who arranges their page back into
// exactly the default by hand.
func TestRestoringTheDefaultStoresNoPage(t *testing.T) {
	i := newInstance(t)
	i.claim()
	storedPage := func() string {
		var stored string
		if err := i.db().QueryRow(`SELECT value FROM settings WHERE key = 'landing_layout'`).Scan(&stored); err != nil {
			t.Fatal(err)
		}
		return stored
	}

	_ = status(t, i.do(http.MethodPut, "/api/landing", words("<p>Mine.</p>")))
	if storedPage() == "" {
		t.Fatal("a page of the operator's own was not stored")
	}
	if got := status(t, i.do(http.MethodPost, "/api/landing/restore", nil)); got != http.StatusOK {
		t.Fatalf("restoring the default: %d", got)
	}
	if stored := storedPage(); stored != "" {
		t.Errorf("the default page was stored as a page: %.60s", stored)
	}
	if got := strings.Join(bandIDs(frontPage(t, i.anonymous(), "")), " "); got != "top where activity how" {
		t.Errorf("after restoring, a visitor sees %q", got)
	}

	_ = status(t, i.do(http.MethodPut, "/api/landing", words("<p>Mine again.</p>")))
	_ = status(t, i.do(http.MethodPost, "/api/landing/restore", nil))
	def := frontPage(t, i, "")
	if got := status(t, i.do(http.MethodPut, "/api/landing", map[string]any{"bands": def["bands"], "settings": def["settings"]})); got != http.StatusOK {
		t.Fatalf("saving the default by hand: %d", got)
	}
	if stored := storedPage(); stored != "" {
		t.Errorf("the default, saved by hand, was stored as a page: %.60s", stored)
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
	if got := status(t, i.do(http.MethodPut, "/api/landing", composed)); got != http.StatusOK {
		t.Fatalf("saving the page: %d", got)
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
// any use for.
func TestAStrangerIsHandedThePageAndNothingElse(t *testing.T) {
	i := newInstance(t)
	i.claim()
	body := frontPage(t, i.anonymous(), "")
	if _, carried := body["boards"]; carried {
		t.Errorf("a stranger was handed the board list: %v", body["boards"])
	}
	if _, carried := body["share_ready"]; carried {
		t.Errorf("a stranger was handed the editor's settings")
	}
}

// TestAPictureThePageUsesIsKept.
//
// A picture the page shows may not be swept; one nothing uses any more is.
func TestAPictureThePageUsesIsKept(t *testing.T) {
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

	if got := status(t, i.do(http.MethodPut, "/api/landing", withPicture)); got != http.StatusOK {
		t.Fatalf("saving a page with a picture %q: %d", name, got)
	}
	if stored() != 1 {
		t.Fatal("a picture the page shows was swept")
	}
	_ = status(t, i.do(http.MethodPut, "/api/landing", words("<p>No picture.</p>")))
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
	if got := status(t, i.do(http.MethodPut, "/api/landing", page)); got != http.StatusOK {
		t.Fatalf("saving the page: %d", got)
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

// TestTheAppCanBeInstalled: the manifest and the icons it names are served,
// publicly and with their proper types — a phone asks for them before anybody
// has signed in.
func TestTheAppCanBeInstalled(t *testing.T) {
	i := newInstance(t).anonymous()
	for path, want := range map[string]string{
		"/manifest.webmanifest":  "application/manifest+json",
		"/icon-192.png":          "image/png",
		"/icon-512.png":          "image/png",
		"/icon-maskable-512.png": "image/png",
		"/apple-touch-icon.png":  "image/png",
	} {
		resp := i.do(http.MethodGet, path, nil)
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusOK || resp.Header.Get("Content-Type") != want {
			t.Errorf("%s: %d %q, want 200 %q", path, resp.StatusCode, resp.Header.Get("Content-Type"), want)
		}
	}
	resp := i.do(http.MethodGet, "/", nil)
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if !strings.Contains(string(body), `<link rel="manifest" href="/manifest.webmanifest" />`) {
		t.Error("the page does not name its manifest, so no browser offers to install it")
	}
}
