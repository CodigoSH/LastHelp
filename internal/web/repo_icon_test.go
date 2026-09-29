package web

import (
	"net/http"
	"strings"
	"testing"
)

// TestARepositoryCarriesTheIconItWasGiven.
//
// A repository shows the initials of its name until somebody gives it an
// icon. The icon is set when it is created or edited, reaches every list that
// draws the repository — the organisation's public page and the front page —
// and survives an edit that does not mention it, which is what a page from
// before icons existed sends.
func TestARepositoryCarriesTheIconItWasGiven(t *testing.T) {
	team := newInstance(t)
	team.claim()
	team.createOrg("codigosh")

	icon := "data:image/png;base64," + strings.Repeat("A", 200)
	if got := status(t, team.do(http.MethodPost, "/api/orgs/codigosh/repositories",
		map[string]any{"slug": "lastboard", "name": "Lastboard", "icon": icon})); got != http.StatusCreated {
		t.Fatalf("creating with an icon answered %d", got)
	}

	publicIcon := func() string {
		t.Helper()
		body := decode(t, team.anonymous().do(http.MethodGet, "/api/orgs/codigosh/repos", nil))
		repos, _ := body["repositories"].([]any)
		if len(repos) != 1 {
			t.Fatalf("the organisation lists %d repositories", len(repos))
		}
		got, _ := repos[0].(map[string]any)["icon"].(string)
		return got
	}
	if publicIcon() != icon {
		t.Error("the organisation's public page does not carry the icon")
	}
	// What a visitor gets: each module's rows under content, the boards module
	// of the default page among them.
	landing := decode(t, team.anonymous().do(http.MethodGet, "/api/landing", nil))
	content, _ := landing["content"].(map[string]any)
	found := false
	for _, rows := range content {
		list, _ := rows.([]any)
		for _, row := range list {
			if b, _ := row.(map[string]any); b["slug"] == "lastboard" && b["org"] == "codigosh" {
				found = b["icon"] == icon
			}
		}
	}
	if !found {
		t.Errorf("the front page's boards module does not carry the icon: %v", content)
	}

	// An edit that does not mention it leaves it where it is.
	edit := map[string]any{"name": "Lastboard", "forge_repo": "", "accepts_bugs": true, "accepts_ideas": true}
	if got := status(t, team.do(http.MethodPatch, "/api/orgs/codigosh/repos/lastboard", edit)); got != http.StatusOK {
		t.Fatalf("an edit without the icon answered %d", got)
	}
	if publicIcon() != icon {
		t.Error("an edit that did not mention the icon wiped it")
	}

	// Sent empty, it goes back to the initials.
	edit["icon"] = ""
	_ = status(t, team.do(http.MethodPatch, "/api/orgs/codigosh/repos/lastboard", edit))
	if got := publicIcon(); got != "" {
		t.Errorf("an empty icon left %q", got[:20])
	}

	// Something that is not an image, and an image past the bound, are refused.
	for name, bad := range map[string]string{
		"a script":  "javascript:alert(1)",
		"too large": "data:image/png;base64," + strings.Repeat("A", maxIconBytes),
	} {
		edit["icon"] = bad
		if got := status(t, team.do(http.MethodPatch, "/api/orgs/codigosh/repos/lastboard", edit)); got != http.StatusBadRequest {
			t.Errorf("%s as an icon answered %d", name, got)
		}
	}
	if got := publicIcon(); got != "" {
		t.Error("a refused icon was stored anyway")
	}
}

// TestAnOrganisationCarriesTheIconItWasGiven.
//
// The same for an organisation: set when it is created or edited, on the
// public list of organisations and on its own page, kept by an edit that does
// not mention it, and gone back to the initials when sent empty.
func TestAnOrganisationCarriesTheIconItWasGiven(t *testing.T) {
	team := newInstance(t)
	team.claim()

	icon := "data:image/png;base64," + strings.Repeat("B", 200)
	if got := status(t, team.do(http.MethodPost, "/api/organizations",
		map[string]any{"slug": "acme", "name": "Acme", "icon": icon})); got != http.StatusCreated {
		t.Fatalf("creating with an icon answered %d", got)
	}

	onList := func() string {
		t.Helper()
		body := decode(t, team.anonymous().do(http.MethodGet, "/api/orgs", nil))
		orgs, _ := body["organizations"].([]any)
		if len(orgs) != 1 {
			t.Fatalf("the public list has %d organisations", len(orgs))
		}
		got, _ := orgs[0].(map[string]any)["icon"].(string)
		return got
	}
	onPage := func() string {
		t.Helper()
		body := decode(t, team.anonymous().do(http.MethodGet, "/api/orgs/acme/repos", nil))
		got, _ := body["organization"].(map[string]any)["icon"].(string)
		return got
	}
	if onList() != icon || onPage() != icon {
		t.Error("the organisation's icon is not on its public list and page")
	}

	if got := status(t, team.do(http.MethodPatch, "/api/orgs/acme", map[string]any{"name": "Acme Inc"})); got != http.StatusOK {
		t.Fatalf("a rename answered %d", got)
	}
	if onList() != icon {
		t.Error("a rename that did not mention the icon wiped it")
	}

	_ = status(t, team.do(http.MethodPatch, "/api/orgs/acme", map[string]any{"name": "Acme Inc", "icon": ""}))
	if onList() != "" {
		t.Error("an empty icon did not go back to the initials")
	}
	if got := status(t, team.do(http.MethodPatch, "/api/orgs/acme",
		map[string]any{"name": "Acme Inc", "icon": "data:text/html;base64,PHNjcmlwdD4="})); got != http.StatusBadRequest {
		t.Errorf("a page as an icon answered %d", got)
	}
}
