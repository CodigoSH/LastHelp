package web

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"git.codigosh.com/CodigoSH/LastHelp/internal/core/account"
	"git.codigosh.com/CodigoSH/LastHelp/internal/core/authz"
)

// teamSetup is an administrator, two organisations with a repository each, an
// owner of the first and a collaborator on its repository.
func teamSetup(t *testing.T) (admin, owner, collaborator *instance) {
	t.Helper()
	admin = newInstance(t)
	admin.claim()
	admin.createOrg("acme")
	admin.createRepo("acme", "atlas")
	admin.createOrg("other")
	admin.createRepo("other", "elsewhere")

	accounts := account.Store{DB: admin.db()}
	var acmeID, atlasID int64
	if err := admin.db().QueryRow(`SELECT id FROM organization WHERE slug = 'acme'`).Scan(&acmeID); err != nil {
		t.Fatal(err)
	}
	if err := admin.db().QueryRow(`SELECT id FROM repository WHERE slug = 'atlas'`).Scan(&atlasID); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"olga", "ana", "newcomer"} {
		if _, err := accounts.Create(name, "a good long passphrase", false, admin.now()); err != nil {
			t.Fatal(err)
		}
	}
	users, _ := accounts.List()
	id := func(name string) int64 {
		for _, u := range users {
			if u.Username == name {
				return u.ID
			}
		}
		t.Fatalf("no account %s", name)
		return 0
	}
	if err := accounts.Grant(id("olga"), authz.Organization(acmeID), authz.RoleOwner, admin.now()); err != nil {
		t.Fatal(err)
	}
	if err := accounts.Grant(id("ana"), authz.Repository(atlasID, acmeID), authz.RoleCollaborator, admin.now()); err != nil {
		t.Fatal(err)
	}

	signIn := func(name string) *instance {
		client := admin.anonymous()
		resp := client.do(http.MethodPost, "/api/session", map[string]string{"username": name, "password": "a good long passphrase"})
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("%s could not sign in: %d", name, resp.StatusCode)
		}
		return client
	}
	return admin, signIn("olga"), signIn("ana")
}

func status(t *testing.T, resp *http.Response) int {
	t.Helper()
	_ = resp.Body.Close()
	return resp.StatusCode
}

// TestAnOwnerRunsTheTeamOfTheirOwnOrganisation.
//
// The person who knows who should triage a board is the person who owns it,
// so an owner may grant and take roles — inside their organisation, at each
// role's own level, and nowhere else.
func TestAnOwnerRunsTheTeamOfTheirOwnOrganisation(t *testing.T) {
	_, owner, _ := teamSetup(t)

	body := decode(t, owner.do(http.MethodGet, "/api/orgs/acme/team", nil))
	members, _ := body["members"].([]any)
	// Three: whoever created the organisation owns it, and olga and ana.
	if len(members) != 3 {
		t.Fatalf("the team of acme should be its two owners and its collaborator: %v", body["members"])
	}

	if got := status(t, owner.do(http.MethodPost, "/api/orgs/acme/team",
		map[string]string{"username": "newcomer", "repository": "atlas", "role": "collaborator"})); got != http.StatusOK {
		t.Errorf("the owner could not add a collaborator to their own repository: %d", got)
	}
	body = decode(t, owner.do(http.MethodGet, "/api/orgs/acme/team", nil))
	if members, _ := body["members"].([]any); len(members) != 4 {
		t.Errorf("the collaborator added does not appear in the team: %v", body["members"])
	}

	// Each role at its own level.
	if got := status(t, owner.do(http.MethodPost, "/api/orgs/acme/team",
		map[string]string{"username": "newcomer", "repository": "atlas", "role": "owner"})); got != http.StatusBadRequest {
		t.Errorf("an owner was made on a repository: %d", got)
	}
	if got := status(t, owner.do(http.MethodPost, "/api/orgs/acme/team",
		map[string]string{"username": "newcomer", "role": "collaborator"})); got != http.StatusBadRequest {
		t.Errorf("a collaborator was made on a whole organisation: %d", got)
	}

	// A repository of another organisation is not found under this one.
	if got := status(t, owner.do(http.MethodPost, "/api/orgs/acme/team",
		map[string]string{"username": "newcomer", "repository": "elsewhere", "role": "collaborator"})); got != http.StatusNotFound {
		t.Errorf("a repository next door was granted through acme: %d", got)
	}
	// And the other organisation's team is not theirs.
	if got := status(t, owner.do(http.MethodGet, "/api/orgs/other/team", nil)); got != http.StatusForbidden {
		t.Errorf("the owner read the team of an organisation they do not own: %d", got)
	}
	if got := status(t, owner.do(http.MethodPost, "/api/orgs/other/team",
		map[string]string{"username": "newcomer", "repository": "elsewhere", "role": "collaborator"})); got != http.StatusForbidden {
		t.Errorf("the owner changed the team of an organisation they do not own: %d", got)
	}

	// Taking a role back.
	if got := status(t, owner.do(http.MethodPost, "/api/orgs/acme/team",
		map[string]string{"username": "newcomer", "repository": "atlas", "role": ""})); got != http.StatusOK {
		t.Errorf("the owner could not take a role back: %d", got)
	}
}

// TestACollaboratorNeitherSeesNorChangesTheTeam.
func TestACollaboratorNeitherSeesNorChangesTheTeam(t *testing.T) {
	_, _, collaborator := teamSetup(t)
	if got := status(t, collaborator.do(http.MethodGet, "/api/orgs/acme/team", nil)); got != http.StatusForbidden {
		t.Errorf("a collaborator read the team: %d", got)
	}
	if got := status(t, collaborator.do(http.MethodPost, "/api/orgs/acme/team",
		map[string]string{"username": "newcomer", "repository": "atlas", "role": "collaborator"})); got != http.StatusForbidden {
		t.Errorf("a collaborator changed the team: %d", got)
	}
}

// TestARepositoryPageSaysWhatEachRoleMayDo, and never returns a stored token.
func TestARepositoryPageSaysWhatEachRoleMayDo(t *testing.T) {
	admin, owner, collaborator := teamSetup(t)

	if got := status(t, admin.do(http.MethodPost, "/api/orgs/acme/repos/atlas/channel",
		map[string]string{"kind": "ntfy", "url": "https://ntfy.example/secret-topic", "token": "tk_very_secret"})); got != http.StatusOK {
		t.Fatalf("setting the channel: %d", got)
	}

	for name, who := range map[string]*instance{"admin": admin, "owner": owner, "collaborator": collaborator} {
		resp := who.do(http.MethodGet, "/api/orgs/acme/repos/atlas/panel", nil)
		raw, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("%s could not open the repository's page: %d", name, resp.StatusCode)
			continue
		}
		text := string(raw)
		if strings.Contains(text, "tk_very_secret") || strings.Contains(text, "secret-topic") {
			t.Errorf("the repository's page returned what the channel stores, to %s: %s", name, text)
		}
		if !strings.Contains(text, `"channel":"ntfy"`) {
			t.Errorf("the page does not say which channel is set, to %s: %s", name, text)
		}
		wantManage := name != "collaborator"
		if strings.Contains(text, `"can_manage":true`) != wantManage {
			t.Errorf("%s: can_manage should be %v: %s", name, wantManage, text)
		}
		// Deleting a repository is whoever runs the organisation's.
		wantDelete := name != "collaborator"
		if strings.Contains(text, `"can_delete":true`) != wantDelete {
			t.Errorf("%s: can_delete should be %v: %s", name, wantDelete, text)
		}
	}

	// Somebody with no role on it does not open it.
	if got := status(t, owner.do(http.MethodGet, "/api/orgs/other/repos/elsewhere/panel", nil)); got != http.StatusForbidden {
		t.Errorf("a repository next door opened: %d", got)
	}
}

// TestAnOrganisationPageShowsEachReaderTheirOwnBoards.
//
// A collaborator on one board opens the organisation to reach that board, and
// is shown that board alone: the others would be doors that do not open. The
// forge is its owner's, and whether it is connected is said without what it
// is connected with.
func TestAnOrganisationPageShowsEachReaderTheirOwnBoards(t *testing.T) {
	admin, owner, collaborator := teamSetup(t)
	admin.createRepo("acme", "second")

	names := func(body map[string]any) []string {
		out := []string{}
		rows, _ := body["repositories"].([]any)
		for _, row := range rows {
			m, _ := row.(map[string]any)
			out = append(out, m["slug"].(string))
		}
		return out
	}

	theirs := decode(t, collaborator.do(http.MethodGet, "/api/orgs/acme/panel", nil))
	if got := names(theirs); len(got) != 1 || got[0] != "atlas" {
		t.Errorf("a collaborator on atlas should see atlas alone: %v", got)
	}
	if theirs["can_manage"] != false {
		t.Errorf("a collaborator was told they may manage the organisation: %v", theirs["can_manage"])
	}
	if _, told := theirs["connection"]; told {
		t.Errorf("a collaborator was told about the forge connection: %v", theirs["connection"])
	}

	owned := decode(t, owner.do(http.MethodGet, "/api/orgs/acme/panel", nil))
	if got := names(owned); len(got) != 2 {
		t.Errorf("the owner should see both repositories: %v", got)
	}
	if owned["can_manage"] != true || owned["can_delete"] != false {
		t.Errorf("an owner manages the organisation and does not end it: %v %v", owned["can_manage"], owned["can_delete"])
	}

	// Somebody with no role in it is refused, rather than shown an empty page.
	if got := status(t, owner.do(http.MethodGet, "/api/orgs/other/panel", nil)); got != http.StatusForbidden {
		t.Errorf("an organisation next door opened: %d", got)
	}

	// The forge, connected and then not, and never what with.
	if got := status(t, admin.do(http.MethodPost, "/api/organizations", map[string]string{
		"slug": "forged", "name": "Forged", "forge": "forgejo",
		"base_url": "https://git.example.com", "forge_owner": "acme",
	})); got != http.StatusCreated {
		t.Fatalf("creating an organisation with a forge: %d", got)
	}
	connection := func() (map[string]any, string) {
		resp := admin.do(http.MethodGet, "/api/orgs/forged/panel", nil)
		raw, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		var body map[string]any
		if err := json.Unmarshal(raw, &body); err != nil {
			t.Fatal(err)
		}
		c, _ := body["connection"].(map[string]any)
		return c, string(raw)
	}
	if c, _ := connection(); c == nil || c["token"] != false {
		t.Errorf("a forge with no token should say it is not connected: %v", c)
	}
	if got := status(t, admin.do(http.MethodPost, "/api/orgs/forged/credentials",
		map[string]string{"token": "tk_very_secret", "webhook_secret": "whs_very_secret"})); got != http.StatusOK {
		t.Fatalf("storing the credentials: %d", got)
	}
	c, raw := connection()
	if c == nil || c["token"] != true || c["secret"] != true {
		t.Errorf("a forge with a token and a secret should say so: %v", c)
	}
	if strings.Contains(raw, "tk_very_secret") || strings.Contains(raw, "whs_very_secret") {
		t.Errorf("the organisation's page returned what the forge credentials are: %s", raw)
	}
}

// TestTheOrganisationsListIsWhatTheReaderMayOpen.
//
// The public list names every organisation, which is right for a visitor. The
// panel's names the ones this person may open, with what they are there and
// what is waiting in the boards they work on.
func TestTheOrganisationsListIsWhatTheReaderMayOpen(t *testing.T) {
	admin, owner, collaborator := teamSetup(t)
	admin.submit("acme", "atlas", "bug", "It crashes on start")

	rows := func(who *instance) map[string]map[string]any {
		body := decode(t, who.do(http.MethodGet, "/api/panel/organizations", nil))
		out := map[string]map[string]any{}
		list, _ := body["organizations"].([]any)
		for _, row := range list {
			m, _ := row.(map[string]any)
			out[m["slug"].(string)] = m
		}
		return out
	}

	theirs := rows(collaborator)
	if len(theirs) != 1 || theirs["acme"] == nil {
		t.Fatalf("a collaborator on atlas should be listed acme alone: %v", theirs)
	}
	if got := theirs["acme"]; got["role"] != "collaborator" || got["repositories"] != float64(1) ||
		got["pending"] != float64(1) || got["can_manage"] != false {
		t.Errorf("a collaborator's row says the wrong thing: %v", got)
	}

	owned := rows(owner)
	if len(owned) != 1 || owned["acme"]["role"] != "owner" || owned["acme"]["can_manage"] != true {
		t.Errorf("an owner should be listed their organisation as its owner: %v", owned)
	}

	all := rows(admin)
	if len(all) != 2 {
		t.Errorf("the administrator should be listed every organisation: %v", all)
	}
	// Created by the administrator, so owned by them: the role they were given
	// there comes before the flag that lets them in everywhere.
	if all["other"]["role"] != "owner" {
		t.Errorf("the administrator's role where they are an owner: %v", all["other"]["role"])
	}
	// And where they hold nothing, what lets them in.
	if got := status(t, admin.do(http.MethodPost, "/api/orgs/other/team",
		map[string]string{"username": "owner", "repository": "", "role": ""})); got != http.StatusOK {
		t.Fatalf("taking the administrator's own role back: %d", got)
	}
	if got := rows(admin)["other"]["role"]; got != "administrator" {
		t.Errorf("the administrator's role where they hold none: %v", got)
	}
}

// TestTheOverviewIsTheReadersBoards.
//
// The same summary for everybody with a role, over what they may see. A
// collaborator on atlas counts atlas, and nothing of the board next door —
// neither its reports, nor its votes, nor the server's people.
func TestTheOverviewIsTheReadersBoards(t *testing.T) {
	admin, _, collaborator := teamSetup(t)
	admin.submit("acme", "atlas", "bug", "It crashes on start")
	admin.submit("other", "elsewhere", "bug", "Somewhere else entirely")
	admin.submit("other", "elsewhere", "idea", "And an idea there too")

	theirs := decode(t, collaborator.do(http.MethodGet, "/api/overview", nil))
	if theirs["server"] != false {
		t.Errorf("a collaborator's overview was said to be the server's: %v", theirs["server"])
	}
	if theirs["pending"] != float64(1) || theirs["repositories"] != float64(1) || theirs["organizations"] != float64(1) {
		t.Errorf("a collaborator on atlas should count atlas alone: pending=%v repositories=%v organizations=%v",
			theirs["pending"], theirs["repositories"], theirs["organizations"])
	}
	if theirs["people"] != float64(0) {
		t.Errorf("a collaborator was told how many people the server has: %v", theirs["people"])
	}
	recent, _ := theirs["recent"].([]any)
	for _, row := range recent {
		if m, _ := row.(map[string]any); m["title"] != "It crashes on start" {
			t.Errorf("a report from another board is in a collaborator's newest: %v", m["title"])
		}
	}
	boards, _ := theirs["boards"].([]any)
	if len(boards) != 1 {
		t.Errorf("a collaborator's work per board should be their one board: %v", boards)
	}

	all := decode(t, admin.do(http.MethodGet, "/api/overview", nil))
	if all["server"] != true || all["pending"] != float64(3) {
		t.Errorf("the administrator's overview is the whole server: server=%v pending=%v", all["server"], all["pending"])
	}
}

// TestAnOwnerDeletesARepositoryButNotTheOrganisation.
//
// The owner runs the organisation, so its repositories are theirs to delete;
// ending the organisation stays the administrator's, and a repository of an
// organisation they do not run is not theirs either. A collaborator deletes
// neither.
func TestAnOwnerDeletesARepositoryButNotTheOrganisation(t *testing.T) {
	admin, owner, collaborator := teamSetup(t)

	if got := status(t, collaborator.do(http.MethodDelete, "/api/orgs/acme/repos/atlas", nil)); got != http.StatusForbidden {
		t.Errorf("a collaborator deleted a repository: %d", got)
	}
	if got := status(t, owner.do(http.MethodDelete, "/api/orgs/other/repos/elsewhere", nil)); got == http.StatusOK {
		t.Errorf("an owner deleted a repository of an organisation they do not run: %d", got)
	}
	if got := status(t, owner.do(http.MethodDelete, "/api/orgs/acme", nil)); got != http.StatusForbidden {
		t.Errorf("an owner ended the organisation: %d", got)
	}
	if got := status(t, owner.do(http.MethodDelete, "/api/orgs/acme/repos/atlas", nil)); got != http.StatusOK {
		t.Fatalf("the owner could not delete a repository of their organisation: %d", got)
	}
	if got := status(t, admin.do(http.MethodGet, "/api/orgs/acme/repos/atlas", nil)); got != http.StatusNotFound {
		t.Errorf("the deleted repository is still there: %d", got)
	}
}
