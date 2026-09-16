package org

import (
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
)

// ErrNoCredentials means this organisation has no token stored.
var ErrNoCredentials = errors.New("no forge credentials")

// SetCredentials stores the token and the webhook secret.
//
// They are stored as they were given. They used to be sealed with a key that
// lived beside the database, which against the one thing encryption at rest can
// cover — the file leaving the machine — protected nothing, because the key left
// with it. The boundary is the backup, and that is where the password is.
//
// What follows from that: this table is worth the same as the token in it. A
// query, a log line or a copy of the file is write access to somebody's forge,
// and the backup screen says so before it makes one.
func (s Store) SetCredentials(orgID int64, token, webhookSecret string) error {
	res, err := s.DB.Exec(`UPDATE organization SET token = ?, webhook_secret = ? WHERE id = ?`,
		token, webhookSecret, orgID)
	if err != nil {
		return fmt.Errorf("storing the credentials: %w", err)
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		return ErrNotFound
	}
	return nil
}

// Credentials returns the token and the webhook secret.
func (s Store) Credentials(orgID int64) (token, webhookSecret string, err error) {
	err = s.DB.QueryRow(`SELECT token, webhook_secret FROM organization WHERE id = ?`, orgID).
		Scan(&token, &webhookSecret)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return "", "", ErrNotFound
	case err != nil:
		return "", "", fmt.Errorf("reading the credentials: %w", err)
	case token == "":
		return "", "", ErrNoCredentials
	}
	return token, webhookSecret, nil
}

// HookID is the opaque address a forge delivers to, generated on first use.
//
// Opaque because it is the only thing identifying the connection, and the
// receiver is chosen from that record rather than from the request's headers:
// Forgejo sends GitHub's headers alongside its own, so a server that guessed
// from headers would let the caller pick which verification it faced — and one
// of those paths, GitLab's legacy token, is weaker than the others.
func (s Store) HookID(orgID int64) (string, error) {
	var existing string
	err := s.DB.QueryRow(`SELECT hook_id FROM organization WHERE id = ?`, orgID).Scan(&existing)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return "", ErrNotFound
	case err != nil:
		return "", fmt.Errorf("reading the webhook address: %w", err)
	case existing != "":
		return existing, nil
	}

	raw := make([]byte, 24)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("generating a webhook address: %w", err)
	}
	generated := base64.RawURLEncoding.EncodeToString(raw)

	// DO NOTHING and then re-read, so two requests racing end up agreeing
	// rather than each keeping the address it generated — one of which the
	// forge would then never use.
	if _, err := s.DB.Exec(`UPDATE organization SET hook_id = ? WHERE id = ? AND hook_id = ''`,
		generated, orgID); err != nil {
		return "", fmt.Errorf("storing the webhook address: %w", err)
	}
	return s.HookID(orgID)
}

// ByHookID resolves a delivery's address to its connection.
func (s Store) ByHookID(hookID string) (Organization, error) {
	if hookID == "" {
		return Organization{}, ErrNotFound
	}
	return s.scanOrganization(s.DB.QueryRow(`
		SELECT id, slug, name, forge_kind, base_url, forge_owner, created_at
		FROM organization WHERE hook_id = ?`, hookID))
}
