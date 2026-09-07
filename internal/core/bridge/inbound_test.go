package bridge

import (
	"context"
	"testing"
	"time"

	"git.codigosh.com/CodigoSH/LastHelp/internal/core/forge"
	"git.codigosh.com/CodigoSH/LastHelp/internal/core/item"
	"git.codigosh.com/CodigoSH/LastHelp/internal/core/org"
)

func linked(t *testing.T) (Bridge, item.Item) {
	t.Helper()
	b, _, it := setup(t, org.ForgeForgejo)
	if err := b.RequestIssue(it.ID, epoch); err != nil {
		t.Fatal(err)
	}
	if _, err := b.Run(context.Background(), epoch, 10); err != nil {
		t.Fatal(err)
	}
	return b, it
}

func statusOf(t *testing.T, b Bridge, id int64) item.Status {
	t.Helper()
	fresh, err := b.Items.ByID(id)
	if err != nil {
		t.Fatal(err)
	}
	return fresh.Status
}

// TestClosingTheIssueClosesTheItem is the roadmap that nobody maintains.
func TestClosingTheIssueClosesTheItem(t *testing.T) {
	b, it := linked(t)

	applied, err := b.Apply(1, it.RepoID, forge.Event{
		Kind: forge.EventClosed, Number: 1, State: forge.Closed, DeliveryID: "d1",
	}, epoch.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if applied.Item != it.PublicID {
		t.Fatalf("applied to %q, want %q (skipped: %q)", applied.Item, it.PublicID, applied.Skipped)
	}
	if got := statusOf(t, b, it.ID); got != item.StatusClosed {
		t.Errorf("the item is %q after its issue closed", got)
	}
}

// TestABugAndAnIdeaFollowTheirOwnVocabulary.
//
// The same delivery means different things: a bug is never "done", an idea is
// never "closed". Getting this wrong stores a status that renders blank.
func TestABugAndAnIdeaFollowTheirOwnVocabulary(t *testing.T) {
	b, bug := linked(t)

	idea, _, err := b.Items.Create(item.New{RepoID: bug.RepoID, Kind: item.Idea, Title: "dark mode"},
		item.Published, epoch)
	if err != nil {
		t.Fatal(err)
	}
	if err := b.RequestIssue(idea.ID, epoch); err != nil {
		t.Fatal(err)
	}
	if _, err := b.Run(context.Background(), epoch, 10); err != nil {
		t.Fatal(err)
	}

	if _, err := b.Apply(1, bug.RepoID, forge.Event{
		Kind: forge.EventClosed, Number: 1, State: forge.Closed, DeliveryID: "a",
	}, epoch); err != nil {
		t.Fatal(err)
	}
	if _, err := b.Apply(1, idea.RepoID, forge.Event{
		Kind: forge.EventClosed, Number: 2, State: forge.Closed, DeliveryID: "b",
	}, epoch); err != nil {
		t.Fatal(err)
	}

	if got := statusOf(t, b, bug.ID); got != item.StatusClosed {
		t.Errorf("the bug is %q, want closed", got)
	}
	if got := statusOf(t, b, idea.ID); got != item.StatusDone {
		t.Errorf("the idea is %q, want done", got)
	}
}

// TestOurOwnChangeComingBackIsIgnored is the loop breaker.
//
// The sequence without it: this server writes the issue, the forge announces
// what changed, this server applies the announcement, which writes again. Two
// systems telling each other the same news for ever.
func TestOurOwnChangeComingBackIsIgnored(t *testing.T) {
	b, it := linked(t)

	// Pretend this server just pushed "closed" — which is what recording a link
	// after a write does.
	if err := b.touch(it.ID, forge.Event{State: forge.Closed}, epoch); err != nil {
		t.Fatal(err)
	}

	applied, err := b.Apply(1, it.RepoID, forge.Event{
		Kind: forge.EventClosed, Number: 1, State: forge.Closed, DeliveryID: "echo",
	}, epoch.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if applied.Skipped != "our own change coming back" {
		t.Errorf("the echo was applied rather than recognised (skipped: %q)", applied.Skipped)
	}
}

// TestADeliveryIsOnlyAppliedOnce.
//
// Every forge redelivers, and anybody who captured a delivery can replay it.
func TestADeliveryIsOnlyAppliedOnce(t *testing.T) {
	b, it := linked(t)

	event := forge.Event{Kind: forge.EventClosed, Number: 1, State: forge.Closed, DeliveryID: "same"}
	if _, err := b.Apply(1, it.RepoID, event, epoch); err != nil {
		t.Fatal(err)
	}

	// Move it back, as a person might, and replay the delivery.
	if err := b.Items.SetStatus(it.ID, item.StatusConfirmed, epoch); err != nil {
		t.Fatal(err)
	}
	applied, err := b.Apply(1, it.RepoID, event, epoch.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if applied.Skipped != "already delivered" {
		t.Errorf("a replayed delivery was applied again (skipped: %q)", applied.Skipped)
	}
	if got := statusOf(t, b, it.ID); got != item.StatusConfirmed {
		t.Errorf("the replay moved the item to %q", got)
	}
}

// TestAnIssueNobodyIsFollowingChangesNothing.
//
// Most issues on a real repository have nothing to do with this portal. A
// delivery about one of those must not move anybody's item.
func TestAnIssueNobodyIsFollowingChangesNothing(t *testing.T) {
	b, it := linked(t)

	applied, err := b.Apply(1, it.RepoID, forge.Event{
		Kind: forge.EventClosed, Number: 999, State: forge.Closed, DeliveryID: "unrelated",
	}, epoch)
	if err != nil {
		t.Fatal(err)
	}
	if applied.Item != "" {
		t.Errorf("an unrelated issue touched item %q", applied.Item)
	}
	if got := statusOf(t, b, it.ID); got != item.StatusNew {
		t.Errorf("the linked item moved to %q because of an unrelated issue", got)
	}
}

// TestReopeningBringsItBack.
func TestReopeningBringsItBack(t *testing.T) {
	b, it := linked(t)

	if _, err := b.Apply(1, it.RepoID, forge.Event{
		Kind: forge.EventClosed, Number: 1, State: forge.Closed, DeliveryID: "one",
	}, epoch); err != nil {
		t.Fatal(err)
	}
	if _, err := b.Apply(1, it.RepoID, forge.Event{
		Kind: forge.EventReopened, Number: 1, State: forge.Open, DeliveryID: "two",
	}, epoch.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}

	if got := statusOf(t, b, it.ID); got != item.StatusInForge {
		t.Errorf("a reopened bug is %q, want in_forge", got)
	}
}

// TestSomethingWeDoNotTrackIsNotAnError.
//
// A forge sends deliveries for assignments, milestones, edits. None of them
// mean anything here, and none of them are a failure.
func TestSomethingWeDoNotTrackIsNotAnError(t *testing.T) {
	b, it := linked(t)

	applied, err := b.Apply(1, it.RepoID, forge.Event{
		Kind: forge.EventIgnored, Number: 1, State: forge.Open, DeliveryID: "ignored",
	}, epoch)
	if err != nil {
		t.Fatal(err)
	}
	if applied.Item != "" || applied.Skipped == "" {
		t.Errorf("an event we do not act on came back as %+v", applied)
	}
}

// TestDeliveriesAreForgottenEventually.
//
// The table is keyed on an identifier somebody else supplies, so it is bounded
// by time rather than left to grow.
func TestDeliveriesAreForgottenEventually(t *testing.T) {
	b, it := linked(t)

	if _, err := b.Apply(1, it.RepoID, forge.Event{
		Kind: forge.EventClosed, Number: 1, State: forge.Closed, DeliveryID: "old",
	}, epoch); err != nil {
		t.Fatal(err)
	}
	if err := b.PurgeDeliveries(epoch.Add(60*24*time.Hour), 30*24*time.Hour); err != nil {
		t.Fatal(err)
	}

	var left int
	if err := b.DB.QueryRow(`SELECT count(*) FROM webhook_delivery`).Scan(&left); err != nil {
		t.Fatal(err)
	}
	if left != 0 {
		t.Errorf("%d deliveries survived the purge", left)
	}
}
