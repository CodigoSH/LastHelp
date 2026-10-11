package web

import (
	"net/http"
	"testing"
)

// TestRenamingAnAccountRenamesIt.
//
// An account has one name: the profile's name is the one the
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

	// And a reply written before the rename carries the new name too, for the
	// visitor reading the page and for the team reading its own pane.
	if got := status(t, team.do(http.MethodPost, "/api/me/profile", map[string]string{"username": "alexandra"})); got != http.StatusOK {
		t.Fatalf("renaming again: %d", got)
	}
	for who, client := range map[string]*instance{"a visitor": team.anonymous(), "the team": team} {
		thread, _ := decode(t, client.do(http.MethodGet, "/api/items/"+published, nil))["comments"].([]any)
		if len(thread) != 1 || thread[0].(map[string]any)["name"] != "alexandra" {
			t.Errorf("%s reads the reply signed %v, want the name the account has now", who, thread)
		}
	}
	if got := status(t, team.do(http.MethodPost, "/api/me/profile", map[string]string{"username": "alex"})); got != http.StatusOK {
		t.Fatalf("renaming back: %d", got)
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

// TestARenameClaimsTheRepliesThatNoAccountOwnedYet.
//
// The upgrade matched an old reply to the account holding its name that day, and only
// then. An account renamed before the upgrade, or since, was left with replies that
// kept the name they were written under whatever it was called afterwards. Renaming it
// is the moment it says which name was its own: the replies still unclaimed under the
// name it leaves follow it from then on.
func TestARenameClaimsTheRepliesThatNoAccountOwnedYet(t *testing.T) {
	team := newInstance(t)
	team.claim()
	team.createOrg("codigosh")
	team.createRepo("codigosh", "lastboard")
	if got := status(t, team.do(http.MethodPost, "/api/people", map[string]string{
		"username": "helper", "password": "a good long passphrase",
	})); got != http.StatusCreated {
		t.Fatalf("creating a second account: %d", got)
	}
	published, _ := team.anonymous().submit("codigosh", "lastboard", "idea", "dark mode")
	_ = status(t, team.do(http.MethodPost, "/api/items/"+published+"/moderation", map[string]string{"state": "published"}))
	for _, body := range []string{"first", "second", "third", "fourth"} {
		_ = status(t, team.do(http.MethodPost, "/api/items/"+published+"/reply", map[string]string{"body": body}))
	}

	// As the upgrade left them. The account is "owner" and never was "kiwinho". The
	// third is by somebody who is nobody's account now; the fourth is already
	// claimed by the helper, with a stale copy of the name "owner" in it.
	for body, ref := range map[string]string{"first": "kiwinho", "second": "owner", "third": "somebody-else", "fourth": "owner"} {
		if _, err := team.db().Exec(`UPDATE comment SET author_id = NULL, author_ref = ? WHERE body = ?`, ref, body); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := team.db().Exec(`UPDATE comment SET author_id = (SELECT id FROM app_user WHERE username = 'helper') WHERE body = 'fourth'`); err != nil {
		t.Fatal(err)
	}
	signed := func() map[string]string {
		thread, _ := decode(t, team.anonymous().do(http.MethodGet, "/api/items/"+published, nil))["comments"].([]any)
		names := map[string]string{}
		for _, c := range thread {
			m := c.(map[string]any)
			names[m["body"].(string)], _ = m["name"].(string)
		}
		return names
	}
	rename := func(to string) {
		t.Helper()
		if got := status(t, team.do(http.MethodPost, "/api/me/profile", map[string]string{"username": to})); got != http.StatusOK {
			t.Fatalf("renaming to %s: %d", to, got)
		}
	}
	want := func(step string, first, second string) {
		t.Helper()
		got := signed()
		if got["first"] != first || got["second"] != second || got["third"] != "somebody-else" || got["fourth"] != "helper" {
			t.Errorf("%s: %v, want first %q, second %q, the stranger's untouched and the helper's still the helper's", step, got, first, second)
		}
	}

	// Leaving "owner" claims what was signed "owner" and was claimed by nobody.
	rename("k4wit0")
	want("after leaving the name it had", "kiwinho", "k4wit0")
	// Taking "kiwinho" proves nothing about who wrote what was signed so...
	rename("kiwinho")
	want("after taking a name", "kiwinho", "kiwinho")
	// ...but leaving it does: from then on both follow every rename.
	rename("k4wit0")
	want("after leaving the name it took", "k4wit0", "k4wit0")
	rename("alex")
	want("after one more rename", "alex", "alex")
}
