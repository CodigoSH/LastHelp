// Package session issues and resolves the team's sessions.
//
// The public never has one: reporting is anonymous and a reporter comes back
// through a secret link, not an account. This is only for the people who run
// the instance.
package session

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"git.codigosh.com/CodigoSH/LastHelp/internal/platform/database"
)

// ErrInvalid is returned for a token that is unknown, expired or malformed. One
// error for all three: telling them apart tells whoever is guessing which of
// their guesses was closer.
var ErrInvalid = errors.New("invalid session")

// DefaultTTL is how long a session lasts without being seen.
const DefaultTTL = 30 * 24 * time.Hour

// Store keeps sessions. Like every store here it takes an Execer, not a handle.
type Store struct {
	DB database.Execer
}

// Create returns the token to hand the browser. Only its hash is stored: a
// leaked database backup must not be a drawer full of live sessions, which is
// the same reason the follow links on items are stored hashed.
func (s Store) Create(userID int64, now time.Time, ttl time.Duration) (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("generating a session token: %w", err)
	}
	token := base64.RawURLEncoding.EncodeToString(raw)

	if ttl <= 0 {
		ttl = DefaultTTL
	}
	_, err := s.DB.Exec(`INSERT INTO session (token_hash, user_id, created_at, expires_at, last_seen)
		VALUES (?, ?, ?, ?, ?)`,
		hashToken(token), userID, now.Unix(), now.Add(ttl).Unix(), now.Unix())
	if err != nil {
		return "", fmt.Errorf("storing the session: %w", err)
	}
	return token, nil
}

// Resolve turns a token into the account it belongs to.
//
// It also slides the expiry forward, so somebody using the panel every day is
// not signed out on the thirtieth. The write is skipped when nothing would
// change by more than an hour, because a session lookup happens on every
// request and this is the one that would otherwise be a write on every one.
func (s Store) Resolve(token string, now time.Time, ttl time.Duration) (int64, error) {
	if token == "" {
		return 0, ErrInvalid
	}
	if ttl <= 0 {
		ttl = DefaultTTL
	}
	hash := hashToken(token)

	var (
		userID    int64
		expiresAt int64
		lastSeen  int64
	)
	err := s.DB.QueryRow(`SELECT user_id, expires_at, last_seen FROM session WHERE token_hash = ?`, hash).
		Scan(&userID, &expiresAt, &lastSeen)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return 0, ErrInvalid
	case err != nil:
		// Not ErrInvalid: a broken query is not an expired session, and
		// reporting it as one is how a database problem spends a week being
		// investigated as "people keep getting signed out".
		return 0, fmt.Errorf("looking up the session: %w", err)
	}

	if now.Unix() >= expiresAt {
		// Clean up on the way past rather than leaving it for the sweeper: the
		// row is already in hand.
		_, _ = s.DB.Exec(`DELETE FROM session WHERE token_hash = ?`, hash)
		return 0, ErrInvalid
	}

	if now.Unix()-lastSeen > int64(time.Hour.Seconds()) {
		_, _ = s.DB.Exec(`UPDATE session SET last_seen = ?, expires_at = ? WHERE token_hash = ?`,
			now.Unix(), now.Add(ttl).Unix(), hash)
	}
	return userID, nil
}

// Delete ends one session. Signing out has to actually end it, not just drop
// the cookie: a token the browser forgot is still a token somebody could have.
func (s Store) Delete(token string) error {
	if token == "" {
		return nil
	}
	if _, err := s.DB.Exec(`DELETE FROM session WHERE token_hash = ?`, hashToken(token)); err != nil {
		return fmt.Errorf("deleting the session: %w", err)
	}
	return nil
}

// DeleteAllFor ends every session of one account, which is what disabling
// somebody has to do to mean anything.
func (s Store) DeleteAllFor(userID int64) error {
	if _, err := s.DB.Exec(`DELETE FROM session WHERE user_id = ?`, userID); err != nil {
		return fmt.Errorf("deleting sessions: %w", err)
	}
	return nil
}

// Purge removes what has expired.
func (s Store) Purge(now time.Time) error {
	if _, err := s.DB.Exec(`DELETE FROM session WHERE expires_at < ?`, now.Unix()); err != nil {
		return fmt.Errorf("purging sessions: %w", err)
	}
	return nil
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
