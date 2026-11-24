// Package passkey keeps the passkeys of the team's accounts: each one's public
// key — never anything secret, the private half stays on the person's device —
// the id its device knows the account by, and the challenges handed to a
// browser until they are answered.
//
// Whether what a browser sent is right is package webauthn's to say; this is
// what is stored, and the rules that a challenge is answered once and a
// signature counter never goes back.
package passkey

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"time"
	"unicode/utf8"

	"git.codigosh.com/CodigoSH/LastHelp/internal/platform/database"
)

const (
	// ChallengeLifetime is how long a browser has to answer.
	ChallengeLifetime = 5 * time.Minute
	// MaxName is how long a passkey's name may be, in characters.
	MaxName = 64

	PurposeRegister = "register"
	PurposeSignIn   = "sign-in"
)

var (
	// ErrChallengeGone: not issued here, answered already, or out of time.
	ErrChallengeGone = errors.New("challenge gone")
	// ErrNotFound: no such passkey, or one of another account.
	ErrNotFound = errors.New("passkey not found")
	// ErrAlreadyAdded: a passkey with this id is kept already.
	ErrAlreadyAdded = errors.New("passkey already added")
	// ErrRaced: the passkey was used by another request at the same moment.
	ErrRaced = errors.New("passkey used at the same moment")
)

// Store is every account's passkeys.
type Store struct {
	DB database.Execer
}

// Listed is a passkey as its owner sees it.
type Listed struct {
	ID         int64  `json:"id"`
	Name       string `json:"name"`
	CreatedAt  int64  `json:"created_at"`
	LastUsedAt *int64 `json:"last_used_at"`
}

// List is the account's passkeys, oldest first.
func (s Store) List(userID int64) ([]Listed, error) {
	rows, err := s.DB.Query(`SELECT id, name, created_at, last_used_at FROM passkey WHERE user_id = ? ORDER BY created_at, id`, userID)
	if err != nil {
		return nil, fmt.Errorf("listing passkeys: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := []Listed{}
	for rows.Next() {
		var (
			p    Listed
			used sql.NullInt64
		)
		if err := rows.Scan(&p.ID, &p.Name, &p.CreatedAt, &used); err != nil {
			return nil, fmt.Errorf("listing passkeys: %w", err)
		}
		if used.Valid {
			p.LastUsedAt = &used.Int64
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// CredentialIDs are the ids of the account's passkeys, so a device that has
// one is not asked to make a second.
func (s Store) CredentialIDs(userID int64) ([]string, error) {
	rows, err := s.DB.Query(`SELECT credential_id FROM passkey WHERE user_id = ?`, userID)
	if err != nil {
		return nil, fmt.Errorf("listing passkeys: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// Handle is the id an account's devices store for it, drawn the first time it
// is asked for.
func (s Store) Handle(userID int64) ([]byte, error) {
	var handle []byte
	err := s.DB.QueryRow(`SELECT handle FROM passkey_handle WHERE user_id = ?`, userID).Scan(&handle)
	if err == nil {
		return handle, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("reading the account's handle: %w", err)
	}
	handle = make([]byte, 16)
	if _, err := rand.Read(handle); err != nil {
		return nil, err
	}
	if _, err := s.DB.Exec(`INSERT OR IGNORE INTO passkey_handle (user_id, handle) VALUES (?, ?)`, userID, handle); err != nil {
		return nil, fmt.Errorf("storing the account's handle: %w", err)
	}
	// Read back: two requests drawing at once keep the first one written.
	if err := s.DB.QueryRow(`SELECT handle FROM passkey_handle WHERE user_id = ?`, userID).Scan(&handle); err != nil {
		return nil, fmt.Errorf("reading the account's handle: %w", err)
	}
	return handle, nil
}

func challengeHash(challenge []byte) string {
	sum := sha256.Sum256(challenge)
	return hex.EncodeToString(sum[:])
}

// NewChallenge hands out a challenge for one ceremony: to add a passkey, for
// the account signed in; to sign in, for nobody yet (userID 0).
func (s Store) NewChallenge(purpose string, userID int64, now time.Time) ([]byte, error) {
	challenge := make([]byte, 32)
	if _, err := rand.Read(challenge); err != nil {
		return nil, err
	}
	var owner any
	if userID > 0 {
		owner = userID
	}
	if _, err := s.DB.Exec(`INSERT INTO passkey_challenge (challenge_hash, purpose, user_id, expires_at) VALUES (?, ?, ?, ?)`,
		challengeHash(challenge), purpose, owner, now.Add(ChallengeLifetime).Unix()); err != nil {
		return nil, fmt.Errorf("storing the challenge: %w", err)
	}
	return challenge, nil
}

// TakeChallenge spends a challenge and says whose it was (0 for a sign-in). One
// statement, so the same answer sent twice is accepted once.
func (s Store) TakeChallenge(challenge []byte, purpose string, now time.Time) (int64, error) {
	var owner sql.NullInt64
	err := s.DB.QueryRow(`DELETE FROM passkey_challenge
		WHERE challenge_hash = ? AND purpose = ? AND expires_at >= ?
		RETURNING user_id`, challengeHash(challenge), purpose, now.Unix()).Scan(&owner)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrChallengeGone
	}
	if err != nil {
		return 0, fmt.Errorf("spending the challenge: %w", err)
	}
	return owner.Int64, nil
}

// PurgeChallenges drops the challenges nobody answered in time.
func (s Store) PurgeChallenges(now time.Time) error {
	if _, err := s.DB.Exec(`DELETE FROM passkey_challenge WHERE expires_at < ?`, now.Unix()); err != nil {
		return fmt.Errorf("sweeping passkey challenges: %w", err)
	}
	return nil
}

// New is a passkey to keep.
type New struct {
	CredentialID string
	PublicKey    []byte
	Alg          int64
	SignCount    uint32
	Name         string
}

// CleanName is a passkey's name as it is kept: cut to MaxName characters, and
// "Passkey" when empty.
func CleanName(name string) string {
	if utf8.RuneCountInString(name) > MaxName {
		name = string([]rune(name)[:MaxName])
	}
	if name == "" {
		return "Passkey"
	}
	return name
}

// Add keeps a passkey for an account and returns its id.
func (s Store) Add(userID int64, p New, now time.Time) (int64, error) {
	res, err := s.DB.Exec(`INSERT INTO passkey (user_id, credential_id, public_key, alg, sign_count, name, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?) ON CONFLICT (credential_id) DO NOTHING`,
		userID, p.CredentialID, p.PublicKey, p.Alg, p.SignCount, CleanName(p.Name), now.Unix())
	if err != nil {
		return 0, fmt.Errorf("storing the passkey: %w", err)
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return 0, ErrAlreadyAdded
	}
	return res.LastInsertId()
}

// Delete removes one of the account's passkeys. Another account's is not found.
func (s Store) Delete(userID, id int64) error {
	res, err := s.DB.Exec(`DELETE FROM passkey WHERE id = ? AND user_id = ?`, id, userID)
	if err != nil {
		return fmt.Errorf("removing the passkey: %w", err)
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return ErrNotFound
	}
	return nil
}

// Stored is a passkey as a sign-in needs it.
type Stored struct {
	ID        int64
	UserID    int64
	PublicKey []byte
	SignCount uint32
	// Handle is the id the account's devices store, or empty if none was drawn.
	Handle []byte
}

// ByCredential finds the passkey a device names.
func (s Store) ByCredential(credentialID string) (Stored, error) {
	var (
		p     Stored
		count int64
	)
	err := s.DB.QueryRow(`SELECT p.id, p.user_id, p.public_key, p.sign_count, COALESCE(h.handle, x'')
		FROM passkey p LEFT JOIN passkey_handle h ON h.user_id = p.user_id
		WHERE p.credential_id = ?`, credentialID).Scan(&p.ID, &p.UserID, &p.PublicKey, &count, &p.Handle)
	if errors.Is(err, sql.ErrNoRows) {
		return Stored{}, ErrNotFound
	}
	if err != nil {
		return Stored{}, fmt.Errorf("reading the passkey: %w", err)
	}
	p.SignCount = uint32(count)
	return p, nil
}

// Used records a sign-in: the new counter and when. It moves only from the
// counter just read, so two answers signed with the same count cannot both
// sign in.
func (s Store) Used(id int64, from, to uint32, now time.Time) error {
	res, err := s.DB.Exec(`UPDATE passkey SET sign_count = ?, last_used_at = ? WHERE id = ? AND sign_count = ?`,
		to, now.Unix(), id, from)
	if err != nil {
		return fmt.Errorf("recording the sign-in: %w", err)
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return ErrRaced
	}
	return nil
}
