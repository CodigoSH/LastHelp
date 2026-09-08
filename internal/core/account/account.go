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

// ErrNameTaken is returned when the username already has an account.
var ErrNameTaken = errors.New("that username already has an account")

// Store reads and writes accounts. It takes an Execer rather than a handle, so
// a caller can pass a transaction and make this part of a larger unit of work.
type Store struct {
	DB database.Execer
}

// User is an account as stored.
type User struct {
	ID       int64
	Username string
	// Avatar is a data URL, or empty. Kept in the row so a backup is one file.
	Avatar          string
	DisplayName     string
	IsAdministrator bool
	Disabled        bool
}

// Create adds an account with an already-chosen password.
//
// now is a parameter rather than a call to the clock: the domain does not read
// the wall clock, which is what lets a test place an account in the past
// without sleeping.
func (s Store) Create(username, plainPassword, displayName string, administrator bool, now time.Time) (User, error) {
	username = normaliseUsername(username)
	if username == "" {
		return User{}, errors.New("an account needs a username")
	}

	hash, err := password.Hash(plainPassword)
	if err != nil {
		return User{}, err
	}

	res, err := s.DB.Exec(`INSERT INTO app_user
		(username, password_hash, display_name, is_administrator, created_at)
		VALUES (?, ?, ?, ?, ?)`,
		username, hash, displayName, administrator, now.Unix())
	if err != nil {
		if isUniqueViolation(err) {
			return User{}, ErrNameTaken
		}
		return User{}, fmt.Errorf("creating the account: %w", err)
	}

	id, err := res.LastInsertId()
	if err != nil {
		return User{}, fmt.Errorf("creating the account: %w", err)
	}
	return User{
		ID: id, Username: username, DisplayName: displayName, IsAdministrator: administrator,
	}, nil
}

// Authenticate checks an email and password and returns the account.
//
// A missing account and a wrong password come back as the same error, and both
// paths do the same work: answering faster for an address that does not exist
// is how an attacker enumerates who has an account here.
func (s Store) Authenticate(username, plainPassword string) (User, error) {
	username = normaliseUsername(username)

	var (
		u          User
		storedHash string
	)
	err := s.DB.QueryRow(`SELECT id, username, display_name, is_administrator, disabled, avatar, password_hash
		FROM app_user WHERE username = ?`, username).
		Scan(&u.ID, &u.Username, &u.DisplayName, &u.IsAdministrator, &u.Disabled, &u.Avatar, &storedHash)

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
	err := s.DB.QueryRow(`SELECT id, username, display_name, is_administrator, disabled, avatar
		FROM app_user WHERE id = ?`, userID).
		Scan(&u.ID, &u.Username, &u.DisplayName, &u.IsAdministrator, &u.Disabled, &u.Avatar)

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

// normaliseUsername lowercases and trims, so nobody signs up twice as "Ana"
// and "ana" and then wonders which one they are.
func normaliseUsername(username string) string {
	return strings.ToLower(strings.TrimSpace(username))
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

// List reads every account, with the memberships that say what each one may do.
//
// Ordered by name so the screen is stable: a list that reorders itself between
// visits is one where somebody clicks the wrong row.
func (s Store) List() ([]User, error) {
	rows, err := s.DB.Query(`SELECT id, username, display_name, is_administrator, disabled, avatar
		FROM app_user ORDER BY username`)
	if err != nil {
		return nil, fmt.Errorf("listing accounts: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []User
	for rows.Next() {
		var u User
		if err := rows.Scan(&u.ID, &u.Username, &u.DisplayName, &u.IsAdministrator, &u.Disabled, &u.Avatar); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

// Memberships reads what one account has been granted.
func (s Store) Memberships(userID int64) ([]authz.Membership, error) {
	actor, err := s.Actor(userID)
	if err != nil {
		return nil, err
	}
	return actor.Memberships, nil
}

// SetPassword replaces a password, and only after the old one is proved.
//
// Proving it is the point: a session left open on a shared machine is the
// ordinary way an account is taken, and a change that asked for nothing would
// hand it over permanently.
func (s Store) SetPassword(userID int64, currentPassword, newPassword string) error {
	var storedHash string
	err := s.DB.QueryRow(`SELECT password_hash FROM app_user WHERE id = ?`, userID).Scan(&storedHash)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return ErrNotFound
	case err != nil:
		return fmt.Errorf("looking up the account: %w", err)
	}

	if err := password.Verify(currentPassword, storedHash); err != nil {
		return ErrNotFound
	}

	hash, err := password.Hash(newPassword)
	if err != nil {
		return fmt.Errorf("hashing the password: %w", err)
	}
	if _, err := s.DB.Exec(`UPDATE app_user SET password_hash = ? WHERE id = ?`, hash, userID); err != nil {
		return fmt.Errorf("storing the password: %w", err)
	}
	return nil
}

// SetDisplayName changes the name shown beside a reply.
func (s Store) SetDisplayName(userID int64, name string) error {
	if _, err := s.DB.Exec(`UPDATE app_user SET display_name = ? WHERE id = ?`,
		strings.TrimSpace(name), userID); err != nil {
		return fmt.Errorf("storing the display name: %w", err)
	}
	return nil
}

// SetDisabled turns an account off without deleting what it wrote.
//
// Deleting would take the replies with it — the ON DELETE CASCADE is there for
// a reason — and a thread that loses half a conversation because somebody left
// the team is worse than a row marked disabled.
func (s Store) SetDisabled(userID int64, disabled bool) error {
	if _, err := s.DB.Exec(`UPDATE app_user SET disabled = ? WHERE id = ?`, disabled, userID); err != nil {
		return fmt.Errorf("storing the account state: %w", err)
	}
	return nil
}

// ResetPassword sets a password without asking for the old one.
//
// Only an administrator reaches this, and only for somebody else: it is what
// "I forgot mine" looks like on an installation with no email to send a link
// to. Changing your own still goes through SetPassword, which proves the old
// one first — the two look similar and mean opposite things.
func (s Store) ResetPassword(userID int64, newPassword string) error {
	hash, err := password.Hash(newPassword)
	if err != nil {
		return fmt.Errorf("hashing the password: %w", err)
	}
	if _, err := s.DB.Exec(`UPDATE app_user SET password_hash = ? WHERE id = ?`, hash, userID); err != nil {
		return fmt.Errorf("storing the password: %w", err)
	}
	return nil
}

// Delete removes an account.
//
// The memberships go with it through the foreign key; what it wrote does not,
// because a reply losing its author is better than a thread losing half a
// conversation.
func (s Store) Delete(userID int64) error {
	if _, err := s.DB.Exec(`DELETE FROM app_user WHERE id = ?`, userID); err != nil {
		return fmt.Errorf("deleting the account: %w", err)
	}
	return nil
}

// SetAvatar stores the picture shown beside a name, or clears it.
func (s Store) SetAvatar(userID int64, dataURL string) error {
	if _, err := s.DB.Exec(`UPDATE app_user SET avatar = ? WHERE id = ?`, dataURL, userID); err != nil {
		return fmt.Errorf("storing the avatar: %w", err)
	}
	return nil
}

// Dashboard reads the layout this account arranged for itself.
//
// Empty means "never arranged one", which is not the same as an empty
// dashboard: the screen falls back to its default order, and an account that
// has hidden every card stores that explicitly.
func (s Store) Dashboard(userID int64) (string, error) {
	var layout string
	err := s.DB.QueryRow(`SELECT dashboard FROM app_user WHERE id = ?`, userID).Scan(&layout)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return "", ErrNotFound
	case err != nil:
		return "", fmt.Errorf("reading the dashboard: %w", err)
	}
	return layout, nil
}

// SetDashboard stores it.
func (s Store) SetDashboard(userID int64, layout string) error {
	if _, err := s.DB.Exec(`UPDATE app_user SET dashboard = ? WHERE id = ?`, layout, userID); err != nil {
		return fmt.Errorf("storing the dashboard: %w", err)
	}
	return nil
}
