package authz

import "testing"

const (
	orgCodigosh = int64(1)
	orgJellyfin = int64(2)
	repoBoard   = int64(10) // in orgCodigosh
	repoDav     = int64(11) // in orgCodigosh
	repoAndroid = int64(20) // in orgJellyfin
)

var (
	nobody = Actor{}

	admin = Actor{UserID: 1, IsAdministrator: true}

	// Owns one organisation and nothing in the other.
	owner = Actor{UserID: 2, Memberships: []Membership{
		{Scope: Organization(orgCodigosh), Role: RoleOwner},
	}}

	// Triages one repository and nothing else.
	collaborator = Actor{UserID: 3, Memberships: []Membership{
		{Scope: Repository(repoBoard, orgCodigosh), Role: RoleCollaborator},
	}}
)

func TestWhoCanDoWhat(t *testing.T) {
	cases := []struct {
		name   string
		actor  Actor
		action Action
		scope  Scope
		want   bool
	}{
		// Nobody. Every one of these is the answer a bug should fall back to.
		{"nobody triages nothing", nobody, Triage, Repository(repoBoard, orgCodigosh), false},
		{"nobody manages nothing", nobody, ManageOrganization, Organization(orgCodigosh), false},
		{"nobody sees the panel", nobody, ViewPanel, Scope{}, false},

		// The administrator is the only one who crosses organisations.
		{"admin manages the server", admin, ManageServer, Scope{}, true},
		{"admin manages any organisation", admin, ManageOrganization, Organization(orgJellyfin), true},
		{"admin triages any repository", admin, Triage, Repository(repoAndroid, orgJellyfin), true},

		// An owner runs their own organisation and everything under it.
		{"owner manages their organisation", owner, ManageOrganization, Organization(orgCodigosh), true},
		{"owner reads their own token", owner, ReadForgeCredentials, Organization(orgCodigosh), true},
		{"owner triages a repository of theirs without a row for it",
			owner, Triage, Repository(repoDav, orgCodigosh), true},

		// And nothing at all in somebody else's. This is the isolation the two
		// levels exist for.
		{"owner does not manage another organisation",
			owner, ManageOrganization, Organization(orgJellyfin), false},
		{"owner does not triage another organisation's repository",
			owner, Triage, Repository(repoAndroid, orgJellyfin), false},
		{"owner does not read another organisation's token",
			owner, ReadForgeCredentials, Organization(orgJellyfin), false},
		{"owner does not manage the server", owner, ManageServer, Scope{}, false},

		// A collaborator triages their repository and only theirs.
		{"collaborator triages their repository",
			collaborator, Triage, Repository(repoBoard, orgCodigosh), true},
		{"collaborator does not triage the one next door",
			collaborator, Triage, Repository(repoDav, orgCodigosh), false},
		{"collaborator does not manage the organisation",
			collaborator, ManageOrganization, Organization(orgCodigosh), false},
		{"collaborator sees the panel", collaborator, ViewPanel, Scope{}, true},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.actor.Can(c.action, c.scope); got != c.want {
				t.Errorf("Can(%s, %+v) = %v, want %v", c.action, c.scope, got, c.want)
			}
		})
	}
}

// TestACollaboratorNeverReadsTheToken is stated on its own because it is the
// reason the two levels exist at all.
//
// A collaborator is somebody trusted to triage a repository. That is not the
// same as somebody trusted with a credential that can write to every repository
// the organisation owns, and the difference is the entire blast radius.
func TestACollaboratorNeverReadsTheToken(t *testing.T) {
	scopes := []Scope{
		Repository(repoBoard, orgCodigosh), // their own
		Organization(orgCodigosh),          // the organisation it lives in
	}
	for _, s := range scopes {
		if collaborator.Can(ReadForgeCredentials, s) {
			t.Errorf("a collaborator could read the forge credentials at %+v", s)
		}
	}
}

// TestAnUnknownActionIsRefused.
//
// The next person adds an action. If Can fell through to true for anything it
// did not recognise, that endpoint would ship open and every existing test
// would still pass.
func TestAnUnknownActionIsRefused(t *testing.T) {
	if owner.Can(Action("delete_everything"), Organization(orgCodigosh)) {
		t.Error("an action nobody wrote a rule for was allowed")
	}
	// The administrator is deliberately excepted: they can do everything,
	// including things nobody has written a rule for yet.
	if !admin.Can(Action("delete_everything"), Scope{}) {
		t.Error("the administrator should not be stopped by an unknown action")
	}
}

// TestARepositoryScopeWithoutItsOrganisationDoesNotEscalate.
//
// Scope.OrgID is filled in by the caller from a database row. A caller that
// forgets it must lose the answer, not gain one: if a missing 0 matched an
// owner's organisation, forgetting the field would hand every owner every
// repository in the instance.
func TestARepositoryScopeWithoutItsOrganisationDoesNotEscalate(t *testing.T) {
	orphan := Scope{Kind: ScopeRepository, ID: repoBoard} // OrgID left at zero

	if owner.Can(Triage, orphan) {
		t.Error("an owner triaged a repository whose organisation was not resolved")
	}
	if owner.Can(ReadForgeCredentials, orphan) {
		t.Error("an owner read credentials through a repository whose organisation was not resolved")
	}

	// The collaborator still matches, because their membership is on the
	// repository itself and needs no organisation to be resolved.
	if !collaborator.Can(Triage, orphan) {
		t.Error("a collaborator's own repository stopped matching")
	}
}

func TestStandingInNamesWhatSomebodyIsInAnOrganisation(t *testing.T) {
	const acme, other = 1, 2
	owner := Actor{UserID: 2, Memberships: []Membership{{Role: RoleOwner, Scope: Organization(acme)}}}
	collaborator := Actor{UserID: 3, Memberships: []Membership{{Role: RoleCollaborator, Scope: Repository(10, acme)}}}
	both := Actor{UserID: 4, Memberships: []Membership{
		{Role: RoleCollaborator, Scope: Repository(10, acme)},
		{Role: RoleOwner, Scope: Organization(acme)},
	}}
	admin := Actor{UserID: 1, IsAdministrator: true}

	cases := []struct {
		name  string
		actor Actor
		org   int64
		want  string
	}{
		{"an owner", owner, acme, "owner"},
		{"an owner elsewhere", owner, other, ""},
		{"a collaborator", collaborator, acme, "collaborator"},
		{"a collaborator seen from another organisation", collaborator, other, ""},
		{"owning outranks collaborating", both, acme, "owner"},
		{"the administrator where they hold nothing", admin, acme, "administrator"},
	}
	for _, c := range cases {
		if got := c.actor.StandingIn(c.org); got != c.want {
			t.Errorf("%s: StandingIn = %q, want %q", c.name, got, c.want)
		}
	}
}
