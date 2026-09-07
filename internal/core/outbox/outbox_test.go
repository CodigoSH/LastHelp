package outbox

import (
	"errors"
	"path/filepath"
	"testing"
	"time"

	"git.codigosh.com/CodigoSH/LastHelp/internal/platform/database"
)

var epoch = time.Unix(1_700_000_000, 0)

func queue(t *testing.T) Store {
	t.Helper()
	db, err := database.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return Store{DB: db}
}

func TestWorkComesBackOut(t *testing.T) {
	q := queue(t)
	if err := q.Enqueue("create_issue", map[string]any{"item": 7}, epoch); err != nil {
		t.Fatal(err)
	}

	due, err := q.Due(epoch, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(due) != 1 || due[0].Kind != "create_issue" {
		t.Fatalf("got %+v", due)
	}
	if err := q.Done(due[0].ID); err != nil {
		t.Fatal(err)
	}
	if due, _ = q.Due(epoch, 10); len(due) != 0 {
		t.Error("a finished job came back")
	}
}

// TestAFailedJobWaitsLongerEachTime.
//
// A queue that retries immediately turns somebody else's outage into a denial
// of service aimed at them, and gets this instance rate limited for the rest of
// the day.
func TestAFailedJobWaitsLongerEachTime(t *testing.T) {
	q := queue(t)
	if err := q.Enqueue("create_issue", map[string]any{}, epoch); err != nil {
		t.Fatal(err)
	}
	due, _ := q.Due(epoch, 1)
	job := due[0]

	waits := []time.Duration{}
	at := epoch
	for attempt := 0; attempt < 4; attempt++ {
		if err := q.Retry(job.ID, attempt, errors.New("the forge is down"), at); err != nil {
			t.Fatal(err)
		}
		var next int64
		if err := q.DB.QueryRow(`SELECT next_attempt_at FROM outbox WHERE id = ?`, job.ID).Scan(&next); err != nil {
			t.Fatal(err)
		}
		waits = append(waits, time.Unix(next, 0).Sub(at))
		at = time.Unix(next, 0)
	}

	for i := 1; i < len(waits); i++ {
		if waits[i] <= waits[i-1] {
			t.Fatalf("wait %d (%v) is not longer than wait %d (%v)", i, waits[i], i-1, waits[i-1])
		}
	}

	// And it is not due before its time.
	if due, _ := q.Due(epoch, 10); len(due) != 0 {
		t.Error("a job waiting for its next attempt came back as due")
	}
}

// TestTheBackoffIsCapped.
//
// Without the cap the twelfth attempt is weeks away, which is giving up while
// keeping a row that pretends otherwise.
func TestTheBackoffIsCapped(t *testing.T) {
	q := queue(t)
	if err := q.Enqueue("create_issue", map[string]any{}, epoch); err != nil {
		t.Fatal(err)
	}
	due, _ := q.Due(epoch, 1)

	if err := q.Retry(due[0].ID, 30, errors.New("still down"), epoch); err != nil {
		t.Fatal(err)
	}
	var next int64
	if err := q.DB.QueryRow(`SELECT next_attempt_at FROM outbox WHERE id = ?`, due[0].ID).Scan(&next); err != nil {
		t.Fatal(err)
	}
	if wait := time.Unix(next, 0).Sub(epoch); wait > 6*time.Hour {
		t.Errorf("the wait after 30 attempts is %v; the cap is six hours", wait)
	}
}

// TestGivingUpKeepsTheJobVisible.
//
// A queue that silently drops what it could not do loses work without telling
// anybody, and the person who wrote the report never finds out it went nowhere.
func TestGivingUpKeepsTheJobVisible(t *testing.T) {
	q := queue(t)
	if err := q.Enqueue("create_issue", map[string]any{"item": 7}, epoch); err != nil {
		t.Fatal(err)
	}
	due, _ := q.Due(epoch, 1)

	if err := q.Abandon(due[0].ID, errors.New("the token was revoked"), epoch); err != nil {
		t.Fatal(err)
	}

	if due, _ := q.Due(epoch.Add(365*24*time.Hour), 10); len(due) != 0 {
		t.Error("an abandoned job is still being retried a year later")
	}

	abandoned, err := q.Abandoned()
	if err != nil {
		t.Fatal(err)
	}
	if len(abandoned) != 1 {
		t.Fatalf("%d abandoned jobs are visible, want 1", len(abandoned))
	}
	if abandoned[0].LastErr == "" {
		t.Error("the reason it gave up was not kept, so nobody can act on it")
	}
}

func TestOldestFirst(t *testing.T) {
	q := queue(t)
	for i := 0; i < 3; i++ {
		if err := q.Enqueue("create_issue", map[string]any{"n": i}, epoch.Add(time.Duration(i)*time.Minute)); err != nil {
			t.Fatal(err)
		}
	}
	due, err := q.Due(epoch.Add(time.Hour), 10)
	if err != nil {
		t.Fatal(err)
	}
	for i := 1; i < len(due); i++ {
		if due[i].ID < due[i-1].ID {
			t.Fatal("the queue came back out of order; a report would be filed before an earlier one")
		}
	}
}
