package web

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

// status asks for a path and returns only the code.
func (i *instance) status(method, path string) int {
	i.t.Helper()
	resp := i.do(method, path, nil)
	_ = resp.Body.Close()
	return resp.StatusCode
}

func (i *instance) publish(id string) {
	i.t.Helper()
	resp := i.do(http.MethodPost, "/api/items/"+id+"/moderation", map[string]string{"state": "published"})
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		i.t.Fatalf("publishing %s returned %d", id, resp.StatusCode)
	}
}

func listed(t *testing.T, c *instance, path, key, slug string) bool {
	t.Helper()
	body := decode(t, c.do(http.MethodGet, path, nil))
	list, _ := body[key].([]any)
	for _, e := range list {
		if m, _ := e.(map[string]any); m["slug"] == slug {
			return true
		}
	}
	return false
}

// A private board is a 404 to the public — the same one a missing board gives —
// in every door the public has, and stays whole for the team.
func TestAPrivateBoardIsAnotherNotFoundForThePublic(t *testing.T) {
	team := newInstance(t)
	team.claim()
	team.createOrg("codigosh")
	team.createRepo("codigosh", "lastboard")
	team.createRepo("codigosh", "secret")

	public := team.anonymous()
	id, _ := public.submit("codigosh", "secret", "idea", "an idea for the secret board")
	team.publish(id)

	base := "/api/orgs/codigosh/repos/secret"
	doors := []string{base, base + "/items", base + "/form", base + "/roadmap", base + "/similar?q=idea",
		"/api/items/" + id, "/feed/codigosh/secret/atom.xml"}
	for _, d := range doors {
		if got := public.status(http.MethodGet, d); got != http.StatusOK {
			t.Fatalf("before hiding, %s returned %d", d, got)
		}
	}

	resp := team.do(http.MethodPatch, base, map[string]any{"name": "secret", "accepts_bugs": true, "accepts_ideas": true, "private": true})
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("hiding the board returned %d", resp.StatusCode)
	}

	missing := public.status(http.MethodGet, "/api/orgs/codigosh/repos/nothing")
	for _, d := range doors {
		if got := public.status(http.MethodGet, d); got != missing {
			t.Errorf("%s is %d to the public, a missing board gives %d", d, got, missing)
		}
		if got := team.status(http.MethodGet, d); got != http.StatusOK {
			t.Errorf("%s is %d to the team", d, got)
		}
	}
	for _, d := range []string{"/api/items/" + id + "/votes", "/api/items/" + id + "/comments"} {
		if got := public.status(http.MethodPost, d); got != http.StatusNotFound {
			t.Errorf("POST %s returned %d to the public", d, got)
		}
	}

	if listed(t, public, "/api/orgs/codigosh/repos", "repositories", "secret") {
		t.Error("the public list names the private board")
	}
	if !listed(t, public, "/api/orgs/codigosh/repos", "repositories", "lastboard") {
		t.Error("the public list lost the board that is public")
	}
	if !listed(t, team, "/api/orgs/codigosh/repos", "repositories", "secret") {
		t.Error("the team list does not name the private board")
	}
	landing := public.do(http.MethodGet, "/api/landing", nil)
	raw, _ := io.ReadAll(landing.Body)
	_ = landing.Body.Close()
	if strings.Contains(string(raw), "secret") {
		t.Error("the front page names a private board")
	}
	if !strings.Contains(string(raw), "lastboard") {
		t.Error("the front page no longer shows the public board; this check is not reading the boards")
	}
	if hits, _ := decode(t, public.do(http.MethodGet, "/api/search?q=secret", nil))["items"].([]any); len(hits) != 0 {
		t.Errorf("search finds %d reports of a private board", len(hits))
	}

	// Public again.
	resp = team.do(http.MethodPatch, base, map[string]any{"name": "secret", "accepts_bugs": true, "accepts_ideas": true, "private": false})
	_ = resp.Body.Close()
	if got := public.status(http.MethodGet, base); got != http.StatusOK {
		t.Errorf("a board made public again returned %d", got)
	}
}

// A private organisation takes every board under it with it.
func TestAPrivateOrganizationHidesItsBoards(t *testing.T) {
	team := newInstance(t)
	team.claim()
	team.createOrg("codigosh")
	team.createRepo("codigosh", "lastboard")
	team.createOrg("other")

	public := team.anonymous()
	resp := team.do(http.MethodPatch, "/api/orgs/codigosh", map[string]any{"name": "codigosh", "private": true})
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("hiding the organisation returned %d", resp.StatusCode)
	}

	for _, d := range []string{"/api/orgs/codigosh/repos", "/api/orgs/codigosh/repos/lastboard", "/api/orgs/codigosh/repos/lastboard/items"} {
		if got := public.status(http.MethodGet, d); got != http.StatusNotFound {
			t.Errorf("%s returned %d to the public", d, got)
		}
		if got := team.status(http.MethodGet, d); got != http.StatusOK {
			t.Errorf("%s returned %d to the team", d, got)
		}
	}
	if listed(t, public, "/api/orgs", "organizations", "codigosh") {
		t.Error("the front page names a private organisation")
	}
	if !listed(t, public, "/api/orgs", "organizations", "other") {
		t.Error("the public organisation vanished with it")
	}
	if !listed(t, team, "/api/orgs", "organizations", "codigosh") {
		t.Error("the team no longer sees its own organisation")
	}
}

// Working on one board does not open its private neighbour.
func TestACollaboratorSeesOnlyTheirOwnPrivateBoard(t *testing.T) {
	team := newInstance(t)
	team.claim()
	team.createOrg("codigosh")
	team.createRepo("codigosh", "lastboard")
	team.createRepo("codigosh", "lastdav")
	for _, r := range []string{"lastboard", "lastdav"} {
		resp := team.do(http.MethodPatch, "/api/orgs/codigosh/repos/"+r, map[string]any{"name": r, "accepts_bugs": true, "accepts_ideas": true, "private": true})
		_ = resp.Body.Close()
	}
	helper := team.anonymous()
	makeCollaborator(t, team, helper, "helper")

	if got := helper.status(http.MethodGet, "/api/orgs/codigosh/repos/lastboard"); got != http.StatusOK {
		t.Errorf("a collaborator cannot see their own private board: %d", got)
	}
	if got := helper.status(http.MethodGet, "/api/orgs/codigosh/repos/lastdav"); got != http.StatusNotFound {
		t.Errorf("a collaborator sees the private board next door: %d", got)
	}
}
