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
	"unicode/utf8"

	"git.codigosh.com/CodigoSH/LastHelp/internal/platform/database"
)

var (
	ErrNotFound = errors.New("item not found")

	// ErrKindNotAccepted is what a repository that takes only bugs answers to
	// an idea, and the other way round.
	ErrKindNotAccepted = errors.New("this repository does not accept that")

	// ErrNotVotable is returned when somebody tries to vote a bug.
	ErrNotVotable = errors.New("only ideas are voted")

	// ErrVotingClosed is returned for a vote, or for taking one back, on an idea
	// the team has answered. The count is then the record of how many people
	// wanted it when it was answered, and it stops moving.
	ErrVotingClosed = errors.New("voting is closed on an answered idea")

	// ErrConversationClosed is returned for a comment or a reply on a report the
	// team has answered, once the time after the answer is over. Like the count
	// of votes, what was said by then is the record, and it stops moving.
	// Reopening the report opens it again.
	ErrConversationClosed = errors.New("the conversation is closed on an answered report")

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

// Votable says whether somebody can still vote on this: an idea the team has
// not answered. A bug is never voted, and an answered idea keeps its count.
func (i Item) Votable() bool {
	return i.Kind == Idea && !i.Status.Closes()
}

// GraceAfterClosing is how long the reporter and the team may keep writing on a
// report after it was answered. An answer is rarely the end of the exchange: the
// reporter confirms the fix or says it did not work, and the team has a last word
// to add. A visitor has no such thing to say, so for them the report closes at once.
const GraceAfterClosing = 48 * time.Hour

// TakesComments says whether somebody who is just reading can still write on
// this: anything the team has not answered. It is the visitor's answer; the
// reporter and the team have TakesCommentsFrom.
func (i Item) TakesComments() bool {
	return !i.Status.Closes()
}

// TakesCommentsFrom is the same question for one author at one moment. An
// answered report stays open to the reporter and the team for GraceAfterClosing
// after the answer, and to nobody else. One rule, asked by the store when
// something is written and by the page when it decides to show the form, so the
// two cannot disagree about whether the form should be there.
func (i Item) TakesCommentsFrom(author Author, now time.Time) bool {
	if !i.Status.Closes() {
		return true
	}
	if author == AuthorVisitor || i.ClosedAt.IsZero() {
		return false
	}
	return now.Before(i.ClosedAt.Add(GraceAfterClosing))
}

// Closes says whether a status is an answer: fixed, shipped, declined, not
// reproducible. An item there is closed — the public list files it apart from
// what is still open, and it carries the time it was closed. Everything else
// is open. The one place that decides it.
func (s Status) Closes() bool {
	switch s {
	case StatusDone, StatusDeclined, StatusClosed, StatusNotReproducible:
		return true
	}
	return false
}

// ClosedStatuses and OpenStatuses split both vocabularies by Closes, for a
// filter that asks for one side without naming every status on it.
func ClosedStatuses() []Status { return statusesWhere(true) }
func OpenStatuses() []Status   { return statusesWhere(false) }

func statusesWhere(closed bool) []Status {
	var out []Status
	seen := map[Status]bool{}
	for _, k := range []Kind{Bug, Idea} {
		for _, st := range StatusesFor(k) {
			if st.Closes() == closed && !seen[st] {
				seen[st] = true
				out = append(out, st)
			}
		}
	}
	return out
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

// FieldsFor is the whole set of questions a kind may be asked, in the order
// they are asked in.
//
// Fixed here rather than taken from whatever the browser sends. The submission
// endpoint used to build a field out of every key in the request and use the
// key as the label, which meant the public chose both — a report could arrive
// with any question it liked written on it, and go on to the forge that way.
// A map has no order either, so two identical submissions could reach the team
// with their answers the other way round.
//
// Only a bug has any. An idea is the sentence somebody wrote; asking it which
// version it happened in is a box that means nothing, and every box on a public
// form is somebody deciding not to bother.
func FieldsFor(k Kind) []Field {
	if k != Bug {
		return nil
	}
	// The labels are English and stay English: they are stored with the answer
	// and printed into the forge issue, which the team reads — not necessarily
	// in the language the reporter's browser happened to be in. The page
	// translates them for the person filling them in.
	return []Field{
		{Key: "version", Label: "Version", Position: 0},
		{Key: "steps", Label: "Steps to reproduce", Position: 1},
	}
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

	// ClosedAt is when this reached a status that ends it — done, closed,
	// declined, not reproducible — and the zero time while it is still open.
	// The column was in the table from the first migration and nothing read it.
	ClosedAt time.Time
}

// New is what a reporter submits.
type New struct {
	RepoID int64
	Kind   Kind
	Title  string
	Body   string
	Fields []Field

	// Email is the reporter's address, and only if
	// they asked to be told about changes.
	//
	// Empty is the normal case: the field is only offered when a mail relay
	// exists to use it, and it is deleted when the item closes.
	Email string
}

// Store reads and writes items.
type Store struct {
	DB database.Execer
}

// What a person may write, counted in characters. It was counted with len(),
// which is bytes: a letter in Japanese or Chinese is three, so the same limit
// let in 20,000 characters of English and about 6,600 of Japanese, and refused
// somebody for writing in their own language with no way to tell why. What the
// limits protect is the page the text is drawn on and the queue it is read in,
// and both are measured in what a reader sees.
const (
	maxTitle = 200
	maxBody  = 20_000
	maxValue = 4_000
)

// tooLong says whether text is past a limit in characters.
func tooLong(text string, limit int) bool { return utf8.RuneCountInString(text) > limit }

// clipped cuts text to a limit in characters, never through the middle of one:
// slicing bytes at a limit can leave half of a three-byte letter at the end,
// and that is not text any more.
func clipped(text string, limit int) string {
	if !tooLong(text, limit) {
		return text
	}
	return string([]rune(text)[:limit])
}

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
	if tooLong(in.Title, maxTitle) {
		return Item{}, "", fmt.Errorf("the title is longer than %d characters", maxTitle)
	}
	if tooLong(in.Body, maxBody) {
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

	// Empty rather than NULL: the column is NOT NULL DEFAULT '' like every other
	// plain column added since, and "no address" is the normal case.
	email := in.Email

	res, err := s.DB.Exec(`INSERT INTO item
		(public_id, repo_id, kind, title, body, status, moderation_state,
		 body_hash, author_email, follow_token_hash, created_at, published_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		publicID, in.RepoID, string(in.Kind), in.Title, in.Body,
		string(StatusNew), string(moderation),
		BodyHash(in.Title, in.Body), email, hashToken(token), now.Unix(),
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
		f.Value = clipped(f.Value, maxValue)
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

// Delete removes a report for good, with its votes, comments and fields — the
// schema cascades them. Reports that were merged into it keep existing and
// lose the pointer (ON DELETE SET NULL), so they come back as reports of their
// own rather than disappearing with it.
func (s Store) Delete(id int64) error {
	res, err := s.DB.Exec(`DELETE FROM item WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("deleting the report: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
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

	kind, status, err := s.standing(itemID)
	if err != nil {
		return err
	}
	if kind != Idea {
		return ErrNotVotable
	}
	if status.Closes() {
		return ErrVotingClosed
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

// standing reads what decides whether something may still be added to a report.
//
// One door for votes and comments, so a new way of adding to a report inherits
// the rule without knowing it exists. A report the team turned down takes
// nothing, and a duplicate folded into another is one of those (Merge rejects
// it): it is on no public page, so a vote or a comment given to it would be
// accepted and then lost, which reads as having worked.
func (s Store) standing(itemID int64) (Kind, Status, error) {
	var kind, status, moderation string
	err := s.DB.QueryRow(`SELECT kind, status, moderation_state
		FROM item WHERE id = ?`, itemID).Scan(&kind, &status, &moderation)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return "", "", ErrNotFound
	case err != nil:
		return "", "", fmt.Errorf("looking up the item: %w", err)
	}
	if Moderation(moderation) == Rejected || Moderation(moderation) == Spam {
		return "", "", ErrNotFound
	}
	return Kind(kind), Status(status), nil
}

// mayWriteOn is the rule a comment, a reply and a picture on one share: whether
// this author may still add to the report now.
func (s Store) mayWriteOn(itemID int64, author Author, now time.Time) error {
	_, status, err := s.standing(itemID)
	if err != nil {
		return err
	}
	if !status.Closes() {
		return nil
	}
	var closed sql.NullInt64
	if err := s.DB.QueryRow(`SELECT closed_at FROM item WHERE id = ?`, itemID).Scan(&closed); err != nil {
		return fmt.Errorf("looking up when it was answered: %w", err)
	}
	it := Item{Status: status}
	if closed.Valid {
		it.ClosedAt = time.Unix(closed.Int64, 0)
	}
	if it.TakesCommentsFrom(author, now) {
		return nil
	}
	return ErrConversationClosed
}

// Unvote takes a vote back.
func (s Store) Unvote(itemID int64, voterID string) error {
	// Taking a vote back changes the count as much as giving one does, so an
	// answered idea refuses it too.
	var status string
	switch err := s.DB.QueryRow(`SELECT status FROM item WHERE id = ?`, itemID).Scan(&status); {
	case errors.Is(err, sql.ErrNoRows):
		return ErrNotFound
	case err != nil:
		return fmt.Errorf("looking up the item: %w", err)
	}
	if Status(status).Closes() {
		return ErrVotingClosed
	}

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
	moderation_state, vote_count, COALESCE(merged_into, 0), created_at, closed_at`

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
		closed  sql.NullInt64
	)
	dest := []any{&it.ID, &it.PublicID, &it.RepoID, &kind, &it.Title, &it.Body,
		&status, &mod, &it.VoteCount, &it.MergedInto, &created, &closed}
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
	if closed.Valid {
		it.ClosedAt = time.Unix(closed.Int64, 0)
	}
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

// WithFields fills in the answers for a whole listing, in one query.
//
// List does not load them — it reads the item table and nothing else — so a
// caller that lists and then reads .Fields gets an empty slice from every row
// and no error anywhere. The triage queue did exactly that for as long as it
// took to notice: a screen quietly showing nothing is the same shape as a
// screen with nothing to show.
func (s Store) WithFields(items []Item) ([]Item, error) {
	if len(items) == 0 {
		return items, nil
	}

	placeholders := make([]string, len(items))
	args := make([]any, len(items))
	for i, it := range items {
		placeholders[i] = "?"
		args[i] = it.ID
	}

	rows, err := s.DB.Query(`SELECT item_id, key, label, value, position FROM item_field
		WHERE item_id IN (`+strings.Join(placeholders, ",")+`) ORDER BY item_id, position`, args...)
	if err != nil {
		return nil, fmt.Errorf("reading the answers: %w", err)
	}
	defer func() { _ = rows.Close() }()

	byItem := make(map[int64][]Field, len(items))
	for rows.Next() {
		var (
			id int64
			f  Field
		)
		if err := rows.Scan(&id, &f.Key, &f.Label, &f.Value, &f.Position); err != nil {
			return nil, fmt.Errorf("reading the answers: %w", err)
		}
		byItem[id] = append(byItem[id], f)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reading the answers: %w", err)
	}

	for i := range items {
		items[i].Fields = byItem[items[i].ID]
	}
	return items, nil
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

// ReporterEmail returns the reporter's address, or nothing.
//
// It is asked for by id, on its own, so that the address stays out of every
// listing and out of every response that merely happens to load an item. That
// is now the whole of what keeps it from travelling — it used to be encrypted
// as well — so the narrowness matters more than it did: nothing selects this
// column unless it is about to send a message to it.
func (s Store) ReporterEmail(itemID int64) (string, error) {
	var address string
	err := s.DB.QueryRow(`SELECT author_email FROM item WHERE id = ?`, itemID).Scan(&address)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return "", ErrNotFound
	case err != nil:
		return "", fmt.Errorf("reading the address: %w", err)
	}
	return address, nil
}

// ForgetEmail removes the address.
//
// Called when an item closes: the address was collected for one purpose, that
// purpose is over, and keeping it afterwards is keeping personal data for no
// reason. SECURITY.md says it is deleted when the item closes, and this is the
// code that has to make that true.
func (s Store) ForgetEmail(itemID int64) error {
	// Both columns: the plain one this writes to, and the sealed one an older
	// version of this product may have filled. Forgetting an address has to
	// forget it wherever it is, or the promise in SECURITY.md is only true for
	// databases created after today.
	_, err := s.DB.Exec(
		`UPDATE item SET author_email = '', author_email_enc = NULL WHERE id = ?`, itemID)
	if err != nil {
		return fmt.Errorf("forgetting the address: %w", err)
	}
	return nil
}

// Edit rewrites the title and the description.
//
// The team's, not the reporter's: somebody triaging fixes a title that says
// "it's broken", or trims a description with half a stack trace pasted twice.
// The same limits the public form applies, because a report the team edits into
// a shape the form would have refused is a report nothing else in this product
// expects to see.
//
// It does not touch the kind, the status or the moderation state. Those have
// their own doors, each with its own rules, and a single endpoint that could
// change everything is one where an authorisation mistake costs everything.
func (s Store) Edit(id int64, title, body string, now time.Time) error {
	title = strings.TrimSpace(title)
	body = strings.TrimSpace(body)
	if title == "" {
		return errors.New("a report needs a title")
	}
	if tooLong(title, maxTitle) {
		return fmt.Errorf("the title is longer than %d characters", maxTitle)
	}
	if tooLong(body, maxBody) {
		return fmt.Errorf("the description is longer than %d characters", maxBody)
	}

	// body_hash goes with it. It is what the flood of copy-paste spam is caught
	// by, and leaving it pointing at text that is no longer there would make
	// every edited report look like a stranger to that check.
	res, err := s.DB.Exec(`UPDATE item SET title = ?, body = ?, body_hash = ? WHERE id = ?`,
		title, body, BodyHash(title, body), id)
	if err != nil {
		return fmt.Errorf("editing the report: %w", err)
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		return ErrNotFound
	}
	return nil
}
