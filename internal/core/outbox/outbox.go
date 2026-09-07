// Package outbox is the queue every call to somebody else's server leaves by.
//
// No handler talks to a forge directly. Forges rate-limit — GitHub warns about
// it specifically when creating content quickly, which is exactly what a queue
// draining after an outage does — and forges go down. A request that failed
// because somebody else's server was having a bad afternoon is a report that
// got lost, and the person who wrote it will not write it again.
package outbox

import (
	"encoding/json"
	"fmt"
	"math"
	"time"

	"git.codigosh.com/CodigoSH/LastHelp/internal/platform/database"
)

// MaxAttempts is when to stop trying and leave it for a person.
//
// With the backoff below that is roughly a day and a half of retries, which
// covers an outage nobody is awake for and stops short of retrying a request
// that was never going to work.
const MaxAttempts = 12

// Job is one thing to do.
type Job struct {
	ID       int64
	Kind     string
	Payload  string
	Attempts int
	LastErr  string
}

// Store is the queue.
type Store struct {
	DB database.Execer
}

// Enqueue adds work.
func (s Store) Enqueue(kind string, payload any, now time.Time) error {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("encoding a %s job: %w", kind, err)
	}
	_, err = s.DB.Exec(`INSERT INTO outbox (kind, payload, next_attempt_at, created_at)
		VALUES (?, ?, ?, ?)`, kind, string(encoded), now.Unix(), now.Unix())
	if err != nil {
		return fmt.Errorf("queueing a %s job: %w", kind, err)
	}
	return nil
}

// Due returns work whose time has come, oldest first.
func (s Store) Due(now time.Time, limit int) ([]Job, error) {
	if limit <= 0 {
		limit = 10
	}
	rows, err := s.DB.Query(`SELECT id, kind, payload, attempts, last_error
		FROM outbox WHERE next_attempt_at <= ? ORDER BY next_attempt_at, id LIMIT ?`,
		now.Unix(), limit)
	if err != nil {
		return nil, fmt.Errorf("reading the queue: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []Job
	for rows.Next() {
		var j Job
		if err := rows.Scan(&j.ID, &j.Kind, &j.Payload, &j.Attempts, &j.LastErr); err != nil {
			return nil, fmt.Errorf("reading the queue: %w", err)
		}
		out = append(out, j)
	}
	return out, rows.Err()
}

// Done removes a job that succeeded.
func (s Store) Done(id int64) error {
	_, err := s.DB.Exec(`DELETE FROM outbox WHERE id = ?`, id)
	return err
}

// Retry puts a job back with a longer wait.
//
// Exponential, capped at six hours. The cap matters: without it, the twelfth
// attempt would be weeks away, which is indistinguishable from having given up
// but keeps a row around pretending otherwise.
func (s Store) Retry(id int64, attempts int, cause error, now time.Time) error {
	wait := time.Duration(math.Min(math.Pow(2, float64(attempts))*30, 6*60*60)) * time.Second

	_, err := s.DB.Exec(`UPDATE outbox
		SET attempts = attempts + 1, last_error = ?, next_attempt_at = ?
		WHERE id = ?`, truncate(cause.Error()), now.Add(wait).Unix(), id)
	return err
}

// Abandon marks a job as beyond retrying.
//
// It is kept rather than deleted: a job that failed for good is something a
// person has to know about, and a queue that silently drops what it could not
// do is a queue that loses work without saying so. Pushing next_attempt_at far
// out keeps it out of Due without hiding it from anybody looking.
func (s Store) Abandon(id int64, cause error, now time.Time) error {
	_, err := s.DB.Exec(`UPDATE outbox
		SET attempts = ?, last_error = ?, next_attempt_at = ?
		WHERE id = ?`, MaxAttempts, "gave up: "+truncate(cause.Error()),
		now.Add(100*365*24*time.Hour).Unix(), id)
	return err
}

// Abandoned is what a person needs to look at.
func (s Store) Abandoned() ([]Job, error) {
	rows, err := s.DB.Query(`SELECT id, kind, payload, attempts, last_error
		FROM outbox WHERE attempts >= ? ORDER BY id`, MaxAttempts)
	if err != nil {
		return nil, fmt.Errorf("reading abandoned jobs: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []Job
	for rows.Next() {
		var j Job
		if err := rows.Scan(&j.ID, &j.Kind, &j.Payload, &j.Attempts, &j.LastErr); err != nil {
			return nil, err
		}
		out = append(out, j)
	}
	return out, rows.Err()
}

func truncate(message string) string {
	const limit = 500
	if len(message) <= limit {
		return message
	}
	return message[:limit] + "…"
}
