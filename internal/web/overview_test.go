package web

import (
	"net/http"
	"testing"
)

// TestTheOverviewSplitsStatusesByKind.
//
// A bug and an idea do not share a vocabulary: "fixed" is not what an idea
// becomes and "planned" is not what a bug does. One list of both counted them
// together and said nothing about either.
func TestTheOverviewSplitsStatusesByKind(t *testing.T) {
	team := newInstance(t)
	team.claim()
	team.createOrg("acme")
	team.createRepo("acme", "atlas")

	publish := func(kind, title, state string) {
		id, _ := team.anonymous().submit("acme", "atlas", kind, title)
		_ = status(t, team.do(http.MethodPost, "/api/items/"+id+"/moderation", map[string]string{"state": "published"}))
		if state != "" {
			if got := status(t, team.do(http.MethodPost, "/api/items/"+id+"/status", map[string]string{"status": state})); got != http.StatusOK {
				t.Fatalf("setting %s on %s: %d", state, title, got)
			}
		}
	}
	publish("idea", "dark mode", "planned")
	publish("idea", "csv export", "planned")
	publish("bug", "it crashes", "closed")

	overview := decode(t, team.do(http.MethodGet, "/api/overview", nil))
	split, _ := overview["statuses_by_kind"].(map[string]any)
	ideas, _ := split["idea"].(map[string]any)
	bugs, _ := split["bug"].(map[string]any)

	if ideas["planned"] != float64(2) || ideas["closed"] != nil {
		t.Errorf("the ideas were counted as %v, want two planned and no fixed", ideas)
	}
	if bugs["closed"] != float64(1) || bugs["planned"] != nil {
		t.Errorf("the bugs were counted as %v, want one fixed and no planned", bugs)
	}

	// The whole is still there, for a browser that has not reloaded.
	whole, _ := overview["statuses"].(map[string]any)
	if whole["planned"] != float64(2) || whole["closed"] != float64(1) {
		t.Errorf("the combined count changed shape: %v", whole)
	}
}

// TestAnEmptyOverviewStillSplitsByKind: nothing reported yet must not turn the
// split into a null a browser then has to guess about.
func TestAnEmptyOverviewStillSplitsByKind(t *testing.T) {
	team := newInstance(t)
	team.claim()
	overview := decode(t, team.do(http.MethodGet, "/api/overview", nil))
	split, ok := overview["statuses_by_kind"].(map[string]any)
	if !ok || split["idea"] == nil || split["bug"] == nil {
		t.Errorf("an empty server answered %v, want an empty object for each kind", overview["statuses_by_kind"])
	}
}

// TestTheChartGoesBackAsFarAsAskedAndNoFurther.
func TestTheChartGoesBackAsFarAsAskedAndNoFurther(t *testing.T) {
	team := newInstance(t)
	team.claim()

	for query, want := range map[string]int{"": 30, "?days=7": 7, "?days=30": 30, "?days=90": 90, "?days=10000": 30, "?days=-1": 30, "?days=abc": 30} {
		overview := decode(t, team.do(http.MethodGet, "/api/overview"+query, nil))
		series, _ := overview["series"].([]any)
		if len(series) != want {
			t.Errorf("/api/overview%s drew %d days, want %d", query, len(series), want)
		}
	}
}
