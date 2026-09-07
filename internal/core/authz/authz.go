// Package authz is the only place that decides who may do what.
//
// An architecture rule fails the build if any other package compares against a
// role name. That rule is not tidiness: SECURITY.md and the README describe what
// each role can reach, and that promise is worth exactly as much as the number
// of places that can violate it. This pins that number at one.
//
// Nothing here touches the database. An actor arrives already resolved, which
// is what lets every rule below be a table test rather than a fixture.
package authz

// Role is what somebody is within one scope. There are two, and the third —
// administrator — is not a role here because it is not scoped: it is a property
// of the account, checked separately below.
type Role string

const (
	// RoleOwner runs an organisation: its forge connection, its token, its
	// repositories, and everything a collaborator can do in any of them.
	RoleOwner Role = "owner"

	// RoleCollaborator triages one repository. Never sees the forge token —
	// that is the blast radius the two levels exist to limit.
	RoleCollaborator Role = "collaborator"
)

// ScopeKind is the level a membership or a question is about.
type ScopeKind string

const (
	ScopeOrganization ScopeKind = "org"
	ScopeRepository   ScopeKind = "repo"
)

// Scope is what a question is about.
//
// A repository scope carries the organisation it belongs to. That is on purpose:
// resolving it is a database question, and answering "may this person triage
// this repository" must not need one, or the decision ends up being made in
// whatever handler already had the row to hand.
type Scope struct {
	Kind  ScopeKind
	ID    int64
	OrgID int64 // set for ScopeRepository; the organisation the repository is in
}

// RoleFrom turns what arrived on the wire into a role, or refuses.
//
// It lives here because this package is the only one allowed to know how a role
// is spelled: a handler that compared strings itself would be a second copy of
// the rules, and the two drift the first time one is updated.
func RoleFrom(value string) (Role, bool) {
	switch Role(value) {
	case RoleOwner:
		return RoleOwner, true
	case RoleCollaborator:
		return RoleCollaborator, true
	default:
		return "", false
	}
}

// ScopeFrom turns a scope kind and an identifier into a scope, or refuses.
func ScopeFrom(kind string, id, orgID int64) (Scope, bool) {
	switch ScopeKind(kind) {
	case ScopeOrganization:
		return Organization(id), true
	case ScopeRepository:
		return Repository(id, orgID), true
	default:
		return Scope{}, false
	}
}

func Organization(id int64) Scope { return Scope{Kind: ScopeOrganization, ID: id} }

func Repository(id, orgID int64) Scope {
	return Scope{Kind: ScopeRepository, ID: id, OrgID: orgID}
}

// Membership is one row of the membership table, resolved.
type Membership struct {
	Scope Scope
	Role  Role
}

// Actor is who is asking.
type Actor struct {
	UserID int64

	// IsAdministrator is a flag on the account rather than a membership,
	// because it is about the server and not about any one organisation. It is
	// the only thing that crosses organisations.
	IsAdministrator bool

	Memberships []Membership
}

// Action is what somebody is trying to do.
type Action string

const (
	// ManageServer covers what belongs to nobody's organisation: creating
	// organisations and accounts, and the server's own settings.
	ManageServer Action = "manage_server"

	// ManageOrganization covers a connection and its repositories.
	ManageOrganization Action = "manage_organization"

	// ReadForgeCredentials is separate from managing an organisation, and is
	// asked wherever a token or a webhook secret would be revealed. A
	// collaborator triages happily without ever being able to answer "what is
	// the token".
	ReadForgeCredentials Action = "read_forge_credentials"

	// Triage is the daily work: approving and rejecting what arrives, changing
	// status, replying, and creating or linking an issue.
	Triage Action = "triage"

	// ViewPanel is the door to the panel at all.
	ViewPanel Action = "view_panel"
)

// Can answers the only question this package exists for.
//
// The zero Actor — nobody, not signed in — can do nothing, which is the
// behaviour a bug should fall back to.
func (a Actor) Can(action Action, on Scope) bool {
	if a.IsAdministrator {
		return true
	}

	switch action {
	case ManageServer:
		// Not reachable by any membership: it is not about an organisation.
		return false

	case ManageOrganization, ReadForgeCredentials:
		// Only at the level the thing lives at. Asking this about a repository
		// is a question about its organisation, so it is answered there.
		switch on.Kind {
		case ScopeOrganization:
			return a.hasRole(RoleOwner, ScopeOrganization, on.ID)
		case ScopeRepository:
			return on.OrgID != 0 && a.hasRole(RoleOwner, ScopeOrganization, on.OrgID)
		}
		return false

	case Triage:
		switch on.Kind {
		case ScopeRepository:
			if a.hasRole(RoleCollaborator, ScopeRepository, on.ID) {
				return true
			}
			// An owner triages anything in their own organisation without
			// needing a row per repository.
			return on.OrgID != 0 && a.hasRole(RoleOwner, ScopeOrganization, on.OrgID)
		case ScopeOrganization:
			return a.hasRole(RoleOwner, ScopeOrganization, on.ID)
		}
		return false

	case ViewPanel:
		return len(a.Memberships) > 0
	}

	// An action nobody taught this function about is refused. The alternative —
	// falling through to true — is how a new endpoint ends up open.
	return false
}

func (a Actor) hasRole(role Role, kind ScopeKind, id int64) bool {
	for _, m := range a.Memberships {
		if m.Role == role && m.Scope.Kind == kind && m.Scope.ID == id {
			return true
		}
	}
	return false
}
