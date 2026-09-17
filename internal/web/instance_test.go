package web

import (
	"net/http"
	"testing"

	"git.codigosh.com/CodigoSH/LastHelp/internal/core/account"
	"git.codigosh.com/CodigoSH/LastHelp/internal/core/authz"
)

// TestComposingTheFrontPage.
//
// The front page is the one screen a stranger sees, so the two halves of this
// are checked together: that anybody may read what it is made of, and that only
// an administrator may change it.
func TestComposingTheFrontPage(t *testing.T) {
	i := newInstance(t)
	i.claim()

	// Before anybody composes anything, it is the page that was always there.
	body := decode(t, i.anonymous().do(http.MethodGet, "/api/landing", nil))
	modules, _ := body["modules"].([]any)
	if len(modules) != 3 {
		t.Fatalf("an untouched installation offered %d modules, want 3", len(modules))
	}

	// The administrator composes it.
	resp := i.do(http.MethodPut, "/api/landing", map[string]any{
		"modules": []map[string]any{
			{"id": "words", "kind": "text", "width": "full", "body": "We read everything."},
			{"id": "orgs", "kind": "organisations", "width": "full"},
		},
	})
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("the administrator could not compose the page: %d", resp.StatusCode)
	}

	// And a stranger sees what was composed.
	body = decode(t, i.anonymous().do(http.MethodGet, "/api/landing", nil))
	modules, _ = body["modules"].([]any)
	if len(modules) != 2 {
		t.Fatalf("a visitor was shown %d modules, want 2", len(modules))
	}
	first, _ := modules[0].(map[string]any)
	if first["body"] != "We read everything." {
		t.Errorf("the operator's words came back as %v", first["body"])
	}

	// A module nothing can draw is refused, and the page is left as it was.
	resp = i.do(http.MethodPut, "/api/landing", map[string]any{
		"modules": []map[string]any{{"id": "x", "kind": "iframe", "width": "full"}},
	})
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("a module nothing can draw was accepted: %d", resp.StatusCode)
	}
	body = decode(t, i.anonymous().do(http.MethodGet, "/api/landing", nil))
	if modules, _ := body["modules"].([]any); len(modules) != 2 {
		t.Errorf("a refused layout changed the page: %d modules", len(modules))
	}

	// A stranger cannot compose it. This one is stopped at the door, before any
	// handler runs.
	resp = i.anonymous().do(http.MethodPut, "/api/landing", map[string]any{
		"modules": []map[string]any{{"id": "a", "kind": "hero", "width": "full"}},
	})
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized && resp.StatusCode != http.StatusForbidden {
		t.Errorf("a stranger composed the front page: %d", resp.StatusCode)
	}

	// And neither can somebody who IS signed in and is not an administrator —
	// which is the half the door does not answer. A collaborator triages a
	// board; the front page belongs to whoever runs the server.
	accounts := account.Store{DB: i.db()}
	helper, err := accounts.Create("helper", "a good long passphrase", "Helper", false, i.now())
	if err != nil {
		t.Fatal(err)
	}
	var boardID int64
	i.createOrg("codigosh")
	i.createRepo("codigosh", "lastboard")
	if err := i.db().QueryRow(`SELECT id FROM repository WHERE slug = 'lastboard'`).Scan(&boardID); err != nil {
		t.Fatal(err)
	}
	if err := accounts.Grant(helper.ID, authz.Repository(boardID, 0), authz.RoleCollaborator, i.now()); err != nil {
		t.Fatal(err)
	}
	helperClient := i.anonymous()
	resp = helperClient.do(http.MethodPost, "/api/session", map[string]string{
		"username": "helper", "password": "a good long passphrase",
	})
	_ = resp.Body.Close()

	resp = helperClient.do(http.MethodPut, "/api/landing", map[string]any{
		"modules": []map[string]any{{"id": "a", "kind": "hero", "width": "full"}},
	})
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("a collaborator composed the front page: %d", resp.StatusCode)
	}
}

// TestMostVotedShowsIdeasAndOnlyIdeas.
//
// Only an idea can be voted for, so a "most voted" that included bugs would be
// ranking a list of things that all have nought and putting whichever is newest
// at the top. The front page promises this module by name — "ideas collect
// votes, which shows what is most requested" — so what it shows has to be that.
//
// Proved here rather than in the browser pass: that journey has one published
// idea and no published bug when it reaches the front page, so the module looks
// identical with the filter and without it.
func TestMostVotedShowsIdeasAndOnlyIdeas(t *testing.T) {
	i := newInstance(t)
	i.claim()
	i.createOrg("acme")
	i.createRepo("acme", "atlas")

	idea, _ := i.submit("acme", "atlas", "idea", "dark mode for the dashboard")
	bug, _ := i.submit("acme", "atlas", "bug", "the sidebar forgets its width")
	for _, id := range []string{idea, bug} {
		resp := i.do(http.MethodPost, "/api/items/"+id+"/moderation", map[string]string{"state": "published"})
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("publishing %s: %d", id, resp.StatusCode)
		}
	}

	resp := i.do(http.MethodPut, "/api/landing", map[string]any{
		"modules": []map[string]any{
			{"id": "wanted", "kind": "most_voted", "x": 1, "y": 1, "w": 12, "h": 3},
		},
	})
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("composing a page with a most voted module: %d", resp.StatusCode)
	}

	body := decode(t, i.anonymous().do(http.MethodGet, "/api/landing", nil))
	content, _ := body["content"].(map[string]any)
	rows, _ := content["wanted"].([]any)
	if len(rows) != 1 {
		t.Fatalf("the module carried %d reports, want only the idea", len(rows))
	}
	only, _ := rows[0].(map[string]any)
	if only["kind"] != "idea" || only["title"] != "dark mode for the dashboard" {
		t.Errorf("the module carried %v, which is not the idea", only)
	}
}

// TestAStrangerIsHandedThePageAndNothingElse.
//
// The board list travels with the layout for whoever is going to compose it,
// and for nobody else: it is the picker in the module dialog, not something a
// visitor has any use for.
//
// This deliberately does NOT claim the default page skips the board query —
// that is what layoutUsesData decides, and it has its own test. Asserting it
// here would be asserting it from the response shape, which looks identical
// whether the query ran or not: the first version of this was named for the
// query and passed with the skip mutated away.
func TestAStrangerIsHandedThePageAndNothingElse(t *testing.T) {
	i := newInstance(t)
	i.claim()

	body := decode(t, i.anonymous().do(http.MethodGet, "/api/landing", nil))
	if _, carried := body["content"]; carried {
		t.Errorf("a default page came back carrying content: %v", body["content"])
	}
	if _, carried := body["boards"]; carried {
		t.Errorf("a stranger was handed the board list: %v", body["boards"])
	}
}
