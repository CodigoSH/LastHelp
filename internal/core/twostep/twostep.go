// Package twostep keeps an account's second step of signing in: the secret its
// authenticator app holds, the recovery codes that stand in for the phone, and
// the sign-ins that have passed the password and wait for a code.
//
// What makes a code right lives in package totp; this is what is stored, and
// the rule that each code is spent once. Steps that change more than one table
// (turning it on, taking it off) are run by the caller inside one transaction:
// a store holds an Execer, never a database handle of its own.
package twostep

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"git.codigosh.com/CodigoSH/LastHelp/internal/core/totp"
	"git.codigosh.com/CodigoSH/LastHelp/internal/platform/database"
)

const (
	// ChallengeLifetime is how long the code may take to arrive after the password.
	ChallengeLifetime = 5 * time.Minute
	// ChallengeAttempts is how many wrong codes one sign-in takes before the
	// password has to be typed again.
	ChallengeAttempts = 5
)

var (
	// ErrRejected: neither a current code from the app nor an unspent recovery code.
	ErrRejected = errors.New("code rejected")
	// ErrAlreadyOn: the account already has a second step.
	ErrAlreadyOn = errors.New("second step already on")
	// ErrNotSetUp: there is no secret waiting to be confirmed, or it changed meanwhile.
	ErrNotSetUp = errors.New("second step not set up")
	// ErrChallengeGone: the sign-in waiting for a code is unknown, spent, or out of time.
	ErrChallengeGone = errors.New("challenge gone")
)

// Store is the second step of every account.
type Store struct {
	DB database.Execer
}

// Status is what the account's profile shows.
type Status struct {
	Enabled      bool `json:"enabled"`
	RecoveryLeft int  `json:"recovery_left"`
}

// Enabled says whether the account signs in with a code after its password.
func (s Store) Enabled(userID int64) (bool, error) {
	var enabled int
	err := s.DB.QueryRow(`SELECT enabled FROM two_step WHERE user_id = ?`, userID).Scan(&enabled)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("reading the second step: %w", err)
	}
	return enabled == 1, nil
}

// Status says whether the account has a second step and how many recovery
// codes it has left.
func (s Store) Status(userID int64) (Status, error) {
	enabled, err := s.Enabled(userID)
	if err != nil || !enabled {
		return Status{}, err
	}
	st := Status{Enabled: true}
	if err := s.DB.QueryRow(`SELECT COUNT(*) FROM recovery_code WHERE user_id = ? AND used_at IS NULL`, userID).
		Scan(&st.RecoveryLeft); err != nil {
		return Status{}, fmt.Errorf("counting recovery codes: %w", err)
	}
	return st, nil
}

// Draft draws a new secret for the account to scan. It is not in force until a
// code from it is confirmed with Enable; drawing again replaces one that never
// was. It returns the secret as an app takes it typed in.
func (s Store) Draft(userID int64, now time.Time) (string, error) {
	enabled, err := s.Enabled(userID)
	if err != nil {
		return "", err
	}
	if enabled {
		return "", ErrAlreadyOn
	}
	secret, err := totp.NewSecret()
	if err != nil {
		return "", err
	}
	encoded := totp.EncodeSecret(secret)
	if _, err := s.DB.Exec(`INSERT INTO two_step (user_id, secret, enabled, last_step, created_at)
		VALUES (?, ?, 0, 0, ?)
		ON CONFLICT (user_id) DO UPDATE SET secret = excluded.secret, last_step = 0, created_at = excluded.created_at
		WHERE two_step.enabled = 0`, userID, encoded, now.Unix()); err != nil {
		return "", fmt.Errorf("storing the secret: %w", err)
	}
	return encoded, nil
}

// Enable puts the drafted secret in force once a code from it proves the app has
// it, and returns the recovery codes — the only time they are shown. Run it in a
// transaction: it turns the secret on and writes the codes.
func (s Store) Enable(userID int64, code string, now time.Time) ([]string, error) {
	var (
		encoded string
		enabled int
	)
	err := s.DB.QueryRow(`SELECT secret, enabled FROM two_step WHERE user_id = ?`, userID).Scan(&encoded, &enabled)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return nil, ErrNotSetUp
	case err != nil:
		return nil, fmt.Errorf("reading the secret: %w", err)
	case enabled == 1:
		return nil, ErrAlreadyOn
	}
	secret, err := totp.DecodeSecret(encoded)
	if err != nil {
		return nil, fmt.Errorf("reading the secret: %w", err)
	}
	used, ok := totp.Verify(secret, code, now, 0)
	if !ok {
		return nil, ErrRejected
	}
	// Only the secret that was scanned, and only while it is still a draft: a
	// second tab that drew another one meanwhile makes this one stale.
	res, err := s.DB.Exec(`UPDATE two_step SET enabled = 1, last_step = ? WHERE user_id = ? AND enabled = 0 AND secret = ?`,
		used, userID, encoded)
	if err != nil {
		return nil, fmt.Errorf("turning the second step on: %w", err)
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return nil, ErrNotSetUp
	}
	return s.ReplaceRecoveryCodes(userID)
}

// ReplaceRecoveryCodes draws a new set in place of every code the account had,
// and returns them. Run it in a transaction: a set half written is worse than
// the old one.
func (s Store) ReplaceRecoveryCodes(userID int64) ([]string, error) {
	codes, err := totp.NewRecoveryCodes()
	if err != nil {
		return nil, err
	}
	if _, err := s.DB.Exec(`DELETE FROM recovery_code WHERE user_id = ?`, userID); err != nil {
		return nil, fmt.Errorf("dropping the old recovery codes: %w", err)
	}
	for _, c := range codes {
		if _, err := s.DB.Exec(`INSERT INTO recovery_code (user_id, code_hash) VALUES (?, ?)`,
			userID, totp.HashRecoveryCode(c)); err != nil {
			return nil, fmt.Errorf("storing a recovery code: %w", err)
		}
	}
	return codes, nil
}

// Accept checks a code for an account and spends it: a code from the app moves
// the last accepted period forward, a recovery code is marked used. Each is one
// statement that succeeds once, so two requests carrying the same code cannot
// both sign in.
func (s Store) Accept(userID int64, code string, now time.Time) error {
	if totp.LooksLikeTOTP(code) {
		var (
			encoded  string
			lastStep int64
		)
		err := s.DB.QueryRow(`SELECT secret, last_step FROM two_step WHERE user_id = ? AND enabled = 1`, userID).
			Scan(&encoded, &lastStep)
		switch {
		case errors.Is(err, sql.ErrNoRows):
			return ErrRejected
		case err != nil:
			return fmt.Errorf("reading the secret: %w", err)
		}
		secret, err := totp.DecodeSecret(encoded)
		if err != nil {
			return fmt.Errorf("reading the secret: %w", err)
		}
		used, ok := totp.Verify(secret, code, now, lastStep)
		if !ok {
			return ErrRejected
		}
		res, err := s.DB.Exec(`UPDATE two_step SET last_step = ? WHERE user_id = ? AND last_step < ?`, used, userID, used)
		if err != nil {
			return fmt.Errorf("spending the code: %w", err)
		}
		if n, _ := res.RowsAffected(); n != 1 {
			return ErrRejected
		}
		return nil
	}
	res, err := s.DB.Exec(`UPDATE recovery_code SET used_at = ?
		WHERE user_id = ? AND code_hash = ? AND used_at IS NULL
		AND EXISTS (SELECT 1 FROM two_step WHERE user_id = ? AND enabled = 1)`,
		now.Unix(), userID, totp.HashRecoveryCode(code), userID)
	if err != nil {
		return fmt.Errorf("spending the recovery code: %w", err)
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return ErrRejected
	}
	return nil
}

// Remove takes the second step off an account: its secret, its recovery codes
// and any sign-in waiting for a code. Run it in a transaction.
func (s Store) Remove(userID int64) error {
	for _, q := range []string{
		`DELETE FROM two_step WHERE user_id = ?`,
		`DELETE FROM recovery_code WHERE user_id = ?`,
		`DELETE FROM sign_in_challenge WHERE user_id = ?`,
	} {
		if _, err := s.DB.Exec(q, userID); err != nil {
			return fmt.Errorf("removing the second step: %w", err)
		}
	}
	return nil
}

/* ---------------------------------------------------------------------------
 * The sign-in between the password and the code
 * ------------------------------------------------------------------------ */

func challengeHash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// NewChallenge records a sign-in that has passed the password and returns what
// the browser presents with the code. Only its hash is kept.
func (s Store) NewChallenge(userID int64, now time.Time) (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	if _, err := s.DB.Exec(`INSERT INTO sign_in_challenge (token_hash, user_id, expires_at) VALUES (?, ?, ?)`,
		challengeHash(token), userID, now.Add(ChallengeLifetime).Unix()); err != nil {
		return "", fmt.Errorf("storing the sign-in: %w", err)
	}
	return token, nil
}

// Challenge says whose sign-in a token is, while it is still good.
func (s Store) Challenge(token string, now time.Time) (int64, error) {
	if token == "" {
		return 0, ErrChallengeGone
	}
	var (
		userID    int64
		expiresAt int64
	)
	err := s.DB.QueryRow(`SELECT user_id, expires_at FROM sign_in_challenge WHERE token_hash = ?`, challengeHash(token)).
		Scan(&userID, &expiresAt)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return 0, ErrChallengeGone
	case err != nil:
		return 0, fmt.Errorf("reading the sign-in: %w", err)
	case now.Unix() > expiresAt:
		return 0, ErrChallengeGone
	}
	return userID, nil
}

// FailChallenge counts a wrong code against a sign-in, and ends it at the last
// one, so a guessed password buys a handful of tries and not an afternoon.
func (s Store) FailChallenge(token string) error {
	hash := challengeHash(token)
	if _, err := s.DB.Exec(`UPDATE sign_in_challenge SET attempts = attempts + 1 WHERE token_hash = ?`, hash); err != nil {
		return fmt.Errorf("counting a wrong code: %w", err)
	}
	if _, err := s.DB.Exec(`DELETE FROM sign_in_challenge WHERE token_hash = ? AND attempts >= ?`, hash, ChallengeAttempts); err != nil {
		return fmt.Errorf("ending the sign-in: %w", err)
	}
	return nil
}

// SpendChallenge ends a sign-in that got its code. It is good for one: a second
// request with the same token finds it gone.
func (s Store) SpendChallenge(token string) error {
	res, err := s.DB.Exec(`DELETE FROM sign_in_challenge WHERE token_hash = ?`, challengeHash(token))
	if err != nil {
		return fmt.Errorf("ending the sign-in: %w", err)
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return ErrChallengeGone
	}
	return nil
}

// PurgeChallenges drops the sign-ins that ran out of time without a code.
func (s Store) PurgeChallenges(now time.Time) error {
	if _, err := s.DB.Exec(`DELETE FROM sign_in_challenge WHERE expires_at < ?`, now.Unix()); err != nil {
		return fmt.Errorf("sweeping sign-ins: %w", err)
	}
	return nil
}
