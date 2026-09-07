// Package account stores the team's accounts and their memberships, and
// resolves one into the actor that authz answers questions about.
//
// It decides nothing. Whether somebody may do a thing is [authz]'s job, and
// this package's only contribution to that question is loading the rows.
package account

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"git.codigosh.com/CodigoSH/LastHelp/internal/core/authz"
	"git.codigosh.com/CodigoSH/LastHelp/internal/platform/database"
	"git.codigosh.com/CodigoSH/LastHelp/internal/platform/password"
)

// ErrNotFound is returned when there is no such account.
//
// It is distinct from a database failure on purpose: masking "the query broke"
// as "no such user" is how a schema problem spends a week being investigated as
// a login bug.
var ErrNotFound = errors.New("account not found")

// ErrEmailTaken is returned when the address already has an account.
var ErrEmailTaken = errors.New("that email address already has an account")

// Store reads and writes accounts. It takes an Execer rather than a handle, so
// a caller can pass a transaction and make this part of a larger unit of work.
type Store struct {
	DB database.Execer
}

// User is an account as stored.
type User struct {
	ID              int64
	Email           string
	DisplayName     string
	IsAdministrator bool
	Disabled        bool
}

// Create adds an account with an already-chosen password.
//
// now is a parameter rather than a call to the clock: the domain does not read
// the wall clock, which is what lets a test place an account in the past
// without sleeping.
func (s Store) Create(email, plainPassword, displayName string, administrator bool, now time.Time) (User, error) {
	email = normaliseEmail(email)
	if email == "" {
		return User{}, errors.New("an account needs an email address")
	}

	hash, err := password.Hash(plainPassword)
	if err != nil {
		return User{}, err
	}

	res, err := s.DB.Exec(`INSERT INTO app_user
		(email, password_hash, display_name, is_administrator, created_at)
		VALUES (?, ?, ?, ?, ?)`,
		email, hash, displayName, administrator, now.Unix())
	if err != nil {
		if isUniqueViolation(err) {
			return User{}, ErrEmailTaken
		}
		return User{}, fmt.Errorf("creating the account: %w", err)
	}

	id, err := res.LastInsertId()
	if err != nil {
		return User{}, fmt.Errorf("creating the account: %w", err)
	}
	return User{
		ID: id, Email: email, DisplayName: displayName, IsAdministrator: administrator,
	}, nil
}

// Authenticate checks an email and password and returns the account.
//
// A missing account and a wrong password come back as the same error, and both
// paths do the same work: answering faster for an address that does not exist
// is how an attacker enumerates who has an account here.
func (s Store) Authenticate(email, plainPassword string) (User, error) {
	email = normaliseEmail(email)

	var (
		u          User
		storedHash string
	)
	err := s.DB.QueryRow(`SELECT id, email, display_name, is_administrator, disabled, password_hash
		FROM app_user WHERE email = ?`, email).
		Scan(&u.ID, &u.Email, &u.DisplayName, &u.IsAdministrator, &u.Disabled, &storedHash)

	switch {
	case errors.Is(err, sql.ErrNoRows):
		// Spend the same work as a real verification would, against a hash of
		// this package's own making, so the two take the same time.
		_ = password.Verify(plainPassword, decoy())
		return User{}, ErrNotFound
	case err != nil:
		return User{}, fmt.Errorf("looking up the account: %w", err)
	}

	if err := password.Verify(plainPassword, storedHash); err != nil {
		return User{}, ErrNotFound
	}
	if u.Disabled {
		return User{}, ErrNotFound
	}
	return u, nil
}

// ByID reads an account, for a screen that has to say which one is acting.
//
// It carries no password material and asks for nothing beyond the identity: a
// lookup used to label a page must not become the second place a credential can
// leak from.
func (s Store) ByID(userID int64) (User, error) {
	var u User
	err := s.DB.QueryRow(`SELECT id, email, display_name, is_administrator, disabled
		FROM app_user WHERE id = ?`, userID).
		Scan(&u.ID, &u.Email, &u.DisplayName, &u.IsAdministrator, &u.Disabled)

	switch {
	case errors.Is(err, sql.ErrNoRows):
		return User{}, ErrNotFound
	case err != nil:
		return User{}, fmt.Errorf("looking up the account: %w", err)
	}
	return u, nil
}

// Actor loads everything authz needs to answer questions about this account.
//
// A repository membership carries the organisation the repository belongs to,
// resolved here, because authz must be able to answer without a database.
func (s Store) Actor(userID int64) (authz.Actor, error) {
	actor := authz.Actor{UserID: userID}

	var disabled bool
	err := s.DB.QueryRow(`SELECT is_administrator, disabled FROM app_user WHERE id = ?`, userID).
		Scan(&actor.IsAdministrator, &disabled)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return authz.Actor{}, ErrNotFound
	case err != nil:
		return authz.Actor{}, fmt.Errorf("loading the account: %w", err)
	}
	if disabled {
		return authz.Actor{}, ErrNotFound
	}

	rows, err := s.DB.Query(`
		SELECT m.scope_kind, m.scope_id, m.role, COALESCE(r.org_id, 0)
		FROM membership m
		LEFT JOIN repository r ON m.scope_kind = 'repo' AND r.id = m.scope_id
		WHERE m.user_id = ?`, userID)
	if err != nil {
		return authz.Actor{}, fmt.Errorf("loading memberships: %w", err)
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var (
			kind  string
			id    int64
			role  string
			orgID int64
		)
		if err := rows.Scan(&kind, &id, &role, &orgID); err != nil {
			return authz.Actor{}, fmt.Errorf("loading memberships: %w", err)
		}
		actor.Memberships = append(actor.Memberships, authz.Membership{
			Scope: authz.Scope{Kind: authz.ScopeKind(kind), ID: id, OrgID: orgID},
			Role:  authz.Role(role),
		})
	}
	if err := rows.Err(); err != nil {
		return authz.Actor{}, fmt.Errorf("loading memberships: %w", err)
	}
	return actor, nil
}

// Grant gives an account a role in one scope, replacing whatever it had there.
//
// ON CONFLICT DO UPDATE, never INSERT OR REPLACE: the second deletes the row
// and inserts a new one, and a delete propagates along every cascade pointing
// at it. That has already cost this house a second factor and every token
// belonging to a restored account.
func (s Store) Grant(userID int64, scope authz.Scope, role authz.Role, now time.Time) error {
	_, err := s.DB.Exec(`INSERT INTO membership (user_id, scope_kind, scope_id, role, created_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT (user_id, scope_kind, scope_id) DO UPDATE SET role = excluded.role`,
		userID, string(scope.Kind), scope.ID, string(role), now.Unix())
	if err != nil {
		return fmt.Errorf("granting %s: %w", role, err)
	}
	return nil
}

// Revoke removes whatever role an account had in one scope.
func (s Store) Revoke(userID int64, scope authz.Scope) error {
	_, err := s.DB.Exec(`DELETE FROM membership WHERE user_id = ? AND scope_kind = ? AND scope_id = ?`,
		userID, string(scope.Kind), scope.ID)
	if err != nil {
		return fmt.Errorf("revoking: %w", err)
	}
	return nil
}

// Count is how the setup flow knows whether this instance has been claimed.
func (s Store) Count() (int, error) {
	var n int
	if err := s.DB.QueryRow(`SELECT count(*) FROM app_user`).Scan(&n); err != nil {
		return 0, fmt.Errorf("counting accounts: %w", err)
	}
	return n, nil
}

func normaliseEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

func isUniqueViolation(err error) bool {
	return err != nil && strings.Contains(err.Error(), "UNIQUE constraint failed")
}

// decoyHash is a real hash, generated rather than written out.
//
// It is verified against when the address does not exist, so that both paths
// cost the same and an attacker cannot tell who has an account here by how fast
// the answer comes back.
//
// Generated, because a hand-written one is a hash that might not parse — and
// Verify returns immediately when it cannot parse, which is exactly the fast
// answer this exists to avoid. The failure would be invisible: the login still
// works, the test still passes, and the timing signal is back.
//
// Computed on first use rather than at startup, so a server nobody has tried to
// sign into wrongly never pays for it.
var decoy = sync.OnceValue(func() string {
	hash, err := password.Hash("this password belongs to nobody")
	if err != nil {
		// Hashing fails only if the system has no randomness, in which case
		// nothing else here works either. An empty string still costs Verify a
		// parse and no more, which is the honest outcome rather than a panic in
		// a login handler.
		return ""
	}
	return hash
})
