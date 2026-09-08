package item

import (
	"fmt"
	"strings"
	"time"
)

// Who wrote a comment. The three are kept apart because they are moderated
// differently and drawn differently: the team's word is the answer, the
// reporter's is the follow-up, a visitor's is a contribution that waits.
type Author string

const (
	AuthorTeam     Author = "team"
	AuthorReporter Author = "reporter"
	AuthorVisitor  Author = "visitor"
)

// Comment is one message on an item.
type Comment struct {
	ID         int64
	ItemID     int64
	Author     Author
	AuthorRef  string // the team member's name; empty for the others
	Body       string
	Moderation Moderation
	CreatedAt  time.Time
}

const maxComment = 10_000

// Comment adds a message.
//
// Anything not written by the team waits for a human, including the reporter's
// own follow-up: the secret link proves who they are, not that what they wrote
// belongs on a public page.
func (s Store) Comment(itemID int64, author Author, authorRef, body string, now time.Time) (Comment, error) {
	body = strings.TrimSpace(body)
	if body == "" {
		return Comment{}, fmt.Errorf("a comment needs something in it")
	}
	if len(body) > maxComment {
		return Comment{}, fmt.Errorf("the comment is longer than %d characters", maxComment)
	}

	moderation := Pending
	if author == AuthorTeam {
		moderation = Published
	}

	res, err := s.DB.Exec(`INSERT INTO comment
		(item_id, author_kind, author_ref, body, moderation_state, created_at)
		VALUES (?, ?, ?, ?, ?, ?)`,
		itemID, string(author), authorRef, body, string(moderation), now.Unix())
	if err != nil {
		if strings.Contains(err.Error(), "FOREIGN KEY constraint failed") {
			return Comment{}, ErrNotFound
		}
		return Comment{}, fmt.Errorf("storing the comment: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return Comment{}, fmt.Errorf("storing the comment: %w", err)
	}
	return Comment{
		ID: id, ItemID: itemID, Author: author, AuthorRef: authorRef,
		Body: body, Moderation: moderation, CreatedAt: now,
	}, nil
}

// Comments returns the thread. Published only unless the caller asks for the
// queue, so the public listing cannot accidentally show something waiting.
func (s Store) Comments(itemID int64, moderation Moderation) ([]Comment, error) {
	if moderation == "" {
		moderation = Published
	}
	rows, err := s.DB.Query(`SELECT id, item_id, author_kind, author_ref, body,
		moderation_state, created_at FROM comment
		WHERE item_id = ? AND moderation_state = ? ORDER BY created_at, id`,
		itemID, string(moderation))
	if err != nil {
		return nil, fmt.Errorf("reading the thread: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []Comment
	for rows.Next() {
		var (
			c       Comment
			author  string
			mod     string
			created int64
		)
		if err := rows.Scan(&c.ID, &c.ItemID, &author, &c.AuthorRef, &c.Body, &mod, &created); err != nil {
			return nil, fmt.Errorf("reading the thread: %w", err)
		}
		c.Author = Author(author)
		c.Moderation = Moderation(mod)
		c.CreatedAt = time.Unix(created, 0)
		out = append(out, c)
	}
	return out, rows.Err()
}

// Waiting is every comment on a repository that a person has still to look at,
// newest first, each carrying the report it was written on.
//
// It exists because the rest of this file did not add up to anything: a
// visitor's comment was stored as Pending and ApproveComment had no caller, so
// the queue was write-only. The public page said "a person will read this
// before it appears" and no person could — the promise was in the interface
// and the way to keep it was missing.
type Waiting struct {
	Comment
	ItemPublicID string
	ItemTitle    string
}

func (s Store) WaitingComments(repoID int64, limit int) ([]Waiting, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.DB.Query(`SELECT c.id, c.item_id, c.author_kind, c.author_ref, c.body,
		c.moderation_state, c.created_at, i.public_id, i.title
		FROM comment c
		JOIN item i ON i.id = c.item_id
		WHERE i.repo_id = ? AND c.moderation_state = ?
		ORDER BY c.created_at DESC, c.id DESC LIMIT ?`,
		repoID, string(Pending), limit)
	if err != nil {
		return nil, fmt.Errorf("reading the comment queue: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []Waiting
	for rows.Next() {
		var (
			w       Waiting
			author  string
			mod     string
			created int64
		)
		if err := rows.Scan(&w.ID, &w.ItemID, &author, &w.AuthorRef, &w.Body, &mod, &created,
			&w.ItemPublicID, &w.ItemTitle); err != nil {
			return nil, fmt.Errorf("reading the comment queue: %w", err)
		}
		w.Author = Author(author)
		w.Moderation = Moderation(mod)
		w.CreatedAt = time.Unix(created, 0)
		out = append(out, w)
	}
	return out, rows.Err()
}

// CommentItem is which report a comment belongs to, which is what authorising
// a decision about it needs: the rules are written about repositories, and a
// comment id on its own names nothing they can answer about.
func (s Store) CommentItem(commentID int64) (Item, error) {
	var itemID int64
	if err := s.DB.QueryRow(`SELECT item_id FROM comment WHERE id = ?`, commentID).Scan(&itemID); err != nil {
		return Item{}, ErrNotFound
	}
	return s.ByID(itemID)
}

// ApproveComment publishes one that was waiting.
func (s Store) ApproveComment(id int64) error {
	res, err := s.DB.Exec(`UPDATE comment SET moderation_state = ? WHERE id = ?`, string(Published), id)
	if err != nil {
		return fmt.Errorf("approving the comment: %w", err)
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		return ErrNotFound
	}
	return nil
}

// RejectComment hides one without deleting it, for the same reason a rejected
// item is kept: a queue nobody can audit starts turning people away invisibly.
func (s Store) RejectComment(id int64) error {
	res, err := s.DB.Exec(`UPDATE comment SET moderation_state = ? WHERE id = ?`, string(Rejected), id)
	if err != nil {
		return fmt.Errorf("rejecting the comment: %w", err)
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		return ErrNotFound
	}
	return nil
}
