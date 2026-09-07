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

// SetCredentials stores the token and webhook secret, already encrypted.
//
// The store never sees them in the clear and never decrypts: it takes sealed
// bytes and hands sealed bytes back. Which means a query, a log line or a
// backup of this table is not a leak of write access to somebody's forge.
func (s Store) SetCredentials(orgID int64, token, webhookSecret []byte) error {
	res, err := s.DB.Exec(`UPDATE organization SET token_enc = ?, webhook_secret_enc = ? WHERE id = ?`,
		token, webhookSecret, orgID)
	if err != nil {
		return fmt.Errorf("storing the credentials: %w", err)
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		return ErrNotFound
	}
	return nil
}

// Credentials returns the sealed bytes.
func (s Store) Credentials(orgID int64) (token, webhookSecret []byte, err error) {
	err = s.DB.QueryRow(`SELECT token_enc, webhook_secret_enc FROM organization WHERE id = ?`, orgID).
		Scan(&token, &webhookSecret)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return nil, nil, ErrNotFound
	case err != nil:
		return nil, nil, fmt.Errorf("reading the credentials: %w", err)
	case len(token) == 0:
		return nil, nil, ErrNoCredentials
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
