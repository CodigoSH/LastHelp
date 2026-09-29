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
	if got := decode(t, team.do(http.MethodGet, "/api/items/"+report+"/statuses", nil))["can_delete"]; got != true {
		t.Errorf("the administrator was not offered deleting: %v", got)
	}
	if got := decode(t, collaborator.do(http.MethodGet, "/api/items/"+report+"/statuses", nil))["can_delete"]; got != false {
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

// TestOnlyWhoRunsTheOrganisationDeletesAComment.
//
// Comments and team replies follow the report: deleting one is the owners'
// and the administrator's, and it leaves the thread for good.
func TestOnlyWhoRunsTheOrganisationDeletesAComment(t *testing.T) {
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

	report, _ := team.anonymous().submit("codigosh", "lastboard", "idea", "dark mode")
	_ = status(t, team.do(http.MethodPost, "/api/items/"+report+"/moderation", map[string]string{"state": "published"}))
	_ = status(t, team.do(http.MethodPost, "/api/items/"+report+"/reply", map[string]string{"body": "on it"}))

	thread := func() []any {
		c, _ := decode(t, team.anonymous().do(http.MethodGet, "/api/items/"+report, nil))["comments"].([]any)
		return c
	}
	before := thread()
	if len(before) != 1 {
		t.Fatalf("the reply is not in the thread: %v", before)
	}
	id, _ := before[0].(map[string]any)["id"].(float64)
	if id == 0 {
		t.Fatalf("the thread does not say which comment is which: %v", before[0])
	}
	path := "/api/comments/" + strconv.FormatInt(int64(id), 10)

	if got := status(t, collaborator.do(http.MethodDelete, path, nil)); got != http.StatusForbidden {
		t.Errorf("a collaborator deleted a comment: %d", got)
	}
	if got := status(t, team.do(http.MethodDelete, path, nil)); got != http.StatusOK {
		t.Fatalf("the administrator could not delete the comment: %d", got)
	}
	if after := thread(); len(after) != 0 {
		t.Errorf("the deleted comment is still in the thread: %v", after)
	}
}
