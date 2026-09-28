package web

import (
	"net/http"
	"testing"
)

// TestRenamingAnAccountRenamesIt.
//
// An account has one name, as on Lastboard: the profile's name is the one the
// account signs in with and the one its replies carry. Renaming it used to
// change a separate "display" name and leave the sign-in name as it was.
func TestRenamingAnAccountRenamesIt(t *testing.T) {
	team := newInstance(t)
	team.claim()
	team.createOrg("codigosh")
	team.createRepo("codigosh", "lastboard")

	if got := status(t, team.do(http.MethodPost, "/api/me/profile", map[string]string{"username": "  Alex "})); got != http.StatusOK {
		t.Fatalf("renaming: %d", got)
	}
	me := decode(t, team.do(http.MethodGet, "/api/me", nil))
	if me["username"] != "alex" {
		t.Errorf("after renaming, the account is called %v", me["username"])
	}

	signIn := func(name string) int {
		return status(t, team.anonymous().do(http.MethodPost, "/api/session", map[string]string{
			"username": name, "password": "a good long passphrase",
		}))
	}
	if got := signIn("alex"); got != http.StatusOK {
		t.Errorf("the new name does not sign in: %d", got)
	}
	if got := signIn("owner"); got == http.StatusOK {
		t.Error("the old name still signs in")
	}

	// A reply carries the name the account has now.
	published, _ := team.anonymous().submit("codigosh", "lastboard", "idea", "dark mode")
	_ = status(t, team.do(http.MethodPost, "/api/items/"+published+"/moderation", map[string]string{"state": "published"}))
	_ = status(t, team.do(http.MethodPost, "/api/items/"+published+"/reply", map[string]string{"body": "looking at it"}))
	thread, _ := decode(t, team.anonymous().do(http.MethodGet, "/api/items/"+published, nil))["comments"].([]any)
	if len(thread) != 1 || thread[0].(map[string]any)["name"] != "alex" {
		t.Errorf("the reply is signed %v, want the account's name", thread)
	}

	// Somebody else's name is refused, and so is a name too short to be one.
	if got := status(t, team.do(http.MethodPost, "/api/people", map[string]string{
		"username": "helper", "password": "a good long passphrase",
	})); got != http.StatusCreated {
		t.Fatalf("creating a second account: %d", got)
	}
	if got := status(t, team.do(http.MethodPost, "/api/me/profile", map[string]string{"username": "Helper"})); got != http.StatusConflict {
		t.Errorf("taking somebody else's name answered %d", got)
	}
	if got := status(t, team.do(http.MethodPost, "/api/me/profile", map[string]string{"username": "x"})); got != http.StatusBadRequest {
		t.Errorf("a one-letter name answered %d", got)
	}
	if me := decode(t, team.do(http.MethodGet, "/api/me", nil)); me["username"] != "alex" {
		t.Errorf("a refused rename changed the name to %v", me["username"])
	}
}
