package account

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"git.codigosh.com/CodigoSH/LastHelp/internal/core/authz"
	"git.codigosh.com/CodigoSH/LastHelp/internal/platform/database"
	"git.codigosh.com/CodigoSH/LastHelp/internal/platform/password"
)

var epoch = time.Unix(1_700_000_000, 0)

func store(t *testing.T) Store {
	t.Helper()
	db, err := database.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return Store{DB: db}
}

func TestCreateAndAuthenticate(t *testing.T) {
	s := store(t)

	created, err := s.Create("Person@Example.COM ", "a good long passphrase", "Person", true, epoch)
	if err != nil {
		t.Fatal(err)
	}
	if created.Email != "person@example.com" {
		t.Errorf("email stored as %q; addresses are normalised so nobody signs up twice with capitals", created.Email)
	}

	got, err := s.Authenticate("person@example.com", "a good long passphrase")
	if err != nil {
		t.Fatalf("the right password did not authenticate: %v", err)
	}
	if got.ID != created.ID || !got.IsAdministrator {
		t.Errorf("got %+v, want the administrator that was just created", got)
	}

	if _, err := s.Authenticate("person@example.com", "wrong"); !errors.Is(err, ErrNotFound) {
		t.Errorf("wrong password gave %v", err)
	}
}

// TestAMissingAccountAndAWrongPasswordAreIndistinguishable.
//
// Not just the same error: the same work. Answering faster for an address with
// no account is how somebody enumerates who has one here.
func TestAMissingAccountAndAWrongPasswordAreIndistinguishable(t *testing.T) {
	s := store(t)
	if _, err := s.Create("real@example.com", "a good long passphrase", "Real", false, epoch); err != nil {
		t.Fatal(err)
	}

	_, missing := s.Authenticate("nobody@example.com", "guess")
	_, wrong := s.Authenticate("real@example.com", "guess")
	if !errors.Is(missing, ErrNotFound) || !errors.Is(wrong, ErrNotFound) {
		t.Fatalf("different errors: missing=%v wrong=%v", missing, wrong)
	}
}

// TestTheDecoyHashIsRealWork is the test for the trap in the line above.
//
// The decoy exists to make the missing-account path cost what a real
// verification costs. A decoy that does not parse makes Verify return
// immediately, which is the fast answer it was written to avoid — and nothing
// would fail: login still works, the errors still match, and the timing signal
// is quietly back.
func TestTheDecoyHashIsRealWork(t *testing.T) {
	h := decoy()
	if h == "" {
		t.Fatal("the decoy is empty")
	}
	if !strings.HasPrefix(h, "$argon2id$") {
		t.Fatalf("the decoy is not an argon2id hash: %q", h)
	}
	// If it parses, Verify does the full derivation before failing. If it did
	// not parse, it would fail on the format instead — the same error, none of
	// the work.
	if err := password.Verify("anything at all", h); !errors.Is(err, password.ErrMismatch) {
		t.Fatalf("verifying against the decoy gave %v", err)
	}

	fast := time.Now()
	_ = password.Verify("anything at all", "$argon2id$not-a-hash")
	parseOnly := time.Since(fast)

	slow := time.Now()
	_ = password.Verify("anything at all", h)
	realWork := time.Since(slow)

	if realWork < parseOnly*5 {
		t.Errorf("verifying against the decoy took %v while failing to parse took %v: "+
			"the decoy is not costing what a real verification costs", realWork, parseOnly)
	}
}

func TestADisabledAccountCannotSignIn(t *testing.T) {
	s := store(t)
	u, err := s.Create("gone@example.com", "a good long passphrase", "Gone", false, epoch)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB.Exec(`UPDATE app_user SET disabled = 1 WHERE id = ?`, u.ID); err != nil {
		t.Fatal(err)
	}

	if _, err := s.Authenticate("gone@example.com", "a good long passphrase"); !errors.Is(err, ErrNotFound) {
		t.Error("a disabled account signed in")
	}
	if _, err := s.Actor(u.ID); !errors.Is(err, ErrNotFound) {
		t.Error("a disabled account still resolved to an actor, so an existing session would keep working")
	}
}

func TestTheSameAddressCannotBeTakenTwice(t *testing.T) {
	s := store(t)
	if _, err := s.Create("dup@example.com", "a good long passphrase", "", false, epoch); err != nil {
		t.Fatal(err)
	}
	_, err := s.Create("DUP@example.com", "another passphrase entirely", "", false, epoch)
	if !errors.Is(err, ErrEmailTaken) {
		t.Errorf("got %v, want ErrEmailTaken — and note the capitals: normalisation is what makes this collide", err)
	}
}

// TestActorResolvesTheOrganisationOfEachRepository.
//
// authz refuses to answer for a repository whose organisation is zero, which is
// deliberate: a caller that forgets it loses the answer instead of gaining one.
// This is the loader holding up its end.
func TestActorResolvesTheOrganisationOfEachRepository(t *testing.T) {
	s := store(t)
	seed(t, s)

	u, err := s.Create("helper@example.com", "a good long passphrase", "Helper", false, epoch)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Grant(u.ID, authz.Repository(1, 0), authz.RoleCollaborator, epoch); err != nil {
		t.Fatal(err)
	}

	actor, err := s.Actor(u.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(actor.Memberships) != 1 {
		t.Fatalf("got %d memberships, want 1", len(actor.Memberships))
	}
	if got := actor.Memberships[0].Scope.OrgID; got != 1 {
		t.Errorf("the repository membership came back with OrgID %d, want 1 — "+
			"authz cannot resolve it and would refuse every question about this repository", got)
	}
	if !actor.Can(authz.Triage, authz.Repository(1, 1)) {
		t.Error("the collaborator cannot triage their own repository")
	}
}

// TestGrantReplacesInsteadOfDeleting.
//
// INSERT OR REPLACE deletes the row and inserts a new one, and a delete travels
// along every cascade pointing at it. Changing somebody's role must not be a
// way to lose anything attached to that membership.
func TestGrantReplacesInsteadOfDeleting(t *testing.T) {
	s := store(t)
	seed(t, s)

	u, err := s.Create("promoted@example.com", "a good long passphrase", "", false, epoch)
	if err != nil {
		t.Fatal(err)
	}
	scope := authz.Organization(1)
	if err := s.Grant(u.ID, scope, authz.RoleCollaborator, epoch); err != nil {
		t.Fatal(err)
	}

	var firstID int64
	if err := s.DB.QueryRow(`SELECT id FROM membership WHERE user_id = ?`, u.ID).Scan(&firstID); err != nil {
		t.Fatal(err)
	}

	if err := s.Grant(u.ID, scope, authz.RoleOwner, epoch); err != nil {
		t.Fatal(err)
	}

	var secondID int64
	var role string
	if err := s.DB.QueryRow(`SELECT id, role FROM membership WHERE user_id = ?`, u.ID).
		Scan(&secondID, &role); err != nil {
		t.Fatal(err)
	}
	if role != string(authz.RoleOwner) {
		t.Errorf("role is %q, want owner", role)
	}
	if secondID != firstID {
		t.Errorf("the membership row was replaced (id %d became %d) rather than updated: "+
			"that is a delete, and a delete propagates along cascades", firstID, secondID)
	}
}

func TestCountIsHowSetupKnowsTheInstanceIsUnclaimed(t *testing.T) {
	s := store(t)

	n, err := s.Count()
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("a fresh instance reports %d accounts", n)
	}

	if _, err := s.Create("first@example.com", "a good long passphrase", "", true, epoch); err != nil {
		t.Fatal(err)
	}
	if n, _ = s.Count(); n != 1 {
		t.Errorf("after the first account, count is %d", n)
	}
}

func seed(t *testing.T, s Store) {
	t.Helper()
	if _, err := s.DB.Exec(`INSERT INTO organization (id, slug, name, forge_kind, base_url, forge_owner, created_at)
		VALUES (1, 'codigosh', 'CodigoSH', 'forgejo', 'https://git.example.com', 'CodigoSH', 0)`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB.Exec(`INSERT INTO repository (id, org_id, slug, name, forge_repo, created_at)
		VALUES (1, 1, 'lastboard', 'Lastboard', 'CodigoSH/Lastboard', 0)`); err != nil {
		t.Fatal(err)
	}
}
