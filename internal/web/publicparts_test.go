package web

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

// createRepoWith creates a board with the choices the create dialog sends.
func (i *instance) createRepoWith(orgSlug, repoSlug string, choices map[string]any) {
	i.t.Helper()
	body := map[string]any{"slug": repoSlug, "name": repoSlug}
	for k, v := range choices {
		body[k] = v
	}
	resp := i.do(http.MethodPost, "/api/orgs/"+orgSlug+"/repositories", body)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		i.t.Fatalf("creating repository %s/%s returned %d", orgSlug, repoSlug, resp.StatusCode)
	}
}

// publishedIdea is an idea somebody sent and the team approved, on the roadmap's first column.
func publishedIdea(t *testing.T, team *instance, repo string) string {
	t.Helper()
	id, _ := team.anonymous().submit("codigosh", repo, "idea", "dark mode for "+repo)
	for _, step := range []struct {
		path string
		body map[string]string
	}{
		{"/moderation", map[string]string{"state": "published"}},
		{"/status", map[string]string{"status": "planned"}},
	} {
		if got := status(t, team.do(http.MethodPost, "/api/items/"+id+step.path, step.body)); got != http.StatusOK {
			t.Fatalf("%s: %d", step.path, got)
		}
	}
	return id
}

// change sends what the settings page sends: the whole board, with some choices.
func change(t *testing.T, team *instance, repo string, choices map[string]any) int {
	t.Helper()
	body := map[string]any{"name": repo, "accepts_bugs": true, "accepts_ideas": true}
	for k, v := range choices {
		body[k] = v
	}
	return status(t, team.do(http.MethodPatch, "/api/orgs/codigosh/repos/"+repo, body))
}

func repoFlags(t *testing.T, who *instance, repo string) (votes, roadmap any) {
	t.Helper()
	body := decode(t, who.do(http.MethodGet, "/api/orgs/codigosh/repos/"+repo, nil))
	return body["votes"], body["roadmap_public"]
}

// Both are on unless the team turns them off, at creation or later.
func TestVotesAndTheRoadmapAreOnUnlessChosenOff(t *testing.T) {
	team := newInstance(t)
	team.claim()
	team.createOrg("codigosh")
	team.createRepo("codigosh", "plain")
	team.createRepoWith("codigosh", "quiet", map[string]any{"votes": false, "roadmap_public": false})

	if v, r := repoFlags(t, team, "plain"); v != true || r != true {
		t.Errorf("a board made with no choice: votes %v, roadmap %v", v, r)
	}
	if v, r := repoFlags(t, team, "quiet"); v != false || r != false {
		t.Errorf("a board made with both off: votes %v, roadmap %v", v, r)
	}

	// Changing one leaves the other where it was.
	if got := change(t, team, "quiet", map[string]any{"votes": true}); got != http.StatusOK {
		t.Fatalf("turning votes on: %d", got)
	}
	if v, r := repoFlags(t, team, "quiet"); v != true || r != false {
		t.Errorf("after turning votes on: votes %v, roadmap %v", v, r)
	}
	if got := change(t, team, "plain", map[string]any{"roadmap_public": false}); got != http.StatusOK {
		t.Fatalf("hiding the roadmap: %d", got)
	}
	if v, r := repoFlags(t, team, "plain"); v != true || r != false {
		t.Errorf("after hiding the roadmap: votes %v, roadmap %v", v, r)
	}
}

// With votes off the server refuses them, whatever the page draws.
func TestABoardWithoutVotesRefusesThem(t *testing.T) {
	team := newInstance(t)
	team.claim()
	team.createOrg("codigosh")
	team.createRepoWith("codigosh", "lastboard", map[string]any{"votes": false})
	id := publishedIdea(t, team, "lastboard")

	public := team.anonymous()
	for _, method := range []string{http.MethodPost, http.MethodDelete} {
		resp := public.do(method, "/api/items/"+id+"/votes", nil)
		body := decode(t, resp)
		if resp.StatusCode != http.StatusConflict || body["error"] != "error.votes_off" {
			t.Errorf("%s a vote on a board without votes: %d %v", method, resp.StatusCode, body)
		}
	}

	// Turned back on, the same vote counts.
	if got := change(t, team, "lastboard", map[string]any{"votes": true}); got != http.StatusOK {
		t.Fatal(got)
	}
	if got := status(t, public.do(http.MethodPost, "/api/items/"+id+"/votes", nil)); got >= 300 {
		t.Errorf("voting once votes are back on: %d", got)
	}
}

// A roadmap the team keeps to itself is not found by anybody else, and the team still reads it.
func TestAPrivateRoadmapIsTheTeams(t *testing.T) {
	team := newInstance(t)
	team.claim()
	team.createOrg("codigosh")
	team.createRepoWith("codigosh", "lastboard", map[string]any{"roadmap_public": false})
	publishedIdea(t, team, "lastboard")

	public := team.anonymous()
	for _, path := range []string{"/roadmap", "/roadmap/planned"} {
		if got := status(t, public.do(http.MethodGet, "/api/orgs/codigosh/repos/lastboard"+path, nil)); got != http.StatusNotFound {
			t.Errorf("the public read %s: %d", path, got)
		}
		if got := status(t, team.do(http.MethodGet, "/api/orgs/codigosh/repos/lastboard"+path, nil)); got != http.StatusOK {
			t.Errorf("the team read %s: %d", path, got)
		}
	}
	// The board itself is still public.
	if got := status(t, public.do(http.MethodGet, "/api/orgs/codigosh/repos/lastboard", nil)); got != http.StatusOK {
		t.Errorf("the board with a private roadmap: %d", got)
	}
}

// Nor does it reach the front page or the sitemap.
func TestAPrivateRoadmapStaysOffTheFrontPageAndTheSitemap(t *testing.T) {
	team, srv := newPushInstance(t)
	team.claim()
	team.createOrg("codigosh")
	team.createRepoWith("codigosh", "hidden", map[string]any{"roadmap_public": false})
	team.createRepo("codigosh", "shown")
	publishedIdea(t, team, "hidden")
	publishedIdea(t, team, "shown")

	boards, err := srv.orgs().ListBoards()
	if err != nil {
		t.Fatal(err)
	}
	planned, _ := srv.comingIn(boards)["planned"].([]map[string]any)
	if len(planned) != 1 || planned[0]["board"] != "shown" {
		t.Errorf("the front page's roadmap: %v", planned)
	}

	srv.cfg.PublicURL = "https://help.example.com"
	resp := team.anonymous().do(http.MethodGet, "/sitemap.xml", nil)
	raw, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	sitemap := string(raw)
	if !strings.Contains(sitemap, "/codigosh/shown/roadmap") || strings.Contains(sitemap, "/codigosh/hidden/roadmap") {
		t.Errorf("the sitemap's roadmaps:\n%s", sitemap)
	}
	if !strings.Contains(sitemap, "/codigosh/hidden<") {
		t.Errorf("the board with a private roadmap left the sitemap altogether:\n%s", sitemap)
	}
}

// A link on the front page to a roadmap nobody outside the team can read is not sent to anybody
// outside it: the visitor would be taken to "not found".
func TestAPrivateRoadmapIsNotLinkedFromTheFrontPageToStrangers(t *testing.T) {
	team := newInstance(t)
	team.claim()
	team.createOrg("codigosh")
	team.createRepoWith("codigosh", "hidden", map[string]any{"roadmap_public": false})
	team.createRepo("codigosh", "shown")

	link := func(to, board string) map[string]any {
		return map[string]any{"label": to + " " + board, "to": to, "scope": "codigosh/" + board}
	}
	page := map[string]any{
		"bands": []map[string]any{{
			"id": "b", "columns": "1",
			"cells": [][]map[string]any{{{"id": "l", "kind": "links", "links": []map[string]any{
				link("roadmap", "hidden"), link("roadmap", "shown"), link("board", "hidden"),
			}}}},
		}},
		"settings": map[string]any{"footer": []map[string]any{link("roadmap", "hidden"), link("roadmap", "shown")}},
	}
	if got := status(t, team.do(http.MethodPut, "/api/landing", page)); got != http.StatusOK {
		t.Fatalf("the administrator could not save the page: %d", got)
	}

	labels := func(body map[string]any) (module, footer []string) {
		bands, _ := body["bands"].([]any)
		for _, b := range bands {
			cells, _ := b.(map[string]any)["cells"].([]any)
			for _, cell := range cells {
				for _, m := range cell.([]any) {
					links, _ := m.(map[string]any)["links"].([]any)
					for _, l := range links {
						module = append(module, l.(map[string]any)["label"].(string))
					}
				}
			}
		}
		settings, _ := body["settings"].(map[string]any)
		foot, _ := settings["footer"].([]any)
		for _, l := range foot {
			footer = append(footer, l.(map[string]any)["label"].(string))
		}
		return module, footer
	}

	module, footer := labels(frontPage(t, team.anonymous(), ""))
	if got := strings.Join(module, ","); got != "roadmap shown,board hidden" {
		t.Errorf("a stranger's links: %q", got)
	}
	if got := strings.Join(footer, ","); got != "roadmap shown" {
		t.Errorf("a stranger's footer: %q", got)
	}

	// The team, who can read that roadmap, keeps the link; and the page itself was never changed.
	module, footer = labels(frontPage(t, team, ""))
	if len(module) != 3 || len(footer) != 2 {
		t.Errorf("the team's links: %v and footer %v", module, footer)
	}
}

// A board made while the server has a channel starts on it, unless the team chose none.
func TestANewBoardCanStartWithNoChannel(t *testing.T) {
	team := newInstance(t)
	team.claim()
	if got := status(t, team.do(http.MethodPut, "/api/notifications", map[string]any{"kind": "webhook", "url": "https://hooks.example.com/x"})); got != http.StatusOK {
		t.Fatalf("setting the server's channel: %d", got)
	}
	team.createOrg("codigosh")
	team.createRepo("codigosh", "default")
	team.createRepoWith("codigosh", "silent", map[string]any{"channel": "none"})

	config := func(slug string) string {
		var c string
		if err := team.db().QueryRow(`SELECT notify_config FROM repository WHERE slug = ?`, slug).Scan(&c); err != nil {
			t.Fatal(err)
		}
		return c
	}
	if !strings.Contains(config("default"), "server") {
		t.Errorf("a board made with no choice did not take the server's channel: %q", config("default"))
	}
	if strings.Contains(config("silent"), "server") {
		t.Errorf("a board made with no channel took the server's: %q", config("silent"))
	}
}
