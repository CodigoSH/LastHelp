// Package settings is the server's own key-value store: the handful of things
// that belong to the instance rather than to an organisation.
package settings

import (
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"

	"git.codigosh.com/CodigoSH/LastHelp/internal/platform/database"
)

// VoterSigningKey signs the cookie that identifies a voter.
//
// It is generated once, on first use, and kept. Regenerating it would silently
// invalidate every voter identity at once: everybody could vote again on
// everything, and nothing would look broken.
const VoterSigningKey = "voter_signing_key"

// FormSigningKey signs the token a report form carries.
//
// Deliberately a different secret from the voter key. Two things that are not
// the same secret should not be the same secret: leaking one must not hand over
// the other, and they have very different lifetimes.
const FormSigningKey = "form_signing_key"

// InstanceName is what this installation calls itself: the title in the tab,
// the name in the header, the word a reporter sees before anything else.
//
// Empty means "LastHelp", which is what an operator who never set it expects to
// see rather than a blank where a name should be.
const InstanceName = "instance_name"

// InstanceLogo is a small image shown beside the name, as a data URL.
//
// In the settings row rather than as a file: a backup that is one file only
// stays one file if nothing lives outside it.
const InstanceLogo = "instance_logo"

// InstanceAccent overrides the house orange for this installation.
//
// The default is not stored. An operator who never chose one gets whatever the
// stylesheet says, which is how a new accent can ever reach them — a value
// written at install time would freeze the first one they ever saw.
const InstanceAccent = "instance_accent"

// UpdateChannel is "beta" when this installation wants prereleases.
//
// Stored rather than passed on every request: it is a decision about this
// server, and a checkbox that forgets itself between visits is one nobody
// trusts. Anything other than "beta" means stable, so a value nobody
// understands fails towards the safer channel.
// LandingLayout is what the public front page is made of, as JSON.
//
// A setting rather than a table of its own: it is one document belonging to the
// installation, the same as its name and its mark, and a table with one row in
// it is a table somebody has to remember exists.
const LandingLayout = "landing_layout"

// LandingDraft is the front page being edited and not yet published.
//
// Visitors are shown LandingLayout and nothing else, so an operator can move a
// band, change their mind and move it back without anybody seeing the page half
// arranged. Empty means there is no draft: the editor opens on what is
// published.
const LandingDraft = "landing_draft"

const UpdateChannel = "update_channel"

type Store struct {
	DB database.Execer
}

func (s Store) Get(key string) (string, error) {
	var value string
	err := s.DB.QueryRow(`SELECT value FROM settings WHERE key = ?`, key).Scan(&value)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return "", nil
	case err != nil:
		return "", fmt.Errorf("reading setting %s: %w", key, err)
	}
	return value, nil
}

func (s Store) Set(key, value string) error {
	_, err := s.DB.Exec(`INSERT INTO settings (key, value) VALUES (?, ?)
		ON CONFLICT (key) DO UPDATE SET value = excluded.value`, key, value)
	if err != nil {
		return fmt.Errorf("writing setting %s: %w", key, err)
	}
	return nil
}

// Secret returns a stored random key, generating it the first time.
//
// The write is ON CONFLICT DO NOTHING and then re-read, so two servers starting
// at once end up agreeing rather than each keeping the key it generated.
func (s Store) Secret(key string) ([]byte, error) {
	existing, err := s.Get(key)
	if err != nil {
		return nil, err
	}
	if existing != "" {
		return base64.RawStdEncoding.DecodeString(existing)
	}

	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return nil, fmt.Errorf("generating %s: %w", key, err)
	}
	encoded := base64.RawStdEncoding.EncodeToString(raw)

	if _, err := s.DB.Exec(`INSERT INTO settings (key, value) VALUES (?, ?)
		ON CONFLICT (key) DO NOTHING`, key, encoded); err != nil {
		return nil, fmt.Errorf("storing %s: %w", key, err)
	}

	stored, err := s.Get(key)
	if err != nil {
		return nil, err
	}
	return base64.RawStdEncoding.DecodeString(stored)
}
