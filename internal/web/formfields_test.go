package web

import (
	"net/http"
	"testing"
)

// asks turns the problem form's two questions on or off for a project, the way
// its settings page does.
func (i *instance) asks(orgSlug, repoSlug string, version, steps bool) {
	i.t.Helper()
	edit := map[string]any{"name": repoSlug, "forge_repo": "", "accepts_bugs": true, "accepts_ideas": true,
		"asks_version": version, "asks_steps": steps}
	if got := status(i.t, i.do(http.MethodPatch, "/api/orgs/"+orgSlug+"/repos/"+repoSlug, edit)); got != http.StatusOK {
		i.t.Fatalf("setting the form of %s/%s answered %d", orgSlug, repoSlug, got)
	}
}

// keptAnswers sends a problem with both answers to a project, publishes it, and
// returns the keys of the answers it kept.
func keptAnswers(t *testing.T, team *instance, title string) []string {
	t.Helper()
	public := team.anonymous()
	id, _ := public.submitWith("codigosh", "atlas", "bug", title, map[string]string{
		"version": "1.4.2", "steps": "1. open it",
	})
	_ = status(t, team.do(http.MethodPost, "/api/items/"+id+"/moderation", map[string]string{"state": "published"}))
	fields, _ := decode(t, public.do(http.MethodGet, "/api/items/"+id, nil))["fields"].([]any)
	keys := []string{}
	for _, raw := range fields {
		f, _ := raw.(map[string]any)
		key, _ := f["key"].(string)
		keys = append(keys, key)
	}
	return keys
}

// A version and steps are questions for software. A new project is created
// without them, says so to the form, and keeps no answer to either even when one
// arrives: a box nobody was shown is one nobody should be able to fill in.
func TestANewProjectAsksNeitherTheVersionNorTheSteps(t *testing.T) {
	team := newInstance(t)
	team.claim()
	team.createOrg("codigosh")
	team.createRepo("codigosh", "atlas")

	board := decode(t, team.anonymous().do(http.MethodGet, "/api/orgs/codigosh/repos/atlas", nil))
	if board["asks_version"] != false || board["asks_steps"] != false {
		t.Errorf("a new project tells the form version=%v steps=%v, want neither", board["asks_version"], board["asks_steps"])
	}
	if kept := keptAnswers(t, team, "the light is out"); len(kept) != 0 {
		t.Errorf("a project that asks nothing kept %v", kept)
	}
}

// Each question is a switch of its own: turning one on asks that one, and an
// edit that does not mention them — a page from before they existed — leaves
// both as they were.
func TestEachQuestionIsTurnedOnByItself(t *testing.T) {
	team := newInstance(t)
	team.claim()
	team.createOrg("codigosh")
	team.createRepo("codigosh", "atlas")

	team.asks("codigosh", "atlas", true, false)
	panel := decode(t, team.do(http.MethodGet, "/api/orgs/codigosh/repos/atlas/panel", nil))
	if panel["asks_version"] != true || panel["asks_steps"] != false {
		t.Errorf("after turning the version on the panel reads version=%v steps=%v", panel["asks_version"], panel["asks_steps"])
	}
	if kept := keptAnswers(t, team, "crash on import"); len(kept) != 1 || kept[0] != "version" {
		t.Errorf("a project that asks only the version kept %v", kept)
	}

	edit := map[string]any{"name": "atlas", "forge_repo": "", "accepts_bugs": true, "accepts_ideas": true}
	if got := status(t, team.do(http.MethodPatch, "/api/orgs/codigosh/repos/atlas", edit)); got != http.StatusOK {
		t.Fatalf("an edit without the questions answered %d", got)
	}
	board := decode(t, team.anonymous().do(http.MethodGet, "/api/orgs/codigosh/repos/atlas", nil))
	if board["asks_version"] != true || board["asks_steps"] != false {
		t.Errorf("an edit that did not mention the questions changed them: version=%v steps=%v",
			board["asks_version"], board["asks_steps"])
	}
}
