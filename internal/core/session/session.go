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
	"unicode/utf8"

	"git.codigosh.com/CodigoSH/LastHelp/internal/platform/database"
)

// ErrInvalid is returned for a token that is unknown, expired or malformed. One
// error for all three: telling them apart tells whoever is guessing which of
// their guesses was closer.
var ErrInvalid = errors.New("invalid session")

// DefaultTTL is how long a session lasts without being seen.
const DefaultTTL = 30 * 24 * time.Hour

// ErrNotFound is returned when a session to close is not one of the account's:
// another account's session and one that does not exist answer the same.
var ErrNotFound = errors.New("session not found")

// Device is what a sign-in is shown by in the account's list of sessions: the
// browser that holds it and the address it was last seen from. Neither proves
// anything, both are what the browser and the network said, and that is all
// the list claims.
type Device struct {
	UserAgent string
	IP        string
}

// touchEvery is how often a session in use writes down that it was seen.
const touchEvery = time.Minute

// maxUserAgent caps what is kept of a browser's description. A real one is a
// few hundred characters; the header is the caller's to fill with anything.
const maxUserAgent = 512

func (d Device) clean() Device {
	ua := d.UserAgent
	if utf8.RuneCountInString(ua) > maxUserAgent {
		ua = string([]rune(ua)[:maxUserAgent])
	}
	return Device{UserAgent: ua, IP: d.IP}
}

// Found is the session a token resolved to: whose it is, and which of their
// sessions, so the list can say "this device".
type Found struct {
	UserID int64
	ID     string
}

// Listed is one open session of an account, as its owner sees it.
type Listed struct {
	ID        string `json:"id"`
	CreatedAt int64  `json:"created_at"`
	LastSeen  int64  `json:"last_seen"`
	UserAgent string `json:"user_agent"`
	IP        string `json:"ip"`
}

// idLength is how much of the stored hash names a session in the list. It is a
// hash of the token, not the token, so showing it to its owner gives nothing
// away, and it does not change when the database is vacuumed or restored,
// which a rowid would. Sixty-four bits among one account's sessions do not
// collide.
const idLength = 16

func idOf(hash string) string { return hash[:idLength] }

// Store keeps sessions. Like every store here it takes an Execer, not a handle.
type Store struct {
	DB database.Execer
}

// Create returns the token to hand the browser. Only its hash is stored: a
// leaked database backup must not be a drawer full of live sessions, which is
// the same reason the follow links on items are stored hashed.
func (s Store) Create(userID int64, now time.Time, ttl time.Duration, on Device) (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("generating a session token: %w", err)
	}
	token := base64.RawURLEncoding.EncodeToString(raw)

	if ttl <= 0 {
		ttl = DefaultTTL
	}
	on = on.clean()
	_, err := s.DB.Exec(`INSERT INTO session (token_hash, user_id, created_at, expires_at, last_seen, user_agent, ip)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		hashToken(token), userID, now.Unix(), now.Add(ttl).Unix(), now.Unix(), on.UserAgent, on.IP)
	if err != nil {
		return "", fmt.Errorf("storing the session: %w", err)
	}
	return token, nil
}

// Resolve turns a token into the account it belongs to.
//
// It also slides the expiry forward, so somebody using the panel every day is
// not signed out on the thirtieth, and notes the browser and address it came
// from for the list of sessions. The write is skipped within a minute of the
// last one, because a session lookup happens on every request and this is the
// one that would otherwise be a write on every one; a minute is still fine
// enough for the list to say which sessions are in use right now.
// A session that does not know its browser yet — one opened before the list
// existed — is written on its next use, whatever the time.
func (s Store) Resolve(token string, now time.Time, ttl time.Duration, from Device) (Found, error) {
	if token == "" {
		return Found{}, ErrInvalid
	}
	if ttl <= 0 {
		ttl = DefaultTTL
	}
	hash := hashToken(token)

	var (
		userID    int64
		expiresAt int64
		lastSeen  int64
		agent     string
	)
	err := s.DB.QueryRow(`SELECT user_id, expires_at, last_seen, user_agent FROM session WHERE token_hash = ?`, hash).
		Scan(&userID, &expiresAt, &lastSeen, &agent)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return Found{}, ErrInvalid
	case err != nil:
		// Not ErrInvalid: a broken query is not an expired session, and
		// reporting it as one is how a database problem spends a week being
		// investigated as "people keep getting signed out".
		return Found{}, fmt.Errorf("looking up the session: %w", err)
	}

	if now.Unix() >= expiresAt {
		// Clean up on the way past rather than leaving it for the sweeper: the
		// row is already in hand.
		_, _ = s.DB.Exec(`DELETE FROM session WHERE token_hash = ?`, hash)
		return Found{}, ErrInvalid
	}

	if now.Unix()-lastSeen > int64(touchEvery.Seconds()) || (agent == "" && from.UserAgent != "") {
		from = from.clean()
		_, _ = s.DB.Exec(`UPDATE session SET last_seen = ?, expires_at = ?, user_agent = ?, ip = ? WHERE token_hash = ?`,
			now.Unix(), now.Add(ttl).Unix(), from.UserAgent, from.IP, hash)
	}
	return Found{UserID: userID, ID: idOf(hash)}, nil
}

// List returns the sessions of one account that have not expired, the most
// recently seen first.
func (s Store) List(userID int64, now time.Time) ([]Listed, error) {
	rows, err := s.DB.Query(`SELECT token_hash, created_at, last_seen, user_agent, ip FROM session
		WHERE user_id = ? AND expires_at > ? ORDER BY last_seen DESC, created_at DESC`, userID, now.Unix())
	if err != nil {
		return nil, fmt.Errorf("listing sessions: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := []Listed{}
	for rows.Next() {
		var (
			hash string
			l    Listed
		)
		if err := rows.Scan(&hash, &l.CreatedAt, &l.LastSeen, &l.UserAgent, &l.IP); err != nil {
			return nil, fmt.Errorf("reading a session: %w", err)
		}
		l.ID = idOf(hash)
		out = append(out, l)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("listing sessions: %w", err)
	}
	return out, nil
}

// Close ends one session of one account. The account is part of the match, so
// a session of somebody else is not found rather than closed.
func (s Store) Close(userID int64, id string) error {
	if !validID(id) {
		return ErrNotFound
	}
	res, err := s.DB.Exec(`DELETE FROM session WHERE user_id = ? AND substr(token_hash, 1, ?) = ?`, userID, idLength, id)
	if err != nil {
		return fmt.Errorf("closing the session: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// CloseOthers ends every session of one account except the one named, which
// is the one asking. It says how many it ended.
func (s Store) CloseOthers(userID int64, keep string) (int64, error) {
	// Without a session to keep this would be "close all of them", which is
	// a different button.
	if !validID(keep) {
		return 0, ErrNotFound
	}
	res, err := s.DB.Exec(`DELETE FROM session WHERE user_id = ? AND substr(token_hash, 1, ?) <> ?`, userID, idLength, keep)
	if err != nil {
		return 0, fmt.Errorf("closing the other sessions: %w", err)
	}
	n, _ := res.RowsAffected()
	return n, nil
}

// validID accepts what idOf makes and nothing else: lower-case hex of the
// right length.
func validID(id string) bool {
	if len(id) != idLength {
		return false
	}
	for _, c := range id {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
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
