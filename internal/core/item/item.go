// Package item is what the public writes: a bug or an idea, its votes and its
// comments, and the moderation queue they arrive in.
//
// A bug and an idea share this type because they share almost everything —
// moderation, comments, the link to an issue, search. They differ in two
// things, and both are enforced here rather than by splitting the table:
//
//   - Only an idea is voted. Two hundred people agreeing that something crashes
//     does not make it crash harder; what a bug needs is somebody able to
//     reproduce it.
//   - Only a bug carries form fields. A report without a version and steps is
//     work for the person receiving it, and the whole point of routing bugs
//     straight to the forge is that they arrive usable.
package item

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"git.codigosh.com/CodigoSH/LastHelp/internal/platform/database"
)

var (
	ErrNotFound = errors.New("item not found")

	// ErrKindNotAccepted is what a repository that takes only bugs answers to
	// an idea, and the other way round.
	ErrKindNotAccepted = errors.New("this repository does not accept that")

	// ErrNotVotable is returned when somebody tries to vote a bug.
	ErrNotVotable = errors.New("only ideas are voted")

	// ErrAlreadyVoted is not an error the page shows as a failure: it means the
	// vote is already there, which is what the person wanted.
	ErrAlreadyVoted = errors.New("already voted")
)

// Kind is which of the two journeys this is.
type Kind string

const (
	Bug  Kind = "bug"
	Idea Kind = "idea"
)

func (k Kind) valid() bool { return k == Bug || k == Idea }

// Status is where something has got to. The sets are short, fixed, and
// different per kind: a bug is never "planned".
type Status string

const (
	// Shared by both.
	StatusNew Status = "new"

	// Ideas.
	StatusPlanned    Status = "planned"
	StatusInProgress Status = "in_progress"
	StatusDone       Status = "done"
	StatusDeclined   Status = "declined"

	// Bugs.
	StatusConfirmed       Status = "confirmed"
	StatusInForge         Status = "in_forge"
	StatusClosed          Status = "closed"
	StatusNotReproducible Status = "not_reproducible"
)

// StatusesFor is the whole vocabulary a kind may use. Anything outside it is
// refused rather than stored, because a status nobody wrote a screen for is a
// row that renders as blank.
func StatusesFor(k Kind) []Status {
	switch k {
	case Idea:
		return []Status{StatusNew, StatusPlanned, StatusInProgress, StatusDone, StatusDeclined}
	case Bug:
		return []Status{StatusNew, StatusConfirmed, StatusInForge, StatusClosed, StatusNotReproducible}
	}
	return nil
}

func validStatus(k Kind, s Status) bool {
	for _, candidate := range StatusesFor(k) {
		if candidate == s {
			return true
		}
	}
	return false
}

// Moderation is whether the public can see it yet.
type Moderation string

const (
	Pending   Moderation = "pending"
	Published Moderation = "published"
	Rejected  Moderation = "rejected"
	Spam      Moderation = "spam"
)

// Field is one answer to one question of the report form, stored with the label
// it was asked under.
//
// The label is copied rather than referenced on purpose: if the form is edited
// next month, what somebody sent last month still reads the way they sent it.
type Field struct {
	Key      string
	Label    string
	Value    string
	Position int
}

// Item is a report or an idea.
type Item struct {
	ID         int64
	PublicID   string
	RepoID     int64
	Kind       Kind
	Title      string
	Body       string
	Status     Status
	Moderation Moderation
	VoteCount  int
	Fields     []Field
	MergedInto int64
	CreatedAt  time.Time
}

// New is what a reporter submits.
type New struct {
	RepoID int64
	Kind   Kind
	Title  string
	Body   string
	Email  string // optional, only if they asked to be notified
	Fields []Field
}

// Store reads and writes items.
type Store struct {
	DB database.Execer
}

const (
	maxTitle = 200
	maxBody  = 20_000
	maxValue = 4_000
)

// Create stores a submission and returns it along with the follow token, which
// is shown once and never again.
//
// Only the hash of that token is stored, the same way a session is: a leaked
// database must not be a list of links that let somebody reply as any reporter.
func (s Store) Create(in New, moderation Moderation, now time.Time) (Item, string, error) {
	if !in.Kind.valid() {
		return Item{}, "", fmt.Errorf("unknown kind %q", in.Kind)
	}
	in.Title = strings.TrimSpace(in.Title)
	in.Body = strings.TrimSpace(in.Body)
	if in.Title == "" {
		return Item{}, "", errors.New("a report needs a title")
	}
	if len(in.Title) > maxTitle {
		return Item{}, "", fmt.Errorf("the title is longer than %d characters", maxTitle)
	}
	if len(in.Body) > maxBody {
		return Item{}, "", fmt.Errorf("the description is longer than %d characters", maxBody)
	}

	publicID, err := newPublicID()
	if err != nil {
		return Item{}, "", err
	}
	token, err := newToken()
	if err != nil {
		return Item{}, "", err
	}

	res, err := s.DB.Exec(`INSERT INTO item
		(public_id, repo_id, kind, title, body, status, moderation_state,
		 body_hash, follow_token_hash, created_at, published_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		publicID, in.RepoID, string(in.Kind), in.Title, in.Body,
		string(StatusNew), string(moderation),
		BodyHash(in.Title, in.Body), hashToken(token), now.Unix(),
		publishedAt(moderation, now))
	if err != nil {
		if strings.Contains(err.Error(), "FOREIGN KEY constraint failed") {
			return Item{}, "", ErrNotFound
		}
		return Item{}, "", fmt.Errorf("storing the report: %w", err)
	}

	id, err := res.LastInsertId()
	if err != nil {
		return Item{}, "", fmt.Errorf("storing the report: %w", err)
	}

	for i, f := range in.Fields {
		if len(f.Value) > maxValue {
			f.Value = f.Value[:maxValue]
		}
		if _, err := s.DB.Exec(`INSERT INTO item_field (item_id, key, label, value, position)
			VALUES (?, ?, ?, ?, ?)`, id, f.Key, f.Label, f.Value, i); err != nil {
			return Item{}, "", fmt.Errorf("storing the answers: %w", err)
		}
	}

	return Item{
		ID: id, PublicID: publicID, RepoID: in.RepoID, Kind: in.Kind,
		Title: in.Title, Body: in.Body, Status: StatusNew, Moderation: moderation,
		Fields: in.Fields, CreatedAt: now,
	}, token, nil
}

func publishedAt(m Moderation, now time.Time) any {
	if m == Published {
		return now.Unix()
	}
	return nil
}

// ByPublicID is the lookup a public page does.
func (s Store) ByPublicID(publicID string) (Item, error) {
	it, err := s.scan(s.DB.QueryRow(selectItem+` WHERE public_id = ?`, publicID))
	if err != nil {
		return Item{}, err
	}
	return s.withFields(it)
}

func (s Store) ByID(id int64) (Item, error) {
	it, err := s.scan(s.DB.QueryRow(selectItem+` WHERE id = ?`, id))
	if err != nil {
		return Item{}, err
	}
	return s.withFields(it)
}

// CheckFollowToken says whether this secret link belongs to this item.
//
// Constant-time, and the comparison is on the hash rather than the token, so
// what is compared is what is stored.
func (s Store) CheckFollowToken(publicID, token string) (Item, error) {
	var stored string
	it, err := s.scanWith(s.DB.QueryRow(
		`SELECT `+itemColumns+`, follow_token_hash FROM item WHERE public_id = ?`, publicID), &stored)
	if err != nil {
		return Item{}, err
	}
	if !sameToken(stored, hashToken(token)) {
		return Item{}, ErrNotFound
	}
	return s.withFields(it)
}

// Vote records that somebody wants an idea.
//
// The voter is a random id from a signed cookie, never an address: behind a
// household router or a mobile carrier the address is shared, and keying votes
// on it would let one neighbour spend everybody else's vote. Clearing the
// cookie allows a second vote, which is a known and documented limit rather
// than a defect: the alternative is asking for an account, which is the thing
// this product exists not to do.
func (s Store) Vote(itemID int64, voterID string, now time.Time) error {
	if voterID == "" {
		return errors.New("a vote needs a voter")
	}

	var kind string
	err := s.DB.QueryRow(`SELECT kind FROM item WHERE id = ?`, itemID).Scan(&kind)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return ErrNotFound
	case err != nil:
		return fmt.Errorf("looking up the item: %w", err)
	}
	if Kind(kind) != Idea {
		return ErrNotVotable
	}

	res, err := s.DB.Exec(`INSERT INTO vote (item_id, voter_id, created_at)
		VALUES (?, ?, ?) ON CONFLICT (item_id, voter_id) DO NOTHING`,
		itemID, voterID, now.Unix())
	if err != nil {
		return fmt.Errorf("recording the vote: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("recording the vote: %w", err)
	}
	if affected == 0 {
		return ErrAlreadyVoted
	}

	// The count is kept on the row because every list orders by it, and
	// counting votes per item on every page load is the query that stops
	// scaling first.
	if _, err := s.DB.Exec(`UPDATE item SET vote_count = vote_count + 1 WHERE id = ?`, itemID); err != nil {
		return fmt.Errorf("updating the count: %w", err)
	}
	return nil
}

// Unvote takes a vote back.
func (s Store) Unvote(itemID int64, voterID string) error {
	res, err := s.DB.Exec(`DELETE FROM vote WHERE item_id = ? AND voter_id = ?`, itemID, voterID)
	if err != nil {
		return fmt.Errorf("removing the vote: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil || affected == 0 {
		return err
	}
	// Never below zero: a count that can go negative is a count that will,
	// after any bug anywhere near this.
	_, err = s.DB.Exec(`UPDATE item SET vote_count = MAX(vote_count - 1, 0) WHERE id = ?`, itemID)
	return err
}

// HasVoted is what the page needs to draw the button in the right state.
func (s Store) HasVoted(itemID int64, voterID string) (bool, error) {
	var n int
	err := s.DB.QueryRow(`SELECT count(*) FROM vote WHERE item_id = ? AND voter_id = ?`,
		itemID, voterID).Scan(&n)
	if err != nil {
		return false, fmt.Errorf("checking the vote: %w", err)
	}
	return n > 0, nil
}

// BodyHash is what makes a copy-paste flood a lookup rather than a read. It
// normalises whitespace and case first, so the same text with a different line
// wrap still collides.
func BodyHash(title, body string) string {
	normalised := strings.ToLower(strings.Join(strings.Fields(title+" "+body), " "))
	sum := sha256.Sum256([]byte(normalised))
	return hex.EncodeToString(sum[:])
}

// itemColumns is the column list, kept apart from the FROM so that a query
// needing one extra column can add it in the right place. Concatenating a
// column onto a finished SELECT is how a second table ends up in the FROM and
// the query silently becomes a cross join.
const itemColumns = `id, public_id, repo_id, kind, title, body, status,
	moderation_state, vote_count, COALESCE(merged_into, 0), created_at`

const selectItem = `SELECT ` + itemColumns + ` FROM item`

func (s Store) scan(row interface{ Scan(...any) error }) (Item, error) {
	return s.scanWith(row)
}

func (s Store) scanWith(row interface{ Scan(...any) error }, extra ...any) (Item, error) {
	var (
		it      Item
		kind    string
		status  string
		mod     string
		created int64
	)
	dest := []any{&it.ID, &it.PublicID, &it.RepoID, &kind, &it.Title, &it.Body,
		&status, &mod, &it.VoteCount, &it.MergedInto, &created}
	dest = append(dest, extra...)

	err := row.Scan(dest...)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return Item{}, ErrNotFound
	case err != nil:
		return Item{}, fmt.Errorf("reading the item: %w", err)
	}
	it.Kind = Kind(kind)
	it.Status = Status(status)
	it.Moderation = Moderation(mod)
	it.CreatedAt = time.Unix(created, 0)
	return it, nil
}

func (s Store) withFields(it Item) (Item, error) {
	rows, err := s.DB.Query(`SELECT key, label, value, position FROM item_field
		WHERE item_id = ? ORDER BY position`, it.ID)
	if err != nil {
		return Item{}, fmt.Errorf("reading the answers: %w", err)
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var f Field
		if err := rows.Scan(&f.Key, &f.Label, &f.Value, &f.Position); err != nil {
			return Item{}, fmt.Errorf("reading the answers: %w", err)
		}
		it.Fields = append(it.Fields, f)
	}
	return it, rows.Err()
}

func newPublicID() (string, error) {
	raw := make([]byte, 8)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("generating an identifier: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

func newToken() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("generating a follow link: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func sameToken(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	var diff byte
	for i := range a {
		diff |= a[i] ^ b[i]
	}
	return diff == 0
}
