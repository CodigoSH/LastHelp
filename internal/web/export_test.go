package web

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"
)

// TestWhatGoesInComesOut.
//
// A product that can only be left by hand is one people stay in for the wrong
// reason, and a self-hosted tool that holds data hostage is a contradiction.
func TestWhatGoesInComesOut(t *testing.T) {
	team := newInstance(t)
	team.claim()
	team.createOrg("codigosh")
	team.createRepo("codigosh", "lastboard")

	public := team.anonymous()
	published, _ := public.submit("codigosh", "lastboard", "idea", "dark mode")
	waiting, _ := public.submit("codigosh", "lastboard", "bug", "still in the queue")

	resp := team.do(http.MethodPost, "/api/items/"+published+"/moderation", map[string]string{"state": "published"})
	_ = resp.Body.Close()
	resp = team.do(http.MethodPost, "/api/items/"+published+"/reply", map[string]string{"body": "looking at it"})
	_ = resp.Body.Close()
	resp = team.do(http.MethodPost, "/api/items/"+published+"/votes", nil)
	_ = resp.Body.Close()

	resp = team.do(http.MethodGet, "/api/orgs/codigosh/repos/lastboard/export", nil)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("exporting returned %d", resp.StatusCode)
	}

	if got := resp.Header.Get("Content-Disposition"); !strings.Contains(got, "codigosh-lastboard.json") {
		t.Errorf("the file is named %q; a folder of exports has to be readable a year later", got)
	}

	var out Export
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("the export is not valid JSON: %v", err)
	}

	if out.Format == "" {
		t.Error("the export has no format name; an importer one day has to know what it is reading")
	}
	if len(out.Items) != 2 {
		t.Fatalf("%d items were exported, want both the published one and the one waiting", len(out.Items))
	}

	byID := map[string]exportItem{}
	for _, entry := range out.Items {
		byID[entry.ID] = entry
	}

	shown, ok := byID[published]
	if !ok {
		t.Fatal("the published item is missing")
	}
	if shown.Title != "dark mode" || shown.Votes != 1 {
		t.Errorf("the published item came out as %+v", shown)
	}
	if len(shown.Comments) != 1 || shown.Comments[0].Body != "looking at it" {
		t.Errorf("the team's reply did not travel: %+v", shown.Comments)
	}

	// The queue is part of what the operator decided. An export that showed
	// only the approved half would misrepresent it.
	if _, ok := byID[waiting]; !ok {
		t.Error("what is still waiting was left out; that is the operator's own record")
	}
}

// TestTheExportCarriesNobodyElsesSecrets.
//
// Taking your data with you must not mean taking everybody's. Reporters' email
// addresses are personal data collected for one purpose, and follow tokens are
// credentials.
func TestTheExportCarriesNobodyElsesSecrets(t *testing.T) {
	team := newInstance(t)
	team.claim()
	team.createOrg("codigosh")
	team.createRepo("codigosh", "lastboard")

	public := team.anonymous()
	base := "/api/orgs/codigosh/repos/lastboard"
	form := decode(t, public.do(http.MethodGet, base+"/form", nil))
	public.tick.advance(5 * time.Second)

	sent := decode(t, public.do(http.MethodPost, base+"/items", map[string]any{
		"kind": "bug", "title": "a bug", "body": "it broke",
		"email": "somebody@example.com", "token": form["token"],
	}))
	followToken, _ := sent["follow_token"].(string)
	if followToken == "" {
		t.Fatal("no follow token came back")
	}

	resp := team.do(http.MethodGet, base+"/export", nil)
	defer func() { _ = resp.Body.Close() }()
	body := make([]byte, 64<<10)
	n, _ := resp.Body.Read(body)
	text := string(body[:n])

	if strings.Contains(text, "somebody@example.com") {
		t.Error("a reporter's address is in the export")
	}
	if strings.Contains(text, followToken) {
		t.Error("a follow token is in the export; that is a credential")
	}
}

// TestOnlyTheTeamCanExport.
func TestOnlyTheTeamCanExport(t *testing.T) {
	team := newInstance(t)
	team.claim()
	team.createOrg("codigosh")
	team.createRepo("codigosh", "lastboard")

	resp := team.anonymous().do(http.MethodGet, "/api/orgs/codigosh/repos/lastboard/export", nil)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("the export answered %d to the public", resp.StatusCode)
	}
}
