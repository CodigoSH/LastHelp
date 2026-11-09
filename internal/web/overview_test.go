package web

import (
	"net/http"
	"sort"
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

// TestWhatNobodyAnsweredIsOneThingEverywhere.
//
// The overview says how many reports nobody answered and links to the list of
// them. The two are read from the same conditions, and this holds them to it:
// a number that says seven over a list that holds six is the screen
// contradicting itself, which nothing else would notice.
func TestWhatNobodyAnsweredIsOneThingEverywhere(t *testing.T) {
	admin, _, collaborator := teamSetup(t)
	public := admin.anonymous()

	publish := func(board, kind, title, state string) string {
		org, repo, _ := splitBoard(board)
		id, _ := public.submit(org, repo, kind, title)
		_ = status(t, admin.do(http.MethodPost, "/api/items/"+id+"/moderation", map[string]string{"state": "published"}))
		if state != "" {
			_ = status(t, admin.do(http.MethodPost, "/api/items/"+id+"/status", map[string]string{"status": state}))
		}
		return id
	}
	publish("acme/atlas", "idea", "nobody wrote back", "")
	answered := publish("acme/atlas", "idea", "somebody did", "")
	publish("acme/atlas", "idea", "finished without a word", "done")
	publish("other/elsewhere", "bug", "next door, also silent", "")
	_ = status(t, admin.do(http.MethodPost, "/api/items/"+answered+"/reply", map[string]string{"body": "on it"}))
	// A visitor's comment is not an answer.
	_ = status(t, public.do(http.MethodPost, "/api/items/"+answered+"/comments", map[string]string{"body": "me too"}))

	titles := func(who *instance, query string) []string {
		body := decode(t, who.do(http.MethodGet, "/api/reports?"+query, nil))
		var out []string
		for _, raw := range body["items"].([]any) {
			out = append(out, raw.(map[string]any)["title"].(string))
		}
		sort.Strings(out)
		return out
	}

	for who, want := range map[*instance][]string{
		admin:        {"next door, also silent", "nobody wrote back"},
		collaborator: {"nobody wrote back"},
	} {
		overview := decode(t, who.do(http.MethodGet, "/api/overview", nil))
		listed := titles(who, "reply=none")
		if len(listed) != len(want) || (len(want) > 0 && listed[0] != want[0]) {
			t.Errorf("the unanswered list was %v, want %v", listed, want)
		}
		if overview["unanswered"] != float64(len(listed)) {
			t.Errorf("the overview says %v unanswered over a list of %d: %v", overview["unanswered"], len(listed), listed)
		}
	}

	if got := status(t, admin.do(http.MethodGet, "/api/reports?reply=maybe", nil)); got != http.StatusBadRequest {
		t.Errorf("an unknown reply filter answered %d, want a refusal", got)
	}
}

// TestWhatWaitsIsTheNumberTheBadgeShows.
func TestWhatWaitsIsTheNumberTheBadgeShows(t *testing.T) {
	admin, _, collaborator := teamSetup(t)
	admin.submit("acme", "atlas", "bug", "waiting one")
	admin.submit("acme", "atlas", "idea", "waiting two")
	admin.submit("other", "elsewhere", "bug", "next door")

	for who, want := range map[*instance]float64{admin: 3, collaborator: 2} {
		overview := decode(t, who.do(http.MethodGet, "/api/overview", nil))
		me := decode(t, who.do(http.MethodGet, "/api/me", nil))
		badge, _ := me["pending"].(map[string]any)
		if overview["waiting_reports"] != badge["reports"] || overview["waiting_messages"] != badge["messages"] {
			t.Errorf("the overview waits %v+%v, the badge %v+%v", overview["waiting_reports"], overview["waiting_messages"], badge["reports"], badge["messages"])
		}
		if overview["waiting_reports"] != want {
			t.Errorf("waiting_reports = %v, want %v", overview["waiting_reports"], want)
		}
		if oldest, _ := overview["waiting_oldest"].(float64); oldest == 0 {
			t.Errorf("something waits and the oldest wait is unknown")
		}
	}

	// Only the administrator is told what the server gave up on.
	if got := decode(t, collaborator.do(http.MethodGet, "/api/overview", nil))["failed_jobs"]; got != float64(0) {
		t.Errorf("a collaborator was told about the server's failed jobs: %v", got)
	}
}
