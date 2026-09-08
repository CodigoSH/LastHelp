// Package org holds the two levels of the structure: an organisation, which is
// a connection to a forge, and the repositories under it that accept feedback.
//
// Both levels exist because the forge imposes them. A GitHub fine-grained token
// is bound to a single owner and cannot span two organisations; GitLab issues
// group tokens per group. Two organisations are two connections whether or not
// anybody wanted a second level.
package org

import (
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"git.codigosh.com/CodigoSH/LastHelp/internal/platform/database"
)

var (
	// ErrNotFound is distinct from a database failure, because masking one as
	// the other is how a broken query spends a week being investigated as a
	// missing row.
	ErrNotFound = errors.New("not found")

	// ErrSlugTaken means the address is already in use at that level.
	ErrSlugTaken = errors.New("that address is already taken")

	// ErrInvalidSlug means it would not survive being in a URL.
	ErrInvalidSlug = errors.New("an address may only contain lowercase letters, digits and dashes")

	// ErrUnknownForge means the forge is not one this binary can speak to.
	ErrUnknownForge = errors.New("unknown forge")

	// ErrNothingAccepted is a repository that takes neither bugs nor ideas,
	// which is a page that refuses everything it offers.
	ErrNothingAccepted = errors.New("a repository that accepts neither bugs nor ideas accepts nothing")
)

// Forge is which kind of forge an organisation connects to.
type Forge string

const (
	ForgeNone    Forge = "none"
	ForgeGitHub  Forge = "github"
	ForgeGitLab  Forge = "gitlab"
	ForgeGitea   Forge = "gitea"
	ForgeForgejo Forge = "forgejo"
)

// ForgeNone is not a placeholder for "not configured yet". It is a supported
// way to run: everything except turning an item into an issue works without a
// forge, and the screens say there is none rather than showing buttons that
// fail.
func (f Forge) valid() bool {
	switch f {
	case ForgeNone, ForgeGitHub, ForgeGitLab, ForgeGitea, ForgeForgejo:
		return true
	}
	return false
}

// ModerationMode is how much of what arrives waits for a human.
type ModerationMode string

const (
	// ModerateAll holds everything. The default, and correct on day one.
	ModerateAll ModerationMode = "all"

	// ModerateFirst holds only the first thing from a visitor nobody has
	// approved before. It exists because a queue that holds everything is right
	// on day one and a daily chore the day a project gets popular — which is
	// the real cost of not asking for accounts.
	ModerateFirst ModerationMode = "first"
)

func (m ModerationMode) valid() bool {
	return m == ModerateAll || m == ModerateFirst
}

// Organization is a forge connection.
//
// The token and the webhook secret are deliberately not fields here. Nothing
// that loads an organisation for a page should be carrying a credential around,
// and the only code that needs them asks for them explicitly.
type Organization struct {
	ID         int64
	Slug       string
	Name       string
	Forge      Forge
	BaseURL    string
	ForgeOwner string
	CreatedAt  time.Time
}

// Repository is what the public sees and what a collaborator triages.
type Repository struct {
	ID             int64
	OrgID          int64
	Slug           string
	Name           string
	ForgeRepo      string
	AcceptsBugs    bool
	AcceptsIdeas   bool
	ModerationMode ModerationMode

	// NotifyConfig is the channel this board's team reads, stored as JSON. It
	// is opaque here on purpose: the domain has no business knowing what a
	// Gotify token is.
	NotifyConfig string

	CreatedAt time.Time
}

// Store reads and writes both levels.
type Store struct {
	DB database.Execer
}

// slugPattern is what may appear in a URL. Deliberately narrow: an address is
// typed, pasted into chat, and printed in documentation.
var slugPattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

const maxSlugLength = 64

// NormaliseSlug lowercases and trims. It does not invent a slug from a name:
// guessing produces addresses nobody expected, and the address is the one thing
// that cannot be changed later without breaking every link to it.
func NormaliseSlug(slug string) string {
	return strings.ToLower(strings.TrimSpace(slug))
}

func checkSlug(slug string) error {
	if len(slug) == 0 || len(slug) > maxSlugLength || !slugPattern.MatchString(slug) {
		return ErrInvalidSlug
	}
	return nil
}

// CreateOrganization adds a connection.
func (s Store) CreateOrganization(o Organization, now time.Time) (Organization, error) {
	o.Slug = NormaliseSlug(o.Slug)
	if err := checkSlug(o.Slug); err != nil {
		return Organization{}, err
	}
	if o.Forge == "" {
		o.Forge = ForgeNone
	}
	if !o.Forge.valid() {
		return Organization{}, fmt.Errorf("%w: %q", ErrUnknownForge, o.Forge)
	}
	if strings.TrimSpace(o.Name) == "" {
		o.Name = o.Slug
	}

	res, err := s.DB.Exec(`INSERT INTO organization
		(slug, name, forge_kind, base_url, forge_owner, created_at)
		VALUES (?, ?, ?, ?, ?, ?)`,
		o.Slug, o.Name, string(o.Forge), o.BaseURL, o.ForgeOwner, now.Unix())
	if err != nil {
		if isUnique(err) {
			return Organization{}, ErrSlugTaken
		}
		return Organization{}, fmt.Errorf("creating the organisation: %w", err)
	}
	if o.ID, err = res.LastInsertId(); err != nil {
		return Organization{}, fmt.Errorf("creating the organisation: %w", err)
	}
	o.CreatedAt = now
	return o, nil
}

// OrganizationBySlug is the lookup every public URL starts with.
func (s Store) OrganizationBySlug(slug string) (Organization, error) {
	return s.scanOrganization(s.DB.QueryRow(`
		SELECT id, slug, name, forge_kind, base_url, forge_owner, created_at
		FROM organization WHERE slug = ?`, NormaliseSlug(slug)))
}

func (s Store) OrganizationByID(id int64) (Organization, error) {
	return s.scanOrganization(s.DB.QueryRow(`
		SELECT id, slug, name, forge_kind, base_url, forge_owner, created_at
		FROM organization WHERE id = ?`, id))
}

// ListOrganizations returns them in a stable order, because a list that
// reshuffles between two loads is a list nobody trusts.
func (s Store) ListOrganizations() ([]Organization, error) {
	rows, err := s.DB.Query(`
		SELECT id, slug, name, forge_kind, base_url, forge_owner, created_at
		FROM organization ORDER BY name COLLATE NOCASE, id`)
	if err != nil {
		return nil, fmt.Errorf("listing organisations: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []Organization
	for rows.Next() {
		var o Organization
		var forge string
		var created int64
		if err := rows.Scan(&o.ID, &o.Slug, &o.Name, &forge, &o.BaseURL, &o.ForgeOwner, &created); err != nil {
			return nil, fmt.Errorf("listing organisations: %w", err)
		}
		o.Forge = Forge(forge)
		o.CreatedAt = time.Unix(created, 0)
		out = append(out, o)
	}
	return out, rows.Err()
}

// CreateRepository registers one repository of an organisation for feedback.
//
// Only the ones the operator wants reports for: a project spread over twenty
// repositories registers the one it wants them in, rather than making a
// reporter guess which of twenty their problem belongs to.
func (s Store) CreateRepository(r Repository, now time.Time) (Repository, error) {
	r.Slug = NormaliseSlug(r.Slug)
	if err := checkSlug(r.Slug); err != nil {
		return Repository{}, err
	}
	if strings.TrimSpace(r.Name) == "" {
		r.Name = r.Slug
	}
	if r.ModerationMode == "" {
		r.ModerationMode = ModerateAll
	}
	if !r.ModerationMode.valid() {
		return Repository{}, fmt.Errorf("unknown moderation mode %q", r.ModerationMode)
	}
	if !r.AcceptsBugs && !r.AcceptsIdeas {
		return Repository{}, ErrNothingAccepted
	}

	res, err := s.DB.Exec(`INSERT INTO repository
		(org_id, slug, name, forge_repo, accepts_bugs, accepts_ideas, moderation_mode, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		r.OrgID, r.Slug, r.Name, r.ForgeRepo, r.AcceptsBugs, r.AcceptsIdeas,
		string(r.ModerationMode), now.Unix())
	if err != nil {
		if isUnique(err) {
			return Repository{}, ErrSlugTaken
		}
		if isForeignKey(err) {
			return Repository{}, ErrNotFound
		}
		return Repository{}, fmt.Errorf("creating the repository: %w", err)
	}
	if r.ID, err = res.LastInsertId(); err != nil {
		return Repository{}, fmt.Errorf("creating the repository: %w", err)
	}
	r.CreatedAt = now
	return r, nil
}

// RepositoryBySlugs resolves the two halves of a public address in one query,
// which is also what gives the caller the organisation id that authz needs.
func (s Store) RepositoryBySlugs(orgSlug, repoSlug string) (Repository, error) {
	return s.scanRepository(s.DB.QueryRow(`
		SELECT r.id, r.org_id, r.slug, r.name, r.forge_repo,
		       r.accepts_bugs, r.accepts_ideas, r.moderation_mode, r.notify_config, r.created_at
		FROM repository r
		JOIN organization o ON o.id = r.org_id
		WHERE o.slug = ? AND r.slug = ?`, NormaliseSlug(orgSlug), NormaliseSlug(repoSlug)))
}

func (s Store) RepositoryByID(id int64) (Repository, error) {
	return s.scanRepository(s.DB.QueryRow(`
		SELECT id, org_id, slug, name, forge_repo,
		       accepts_bugs, accepts_ideas, moderation_mode, notify_config, created_at
		FROM repository WHERE id = ?`, id))
}

func (s Store) ListRepositories(orgID int64) ([]Repository, error) {
	rows, err := s.DB.Query(`
		SELECT id, org_id, slug, name, forge_repo,
		       accepts_bugs, accepts_ideas, moderation_mode, notify_config, created_at
		FROM repository WHERE org_id = ? ORDER BY name COLLATE NOCASE, id`, orgID)
	if err != nil {
		return nil, fmt.Errorf("listing repositories: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []Repository
	for rows.Next() {
		r, err := scanRepositoryRow(rows)
		if err != nil {
			return nil, fmt.Errorf("listing repositories: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

type scanner interface{ Scan(dest ...any) error }

func (s Store) scanOrganization(row scanner) (Organization, error) {
	var o Organization
	var forge string
	var created int64
	err := row.Scan(&o.ID, &o.Slug, &o.Name, &forge, &o.BaseURL, &o.ForgeOwner, &created)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return Organization{}, ErrNotFound
	case err != nil:
		return Organization{}, fmt.Errorf("reading the organisation: %w", err)
	}
	o.Forge = Forge(forge)
	o.CreatedAt = time.Unix(created, 0)
	return o, nil
}

func (s Store) scanRepository(row scanner) (Repository, error) {
	r, err := scanRepositoryRow(row)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return Repository{}, ErrNotFound
	case err != nil:
		return Repository{}, fmt.Errorf("reading the repository: %w", err)
	}
	return r, nil
}

func scanRepositoryRow(row scanner) (Repository, error) {
	var r Repository
	var mode string
	var created int64
	err := row.Scan(&r.ID, &r.OrgID, &r.Slug, &r.Name, &r.ForgeRepo,
		&r.AcceptsBugs, &r.AcceptsIdeas, &mode, &r.NotifyConfig, &created)
	if err != nil {
		return Repository{}, err
	}
	r.ModerationMode = ModerationMode(mode)
	r.CreatedAt = time.Unix(created, 0)
	return r, nil
}

func isUnique(err error) bool {
	return err != nil && strings.Contains(err.Error(), "UNIQUE constraint failed")
}

func isForeignKey(err error) bool {
	return err != nil && strings.Contains(err.Error(), "FOREIGN KEY constraint failed")
}

// SetNotifyConfig stores the channel a board's team reads.
//
// Stored as opaque JSON: this package does not know what a Gotify token is, and
// the less of the notification setup that reaches the domain the fewer places
// there are to leak one.
func (s Store) SetNotifyConfig(repoID int64, config string) error {
	res, err := s.DB.Exec(`UPDATE repository SET notify_config = ? WHERE id = ?`, config, repoID)
	if err != nil {
		return fmt.Errorf("storing the notification channel: %w", err)
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		return ErrNotFound
	}
	return nil
}

// RenameOrganization changes what an organisation is called, and nothing else.
//
// The slug is not among the things that change. It is in every URL anybody has
// bookmarked, in every webhook a forge was told to deliver to, and in the links
// inside issues already opened — renaming it breaks all three silently, and the
// gain is a tidier address.
func (s Store) RenameOrganization(id int64, name string) error {
	_, err := s.DB.Exec(`UPDATE organization SET name = ? WHERE id = ?`, strings.TrimSpace(name), id)
	if err != nil {
		return fmt.Errorf("renaming the organisation: %w", err)
	}
	return nil
}

// DeleteOrganization removes an organisation and everything under it.
//
// Everything, and that is the point of saying so: the repositories, the reports
// people wrote in them, the votes, the replies. The foreign keys cascade, so
// this is one statement rather than five that could half-succeed — but a screen
// offering it has to say what it means, because nothing here is recoverable
// without a backup.
func (s Store) DeleteOrganization(id int64) error {
	if _, err := s.DB.Exec(`DELETE FROM organization WHERE id = ?`, id); err != nil {
		return fmt.Errorf("deleting the organisation: %w", err)
	}
	return nil
}

// UpdateRepository changes what can be changed about one.
func (s Store) UpdateRepository(id int64, name, forgeRepo string, bugs, ideas bool) error {
	if !bugs && !ideas {
		return ErrNothingAccepted
	}
	_, err := s.DB.Exec(`UPDATE repository
		SET name = ?, forge_repo = ?, accepts_bugs = ?, accepts_ideas = ?
		WHERE id = ?`, strings.TrimSpace(name), strings.TrimSpace(forgeRepo), bugs, ideas, id)
	if err != nil {
		return fmt.Errorf("updating the repository: %w", err)
	}
	return nil
}

// DeleteRepository removes a repository and every report in it.
func (s Store) DeleteRepository(id int64) error {
	if _, err := s.DB.Exec(`DELETE FROM repository WHERE id = ?`, id); err != nil {
		return fmt.Errorf("deleting the repository: %w", err)
	}
	return nil
}
