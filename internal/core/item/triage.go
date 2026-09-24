package item

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Approve publishes something that was waiting.
func (s Store) Approve(id int64, now time.Time) error {
	return s.setModeration(id, Published, now)
}

// Reject hides it without deleting it.
//
// Kept rather than removed, and for a reason worth stating: without the
// rejected pile there is no way to find out who is being turned away by
// mistake, and a moderation queue nobody can audit is one that quietly starts
// rejecting a language, a phrasing, or a person.
func (s Store) Reject(id int64, now time.Time) error {
	return s.setModeration(id, Rejected, now)
}

// MarkSpam is Reject plus a signal. What it marks is what teaches the filters,
// which is why it is a different verb rather than a checkbox on the same one.
func (s Store) MarkSpam(id int64, now time.Time) error {
	return s.setModeration(id, Spam, now)
}

func (s Store) setModeration(id int64, state Moderation, now time.Time) error {
	published := any(nil)
	if state == Published {
		published = now.Unix()
	}
	res, err := s.DB.Exec(`UPDATE item SET moderation_state = ?,
		published_at = COALESCE(published_at, ?) WHERE id = ?`, string(state), published, id)
	if err != nil {
		return fmt.Errorf("moderating: %w", err)
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		return ErrNotFound
	}
	return nil
}

// SetStatus moves something along its own vocabulary.
//
// The status set is per kind and closed. A status nobody wrote a screen for
// renders as blank, so it is refused here rather than stored.
func (s Store) SetStatus(id int64, status Status, now time.Time) error {
	var kind string
	if err := s.DB.QueryRow(`SELECT kind FROM item WHERE id = ?`, id).Scan(&kind); err != nil {
		return ErrNotFound
	}
	if !validStatus(Kind(kind), status) {
		return fmt.Errorf("a %s cannot be %q; it may be one of %v", kind, status, StatusesFor(Kind(kind)))
	}

	closed := any(nil)
	if status == StatusDone || status == StatusClosed ||
		status == StatusDeclined || status == StatusNotReproducible {
		closed = now.Unix()
	}
	_, err := s.DB.Exec(`UPDATE item SET status = ?, closed_at = ? WHERE id = ?`,
		string(status), closed, id)
	if err != nil {
		return fmt.Errorf("setting the status: %w", err)
	}
	return nil
}

// Merge folds a duplicate into the one that survives, moving its votes across.
//
// The votes move rather than being added: two people who voted both items must
// count once afterwards, which is what the primary key on (item, voter) gives
// for free, and adding the counts would double them.
func (s Store) Merge(duplicateID, survivorID int64, now time.Time) error {
	if duplicateID == survivorID {
		return errors.New("an item cannot be merged into itself")
	}

	if _, err := s.DB.Exec(`INSERT INTO vote (item_id, voter_id, created_at)
		SELECT ?, voter_id, created_at FROM vote WHERE item_id = ?
		ON CONFLICT (item_id, voter_id) DO NOTHING`, survivorID, duplicateID); err != nil {
		return fmt.Errorf("moving the votes: %w", err)
	}
	if _, err := s.DB.Exec(`UPDATE item SET vote_count =
		(SELECT count(*) FROM vote WHERE item_id = ?) WHERE id = ?`,
		survivorID, survivorID); err != nil {
		return fmt.Errorf("recounting: %w", err)
	}
	res, err := s.DB.Exec(`UPDATE item SET merged_into = ?, moderation_state = ?, closed_at = ?
		WHERE id = ?`, survivorID, string(Rejected), now.Unix(), duplicateID)
	if err != nil {
		return fmt.Errorf("merging: %w", err)
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		return ErrNotFound
	}
	return nil
}

// Filter narrows a listing.
type Filter struct {
	RepoID int64
	// RepoIDs is several boards at once, which is what the panel's own listing
	// needs: somebody who triages four repositories wants one screen, not four.
	// When it is set RepoID is ignored; when both are empty nothing matches,
	// which is the direction a mistake here should fail in.
	RepoIDs []int64
	Kind    Kind   // empty for both
	Status  Status // empty for any
	// Statuses is several at once. When it is set Status is ignored: the front
	// page asks for "the ones that are finished", which is two statuses in two
	// different vocabularies — an idea is done, a bug is closed.
	Statuses   []Status
	Moderation Moderation // empty for published only
	// AnyModeration returns every state, including what is still waiting for a
	// person. It has to be asked for by name: the public listing leaves the
	// zero value alone and therefore cannot show something unreviewed by
	// accident, which is the whole reason the default is "published" and not
	// "everything".
	AnyModeration bool
	Search        string // empty for everything
	// AsTyped reads Search as something still being written: its last word
	// matches any word it begins, so "dar" already finds "dark". A search box
	// that answers while somebody types needs it; one that answers a finished
	// question does not.
	AsTyped    bool
	SortByVote bool
	// SortByClosed is newest finished first, and takes only what has actually
	// finished: an item with no closing time has not, whatever its status says,
	// and sorting those by a null would put them somewhere arbitrary.
	SortByClosed bool
	Limit        int
	Offset       int
}

// List returns what matches, newest or most-wanted first.
//
// The default is published items only. A caller that wants the queue asks for
// it by name, which means the public listing cannot accidentally include
// something waiting for a human.
func (s Store) List(f Filter) ([]Item, error) {
	where := []string{"merged_into IS NULL"}
	var args []any

	if len(f.RepoIDs) > 0 {
		where = append(where, "repo_id IN ("+placeholders(len(f.RepoIDs))+")")
		for _, id := range f.RepoIDs {
			args = append(args, id)
		}
	} else {
		where = append(where, "repo_id = ?")
		args = append(args, f.RepoID)
	}

	switch {
	case f.AnyModeration:
		// Every state, asked for by name.
	case f.Moderation == "":
		where = append(where, "moderation_state = ?")
		args = append(args, string(Published))
	default:
		where = append(where, "moderation_state = ?")
		args = append(args, string(f.Moderation))
	}
	if f.Kind != "" {
		where = append(where, "kind = ?")
		args = append(args, string(f.Kind))
	}
	switch {
	case len(f.Statuses) > 0:
		where = append(where, "status IN ("+placeholders(len(f.Statuses))+")")
		for _, st := range f.Statuses {
			args = append(args, string(st))
		}
	case f.Status != "":
		where = append(where, "status = ?")
		args = append(args, string(f.Status))
	}
	if f.SortByClosed {
		where = append(where, "closed_at IS NOT NULL")
	}

	// The search is a subquery rather than a join, on purpose. item_fts has its
	// own title and body columns, so joining it makes every unqualified column
	// ambiguous and forces the select list to be rewritten with a prefix — and
	// a helper that rewrites a column list by splitting on commas breaks on the
	// COALESCE that is already in it. This needs no rewriting at all.
	query := `SELECT ` + itemColumns + ` FROM item`
	if q := strings.TrimSpace(f.Search); q != "" {
		where = append(where, "id IN (SELECT rowid FROM item_fts WHERE item_fts MATCH ?)")
		args = append(args, ftsQuery(q, f.AsTyped))
	}
	query += ` WHERE ` + strings.Join(where, " AND ")

	switch {
	case f.SortByVote:
		query += ` ORDER BY vote_count DESC, created_at DESC`
	case f.SortByClosed:
		query += ` ORDER BY closed_at DESC, id DESC`
	default:
		query += ` ORDER BY created_at DESC`
	}

	limit := f.Limit
	if limit <= 0 || limit > 200 {
		// Every listing is bounded. An unbounded query over a table the public
		// can add rows to is a page that gets slower every week and then stops.
		limit = 50
	}
	query += fmt.Sprintf(" LIMIT %d OFFSET %d", limit, max(f.Offset, 0))

	rows, err := s.DB.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("listing items: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []Item
	for rows.Next() {
		it, err := s.scan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

// Similar is the "this looks like these three" hint, shown while somebody is
// still typing a title.
//
// FTS5 over the text we already store, rather than embeddings from a paid API
// that would also be shown everything the public writes. It finds less; it
// costs nothing, runs instantly, and never leaves the binary.
func (s Store) Similar(repoID int64, title string, limit int) ([]Item, error) {
	if strings.TrimSpace(title) == "" {
		return nil, nil
	}
	if limit <= 0 {
		limit = 3
	}
	return s.List(Filter{RepoID: repoID, Search: title, SortByVote: true, Limit: limit})
}

// ftsQuery turns what somebody typed into something FTS5 will accept.
//
// Every token is quoted, because the syntax has operators — NEAR, OR, a bare
// asterisk, an unbalanced quote — and a title containing one would otherwise be
// a syntax error rather than a search. This is the same class of bug as an
// unescaped quote in SQL, and it is not solved by parameters: the parameter is
// the query language.
//
// asTyped marks the last token as a prefix. The asterisk goes outside the
// quotes, where FTS5 reads it as the prefix operator on a quoted string rather
// than as part of what is searched for.
func ftsQuery(input string, asTyped bool) string {
	fields := strings.Fields(input)
	quoted := make([]string, 0, len(fields))
	for _, f := range fields {
		f = strings.ReplaceAll(f, `"`, "")
		if f == "" {
			continue
		}
		quoted = append(quoted, `"`+f+`"`)
	}
	if len(quoted) == 0 {
		return `""`
	}
	if asTyped {
		quoted[len(quoted)-1] += "*"
	}
	return strings.Join(quoted, " OR ")
}

// placeholders is the "?, ?, ?" for an IN clause of a known length.
//
// The values are still bound; only the number of slots is built by hand, which
// is the one thing a parameter cannot carry.
func placeholders(n int) string {
	return strings.TrimSuffix(strings.Repeat("?, ", n), ", ")
}

// countable is the set of columns Counts may group by.
//
// A whitelist rather than a quoted identifier: the column name goes into the
// query as text, and the only safe way to put a caller's string there is to not
// put a caller's string there.
var countable = map[string]bool{"status": true, "kind": true, "moderation_state": true}

// Counts groups the items of these boards by one column.
//
// It is what lets a listing say how many are waiting before anybody clicks the
// tab that shows them — and it counts in the database, so a portal that has
// been useful for two years opens as fast as one installed this morning.
func (s Store) Counts(repoIDs []int64, column string) (map[string]int, error) {
	out := map[string]int{}
	if len(repoIDs) == 0 {
		return out, nil
	}
	if !countable[column] {
		return nil, fmt.Errorf("counting by %q is not something this counts by", column)
	}

	args := make([]any, 0, len(repoIDs))
	for _, id := range repoIDs {
		args = append(args, id)
	}
	rows, err := s.DB.Query(
		`SELECT `+column+`, count(*) FROM item
		  WHERE merged_into IS NULL AND repo_id IN (`+placeholders(len(repoIDs))+`)
		  GROUP BY `+column, args...)
	if err != nil {
		return nil, fmt.Errorf("counting items: %w", err)
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var key string
		var n int
		if err := rows.Scan(&key, &n); err != nil {
			return nil, err
		}
		out[key] = n
	}
	return out, rows.Err()
}

// Counts is how much has come in and where it got to.
//
// Three numbers and no more. "Received" is everything a person let through,
// "Done" is what was actually put right, and "Open" is what is still waiting on
// somebody. Anything refused is in neither of the last two on purpose: it is
// not open and it was not done, and counting it as either would be the page
// saying something that is not so.
type Counts struct {
	Received int
	Open     int
	Done     int
}

// Count answers all three in one query.
//
// One query rather than three because this runs on the public front page, and
// because three would be three snapshots of three different instants: a report
// published between the first and the third would be received and not counted
// anywhere else.
func (s Store) Count(repoIDs []int64) (Counts, error) {
	var out Counts
	if len(repoIDs) == 0 {
		return out, nil
	}

	args := make([]any, 0, len(repoIDs)+6)
	for _, id := range repoIDs {
		args = append(args, id)
	}
	for _, st := range []Status{StatusDone, StatusClosed} {
		args = append(args, string(st))
	}
	for _, st := range []Status{StatusDone, StatusClosed, StatusDeclined, StatusNotReproducible} {
		args = append(args, string(st))
	}

	query := `SELECT
			count(*),
			sum(CASE WHEN status IN (?, ?) THEN 1 ELSE 0 END),
			sum(CASE WHEN status NOT IN (?, ?, ?, ?) THEN 1 ELSE 0 END)
		FROM item
		WHERE merged_into IS NULL AND moderation_state = '` + string(Published) + `'
		  AND repo_id IN (` + placeholders(len(repoIDs)) + `)`

	// The repository ids come first in the WHERE, so they go first in the args.
	ordered := make([]any, 0, len(args))
	ordered = append(ordered, args[len(repoIDs):]...)
	ordered = append(ordered, args[:len(repoIDs)]...)

	var done, open sql.NullInt64
	err := s.DB.QueryRow(query, ordered...).Scan(&out.Received, &done, &open)
	if err != nil {
		return Counts{}, fmt.Errorf("counting reports: %w", err)
	}
	out.Done = int(done.Int64)
	out.Open = int(open.Int64)
	return out, nil
}
