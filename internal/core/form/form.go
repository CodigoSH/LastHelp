// Package form is a project's own questions: the ones its team adds to the
// report form beside the title and the description.
//
// Four kinds of question and no more — a line of text, a paragraph, a choice
// from a list, a date — because a form with fifteen kinds of field is a
// questionnaire nobody fills in. The table they live in, field_def, has been in
// the schema since the first version for exactly this.
//
// What somebody answers is stored with the label the question had when they
// answered it (item.Field), so renaming or removing a question here never
// rewrites a report that was already sent.
package form

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"git.codigosh.com/CodigoSH/LastHelp/internal/platform/database"
)

// Type is what kind of answer a question takes.
type Type string

const (
	Short  Type = "short"
	Long   Type = "long"
	Choice Type = "choice"
	Date   Type = "date"
)

func (t Type) valid() bool { return t == Short || t == Long || t == Choice || t == Date }

// For is which kind of report a question is asked of. The codes are the item
// kinds, "bug" and "idea", so the two can be compared without a table.
type For string

const (
	Problems For = "bug"
	Ideas    For = "idea"
	Both     For = "both"
)

func (f For) valid() bool { return f == Problems || f == Ideas || f == Both }

// The bounds, in characters: what they protect is the form and the report
// page somebody reads them on, and those are measured in what a reader sees.
const (
	MaxFields  = 10
	MaxLabel   = 60
	MaxOptions = 20
	MaxOption  = 60
	// MaxShort is a line. A paragraph is bounded where every answer is, by the
	// report's own limit on what it stores.
	MaxShort = 200
)

var (
	ErrNotFound = errors.New("no such question")
	ErrTooMany  = errors.New("a project has as many questions as it may")
	ErrInvalid  = errors.New("the question is not one that can be asked")

	// What an answer can be wrong in. Each is refused rather than stored: a
	// required question left empty, a choice that is not on the list, a date that
	// is not a date, a line longer than a line.
	ErrRequired    = errors.New("a required question was not answered")
	ErrNotAnOption = errors.New("the answer is not one of the choices")
	ErrNotADate    = errors.New("the answer is not a date")
	ErrTooLong     = errors.New("the answer is longer than the question takes")
)

// Field is one question.
type Field struct {
	ID       int64
	RepoID   int64
	Key      string
	Label    string
	Type     Type
	Required bool
	Options  []string
	For      For
	Position int
}

// Asks says whether this question is put to a report of this kind ("bug" or
// "idea").
func (f Field) Asks(kind string) bool { return f.For == Both || string(f.For) == kind }

// Check reads an answer to this question. Empty is fine unless the question
// is required; anything else has to be the kind of answer the question takes.
func (f Field) Check(value string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		if f.Required {
			return ErrRequired
		}
		return nil
	}
	switch f.Type {
	case Short:
		if utf8.RuneCountInString(value) > MaxShort {
			return ErrTooLong
		}
	case Choice:
		for _, o := range f.Options {
			if o == value {
				return nil
			}
		}
		return ErrNotAnOption
	case Date:
		if _, err := time.Parse(time.DateOnly, value); err != nil {
			return ErrNotADate
		}
	}
	return nil
}

// clean trims what a team typed and refuses a question that could not be
// asked: no label, a label past the bound, a kind of answer or a kind of report
// that does not exist, or a choice with nothing to choose from.
func clean(f Field) (Field, error) {
	f.Label = strings.TrimSpace(f.Label)
	if f.Label == "" || utf8.RuneCountInString(f.Label) > MaxLabel || !f.Type.valid() || !f.For.valid() {
		return Field{}, ErrInvalid
	}
	if f.Type != Choice {
		f.Options = nil
		return f, nil
	}
	seen := map[string]bool{}
	var options []string
	for _, o := range f.Options {
		o = strings.TrimSpace(o)
		if o == "" || seen[o] {
			continue
		}
		if utf8.RuneCountInString(o) > MaxOption {
			return Field{}, ErrInvalid
		}
		seen[o] = true
		options = append(options, o)
	}
	if len(options) == 0 || len(options) > MaxOptions {
		return Field{}, ErrInvalid
	}
	f.Options = options
	return f, nil
}

// Store reads and writes a project's questions.
type Store struct{ DB database.Execer }

const columns = `id, repo_id, key, label, type, required, options, applies_to, position`

func scan(row interface{ Scan(...any) error }) (Field, error) {
	var f Field
	var typ, options, applies string
	if err := row.Scan(&f.ID, &f.RepoID, &f.Key, &f.Label, &typ, &f.Required, &options, &applies, &f.Position); err != nil {
		return Field{}, err
	}
	f.Type, f.For = Type(typ), For(applies)
	if options != "" {
		// A list that does not parse is read as no list, and a choice with no
		// list refuses every answer rather than accepting any.
		_ = json.Unmarshal([]byte(options), &f.Options)
	}
	return f, nil
}

// List is a project's questions in the order they are asked.
func (s Store) List(repoID int64) ([]Field, error) {
	rows, err := s.DB.Query(`SELECT `+columns+` FROM field_def WHERE repo_id = ? ORDER BY position, id`, repoID)
	if err != nil {
		return nil, fmt.Errorf("listing the questions: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := []Field{}
	for rows.Next() {
		f, err := scan(rows)
		if err != nil {
			return nil, fmt.Errorf("listing the questions: %w", err)
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

func (s Store) byKey(repoID int64, key string) (Field, error) {
	f, err := scan(s.DB.QueryRow(`SELECT `+columns+` FROM field_def WHERE repo_id = ? AND key = ?`, repoID, key))
	if errors.Is(err, sql.ErrNoRows) {
		return Field{}, ErrNotFound
	}
	if err != nil {
		return Field{}, fmt.Errorf("reading the question: %w", err)
	}
	return f, nil
}

// newKey names a question for as long as it exists. Never its label, which
// the team may reword, and never a word, which could be "version" or "steps" —
// the two questions every project has. An "f" and eight hex digits is neither.
func newKey() (string, error) {
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return "f" + hex.EncodeToString(b), nil
}

func encodeOptions(options []string) string {
	if len(options) == 0 {
		return ""
	}
	b, _ := json.Marshal(options)
	return string(b)
}

// Create adds a question at the end of a project's form.
func (s Store) Create(f Field) (Field, error) {
	f, err := clean(f)
	if err != nil {
		return Field{}, err
	}
	var count, last int
	if err := s.DB.QueryRow(`SELECT count(*), coalesce(max(position), -1) FROM field_def WHERE repo_id = ?`, f.RepoID).
		Scan(&count, &last); err != nil {
		return Field{}, fmt.Errorf("counting the questions: %w", err)
	}
	if count >= MaxFields {
		return Field{}, ErrTooMany
	}
	if f.Key, err = newKey(); err != nil {
		return Field{}, err
	}
	f.Position = last + 1
	res, err := s.DB.Exec(`INSERT INTO field_def (repo_id, key, label, type, required, options, applies_to, position)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		f.RepoID, f.Key, f.Label, string(f.Type), f.Required, encodeOptions(f.Options), string(f.For), f.Position)
	if err != nil {
		return Field{}, fmt.Errorf("adding the question: %w", err)
	}
	if f.ID, err = res.LastInsertId(); err != nil {
		return Field{}, fmt.Errorf("adding the question: %w", err)
	}
	return f, nil
}

// Update rewords a question, or changes whether it is required, what it offers
// and which reports it is asked of. Not the kind of answer it takes: answers
// already given were given to that kind, and a team that wants another kind
// adds a new question.
func (s Store) Update(repoID int64, key string, change Field) (Field, error) {
	current, err := s.byKey(repoID, key)
	if err != nil {
		return Field{}, err
	}
	change.Type = current.Type
	change, err = clean(change)
	if err != nil {
		return Field{}, err
	}
	current.Label, current.Required, current.Options, current.For = change.Label, change.Required, change.Options, change.For
	if _, err := s.DB.Exec(`UPDATE field_def SET label = ?, required = ?, options = ?, applies_to = ? WHERE id = ?`,
		current.Label, current.Required, encodeOptions(current.Options), string(current.For), current.ID); err != nil {
		return Field{}, fmt.Errorf("changing the question: %w", err)
	}
	return current, nil
}

// Delete removes a question from the form. The answers somebody already gave
// it stay on their reports, under the label they were given to.
func (s Store) Delete(repoID int64, key string) error {
	res, err := s.DB.Exec(`DELETE FROM field_def WHERE repo_id = ? AND key = ?`, repoID, key)
	if err != nil {
		return fmt.Errorf("removing the question: %w", err)
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		return ErrNotFound
	}
	return nil
}

// Move swaps a question with the one before it (up) or after it. At either
// end there is nothing to swap with, and nothing changes.
func (s Store) Move(repoID int64, key string, up bool) error {
	all, err := s.List(repoID)
	if err != nil {
		return err
	}
	at := -1
	for i, f := range all {
		if f.Key == key {
			at = i
		}
	}
	if at < 0 {
		return ErrNotFound
	}
	other := at + 1
	if up {
		other = at - 1
	}
	if other < 0 || other >= len(all) {
		return nil
	}
	// The positions are rewritten from the order as it is, so two rows that
	// somehow share a position cannot leave a move that does nothing — and in one
	// statement, so a form is never left half reordered.
	all[at], all[other] = all[other], all[at]
	var cases strings.Builder
	args := make([]any, 0, 2*len(all)+1)
	for i, f := range all {
		cases.WriteString(" WHEN ? THEN ?")
		args = append(args, f.ID, i)
	}
	args = append(args, repoID)
	if _, err := s.DB.Exec(`UPDATE field_def SET position = CASE id`+cases.String()+` ELSE position END WHERE repo_id = ?`, args...); err != nil {
		return fmt.Errorf("moving the question: %w", err)
	}
	return nil
}
