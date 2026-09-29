package web

import (
	"net/http"
	"strconv"
	"testing"

	"git.codigosh.com/CodigoSH/LastHelp/internal/core/account"
	"git.codigosh.com/CodigoSH/LastHelp/internal/core/authz"
)

// TestOnlyWhoRunsTheOrganisationDeletesAReport.
//
// Deleting cannot be undone, so it belongs to the organisation's owners and
// the administrator. A collaborator triages — rejects, marks as spam, merges —
// and is told they may not delete, both by the page and by the server.
func TestOnlyWhoRunsTheOrganisationDeletesAReport(t *testing.T) {
	team := newInstance(t)
	team.claim()
	team.createOrg("codigosh")
	team.createRepo("codigosh", "lastboard")

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
	collaborator := team.anonymous()
	_ = status(t, collaborator.do(http.MethodPost, "/api/session", map[string]string{
		"username": "helper", "password": "a good long passphrase",
	}))

	public := team.anonymous()
	report, _ := public.submit("codigosh", "lastboard", "idea", "dark mode")
	duplicate, _ := public.submit("codigosh", "lastboard", "idea", "a dark theme please")
	_ = status(t, team.do(http.MethodPost, "/api/items/"+report+"/moderation", map[string]string{"state": "published"}))
	_ = status(t, team.do(http.MethodPost, "/api/items/"+report+"/reply", map[string]string{"body": "on it"}))
	_ = status(t, team.do(http.MethodPost, "/api/items/"+duplicate+"/merge", map[string]string{"into": report}))

	// The page is told who may, by the server.
	if got := teamPart(t, team, report)["can_delete"]; got != true {
		t.Errorf("the administrator was not offered deleting: %v", got)
	}
	if got := teamPart(t, collaborator, report)["can_delete"]; got != false {
		t.Errorf("a collaborator was offered deleting: %v", got)
	}

	// And the server holds the line whatever the page shows.
	if got := status(t, collaborator.do(http.MethodDelete, "/api/items/"+report, nil)); got != http.StatusForbidden {
		t.Errorf("a collaborator deleted a report: %d", got)
	}
	if got := status(t, team.anonymous().do(http.MethodDelete, "/api/items/"+report, nil)); got != http.StatusUnauthorized && got != http.StatusForbidden {
		t.Errorf("a stranger reached delete: %d", got)
	}

	if got := status(t, team.do(http.MethodDelete, "/api/items/"+report, nil)); got != http.StatusOK {
		t.Fatalf("the administrator could not delete: %d", got)
	}
	if got := status(t, team.anonymous().do(http.MethodGet, "/api/items/"+report, nil)); got != http.StatusNotFound {
		t.Errorf("the deleted report is still served: %d", got)
	}
	var comments int
	_ = team.db().QueryRow(`SELECT count(*) FROM comment`).Scan(&comments)
	if comments != 0 {
		t.Errorf("%d comments outlived their report", comments)
	}
	// A duplicate merged into it is a report of its own again, not gone with it.
	var mergedInto any
	if err := team.db().QueryRow(`SELECT merged_into FROM item WHERE public_id = ?`, duplicate).Scan(&mergedInto); err != nil {
		t.Fatalf("the duplicate went with the report it was merged into: %v", err)
	}
	if mergedInto != nil {
		t.Errorf("the duplicate still points at a deleted report: %v", mergedInto)
	}
}

// TestTheBoardsTeamLooksAfterItsConversation.
//
// Comments and team replies are edited and deleted by whoever works on the
// board: a collaborator of that board may, one of another board may not, and
// a visitor may not. The report, the boards and the organisations are not a
// collaborator's; those are tested where they are handled.
func TestTheBoardsTeamLooksAfterItsConversation(t *testing.T) {
	team := newInstance(t)
	team.claim()
	team.createOrg("codigosh")
	team.createRepo("codigosh", "lastboard")
	team.createRepo("codigosh", "other")

	accounts := account.Store{DB: team.db()}
	signedIn := func(name, board string) *instance {
		person, err := accounts.Create(name, "a good long passphrase", false, team.now())
		if err != nil {
			t.Fatal(err)
		}
		var boardID int64
		if err := team.db().QueryRow(`SELECT id FROM repository WHERE slug = ?`, board).Scan(&boardID); err != nil {
			t.Fatal(err)
		}
		if err := accounts.Grant(person.ID, authz.Repository(boardID, 0), authz.RoleCollaborator, team.now()); err != nil {
			t.Fatal(err)
		}
		c := team.anonymous()
		_ = status(t, c.do(http.MethodPost, "/api/session", map[string]string{
			"username": name, "password": "a good long passphrase",
		}))
		return c
	}
	collaborator := signedIn("helper", "lastboard")
	elsewhere := signedIn("stranger", "other")
	visitor := team.anonymous()

	report, _ := team.anonymous().submit("codigosh", "lastboard", "idea", "dark mode")
	_ = status(t, team.do(http.MethodPost, "/api/items/"+report+"/moderation", map[string]string{"state": "published"}))
	_ = status(t, team.do(http.MethodPost, "/api/items/"+report+"/reply", map[string]string{"body": "on it"}))

	comments := func() []any {
		c, _ := decode(t, team.anonymous().do(http.MethodGet, "/api/items/"+report, nil))["comments"].([]any)
		return c
	}
	thread := func() []string {
		out := []string{}
		for _, x := range comments() {
			body, _ := x.(map[string]any)["body"].(string)
			out = append(out, body)
		}
		return out
	}
	before := comments()
	if len(before) != 1 {
		t.Fatalf("the reply is not in the thread: %v", before)
	}
	id, _ := before[0].(map[string]any)["id"].(float64)
	if id == 0 {
		t.Fatalf("the thread does not say which comment is which: %v", before[0])
	}
	path := "/api/comments/" + strconv.FormatInt(int64(id), 10)
	edit := map[string]string{"body": "not mine to change"}

	for who, c := range map[string]*instance{"a collaborator of another board": elsewhere, "a visitor": visitor} {
		if got := status(t, c.do(http.MethodPatch, path, edit)); got == http.StatusOK {
			t.Errorf("%s edited a comment", who)
		}
		if got := status(t, c.do(http.MethodDelete, path, nil)); got == http.StatusOK {
			t.Errorf("%s deleted a comment", who)
		}
	}
	if got := thread(); len(got) != 1 || got[0] != "on it" {
		t.Fatalf("a refused edit or delete changed the thread: %v", got)
	}

	if got := status(t, collaborator.do(http.MethodPatch, path, map[string]string{"body": "  on it, next week  "})); got != http.StatusOK {
		t.Fatalf("the board's collaborator could not edit the reply: %d", got)
	}
	if got := thread(); len(got) != 1 || got[0] != "on it, next week" {
		t.Errorf("the thread says %v after the edit", got)
	}
	if got := status(t, collaborator.do(http.MethodPatch, path, map[string]string{"body": "   "})); got != http.StatusBadRequest {
		t.Errorf("an empty rewrite was accepted: %d", got)
	}
	if got := status(t, team.do(http.MethodPatch, "/api/comments/999999", edit)); got != http.StatusNotFound {
		t.Errorf("editing a comment that does not exist: %d", got)
	}

	if got := status(t, collaborator.do(http.MethodDelete, path, nil)); got != http.StatusOK {
		t.Fatalf("the board's collaborator could not delete the reply: %d", got)
	}
	if got := thread(); len(got) != 0 {
		t.Errorf("the deleted comment is still in the thread: %v", got)
	}
}

// TestTheReportCarriesWhatTheTeamMayDoToIt.
//
// The reports screen reads one report per row chosen. What the reader may do
// to it — the statuses, whether it can be deleted — rides in the same answer,
// for whoever may triage it and nobody else.
func TestTheReportCarriesWhatTheTeamMayDoToIt(t *testing.T) {
	team := newInstance(t)
	team.claim()
	team.createOrg("codigosh")
	team.createRepo("codigosh", "lastboard")

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
	collaborator := team.anonymous()
	_ = status(t, collaborator.do(http.MethodPost, "/api/session", map[string]string{
		"username": "helper", "password": "a good long passphrase",
	}))

	public := team.anonymous()
	report, _ := public.submit("codigosh", "lastboard", "bug", "crash on import")
	_ = status(t, team.do(http.MethodPost, "/api/items/"+report+"/moderation", map[string]string{"state": "published"}))

	for _, c := range []struct {
		who       string
		as        *instance
		team      bool
		canDelete bool
	}{
		{"a visitor", team.anonymous(), false, false},
		{"a collaborator", collaborator, true, false},
		{"the administrator", team, true, true},
	} {
		got := decode(t, c.as.do(http.MethodGet, "/api/items/"+report, nil))
		work, has := got["team"].(map[string]any)
		if has != c.team {
			t.Errorf("%s: the report carried the team's part: %v, want %v", c.who, has, c.team)
			continue
		}
		if !has {
			continue
		}
		if work["can_delete"] != c.canDelete {
			t.Errorf("%s: can_delete %v, want %v", c.who, work["can_delete"], c.canDelete)
		}
		if work["current"] != "new" || len(work["statuses"].([]any)) == 0 {
			t.Errorf("%s: the statuses it can move to are missing: %v", c.who, work)
		}
	}
}

// teamPart is what the report says its reader may do to it, or nothing.
func teamPart(t *testing.T, as *instance, report string) map[string]any {
	t.Helper()
	work, _ := decode(t, as.do(http.MethodGet, "/api/items/"+report, nil))["team"].(map[string]any)
	return work
}
