package bridge

import (
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"git.codigosh.com/CodigoSH/LastHelp/internal/core/forge"
	"git.codigosh.com/CodigoSH/LastHelp/internal/core/item"
)

// Applied says what a delivery did, so the caller can log something useful and
// a test can assert on something other than "no error".
type Applied struct {
	Item    string // the public id, empty when nothing matched
	Status  item.Status
	Skipped string // why nothing happened, when nothing did
}

// Apply is a verified delivery arriving.
//
// It refuses three things before it changes anything, and each refusal is a bug
// this design exists to prevent:
//
//   - A delivery already seen. Every forge redelivers, and anybody who captured
//     one can replay it.
//   - A delivery describing what this server just wrote. That is our own echo
//     coming back, and applying it is the first half of a loop.
//   - A delivery about an issue nothing here is linked to. Somebody else's
//     issue must not move somebody's item.
func (b Bridge) Apply(orgID int64, repoID int64, event forge.Event, now time.Time) (Applied, error) {
	if event.Kind == forge.EventIgnored {
		return Applied{Skipped: "not an event we act on"}, nil
	}

	if event.DeliveryID != "" {
		seen, err := b.seenBefore(orgID, event.DeliveryID, now)
		if err != nil {
			return Applied{}, err
		}
		if seen {
			return Applied{Skipped: "already delivered"}, nil
		}
	}

	var (
		itemID      int64
		fingerprint string
	)
	err := b.DB.QueryRow(`SELECT item_id, echo_fingerprint FROM issue_link
		WHERE repo_id = ? AND forge_number = ?`, repoID, event.Number).
		Scan(&itemID, &fingerprint)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		// An issue nobody here is following. Most issues on a real repository
		// are exactly that.
		return Applied{Skipped: "no item is linked to that issue"}, nil
	case err != nil:
		return Applied{}, fmt.Errorf("finding the linked item: %w", err)
	}

	arriving := Fingerprint(forge.Issue{State: event.State, Labels: event.Labels})
	if arriving == fingerprint {
		return Applied{Skipped: "our own change coming back"}, nil
	}

	it, err := b.Items.ByID(itemID)
	if err != nil {
		return Applied{}, err
	}

	status, ok := statusFor(it.Kind, event)
	if !ok {
		// The state changed in a way this product does not track. The link is
		// still updated so the next delivery is compared against what is really
		// there rather than against something stale.
		if err := b.touch(itemID, event, now); err != nil {
			return Applied{}, err
		}
		return Applied{Item: it.PublicID, Skipped: "nothing to change here"}, nil
	}

	if status == it.Status {
		if err := b.touch(itemID, event, now); err != nil {
			return Applied{}, err
		}
		return Applied{Item: it.PublicID, Status: status, Skipped: "already in that state"}, nil
	}

	if err := b.Items.SetStatus(itemID, status, now); err != nil {
		return Applied{}, err
	}
	if err := b.touch(itemID, event, now); err != nil {
		return Applied{}, err
	}

	// The reporter hears about it, as when the team changes the status by hand.
	if err := b.RequestTellReporter(itemID, "changed status", now); err != nil {
		slog.Error("queueing a reporter notification", "error", err)
	}

	slog.Info("an item followed its issue",
		"item", it.PublicID, "issue", event.Number, "status", status)
	return Applied{Item: it.PublicID, Status: status}, nil
}

// statusFor decides what an event means for this item.
//
// Both journeys have their own vocabulary — a bug is never "planned" — so the
// same delivery means different things depending on what it is attached to.
func statusFor(kind item.Kind, event forge.Event) (item.Status, bool) {
	switch event.Kind {
	case forge.EventClosed:
		if kind == item.Bug {
			return item.StatusClosed, true
		}
		return item.StatusDone, true
	case forge.EventReopened:
		if kind == item.Bug {
			return item.StatusInForge, true
		}
		return item.StatusInProgress, true
	}
	return "", false
}

func (b Bridge) touch(itemID int64, event forge.Event, now time.Time) error {
	_, err := b.DB.Exec(`UPDATE issue_link
		SET last_state = ?, last_synced_at = ?, echo_fingerprint = ?
		WHERE item_id = ?`,
		string(event.State), now.Unix(),
		Fingerprint(forge.Issue{State: event.State, Labels: event.Labels}), itemID)
	if err != nil {
		return fmt.Errorf("updating the issue link: %w", err)
	}
	return nil
}

// seenBefore records a delivery and says whether it had already been seen.
//
// The uniqueness is the primary key doing the work, so two copies of the same
// delivery arriving at once cannot both win.
func (b Bridge) seenBefore(orgID int64, deliveryID string, now time.Time) (bool, error) {
	res, err := b.DB.Exec(`INSERT INTO webhook_delivery (org_id, delivery_id, received_at)
		VALUES (?, ?, ?) ON CONFLICT (org_id, delivery_id) DO NOTHING`,
		orgID, deliveryID, now.Unix())
	if err != nil {
		return false, fmt.Errorf("recording the delivery: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("recording the delivery: %w", err)
	}
	return affected == 0, nil
}

// PurgeDeliveries drops what can no longer be replayed usefully.
//
// The table is indexed by something the caller supplies, so it is bounded by
// time rather than left to grow: an unbounded table keyed on somebody else's
// identifier is a slow way to fill a disk.
func (b Bridge) PurgeDeliveries(now time.Time, keep time.Duration) error {
	if keep <= 0 {
		keep = 30 * 24 * time.Hour
	}
	_, err := b.DB.Exec(`DELETE FROM webhook_delivery WHERE received_at < ?`,
		now.Add(-keep).Unix())
	return err
}
