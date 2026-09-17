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
