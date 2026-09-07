package item

import (
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
	RepoID     int64
	Kind       Kind       // empty for both
	Status     Status     // empty for any
	Moderation Moderation // empty for published only
	Search     string     // empty for everything
	SortByVote bool
	Limit      int
	Offset     int
}

// List returns what matches, newest or most-wanted first.
//
// The default is published items only. A caller that wants the queue asks for
// it by name, which means the public listing cannot accidentally include
// something waiting for a human.
func (s Store) List(f Filter) ([]Item, error) {
	where := []string{"repo_id = ?", "merged_into IS NULL"}
	args := []any{f.RepoID}

	if f.Moderation == "" {
		where = append(where, "moderation_state = ?")
		args = append(args, string(Published))
	} else {
		where = append(where, "moderation_state = ?")
		args = append(args, string(f.Moderation))
	}
	if f.Kind != "" {
		where = append(where, "kind = ?")
		args = append(args, string(f.Kind))
	}
	if f.Status != "" {
		where = append(where, "status = ?")
		args = append(args, string(f.Status))
	}

	// The search is a subquery rather than a join, on purpose. item_fts has its
	// own title and body columns, so joining it makes every unqualified column
	// ambiguous and forces the select list to be rewritten with a prefix — and
	// a helper that rewrites a column list by splitting on commas breaks on the
	// COALESCE that is already in it. This needs no rewriting at all.
	query := `SELECT ` + itemColumns + ` FROM item`
	if q := strings.TrimSpace(f.Search); q != "" {
		where = append(where, "id IN (SELECT rowid FROM item_fts WHERE item_fts MATCH ?)")
		args = append(args, ftsQuery(q))
	}
	query += ` WHERE ` + strings.Join(where, " AND ")

	if f.SortByVote {
		query += ` ORDER BY vote_count DESC, created_at DESC`
	} else {
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
func ftsQuery(input string) string {
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
	return strings.Join(quoted, " OR ")
}
