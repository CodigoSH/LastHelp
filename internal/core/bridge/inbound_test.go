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

// TestTheReporterIsToldAndThenForgotten.
//
// SECURITY.md says the reporter's address is deleted when the item closes, and
// says of that sentence that it is a test rather than a promise. It was not:
// the code existed and nothing exercised it, which is how a document ends up
// describing a behaviour nobody has ever seen run.
func TestTheReporterIsToldAndThenForgotten(t *testing.T) {
	b, _, _ := setup(t, org.ForgeNone)

	var told []string
	b.Tell = func(_ context.Context, sealed []byte, title, reason, _ string) error {
		told = append(told, string(sealed)+"|"+title+"|"+reason)
		return nil
	}

	// Somebody who asked to be notified. The address arrives already sealed —
	// the domain never sees it in the clear — so for this test the "sealed"
	// bytes stand in for whatever the box produced.
	it, _, err := b.Items.Create(item.New{
		RepoID: 1, Kind: item.Bug, Title: "crash on import",
		EmailSealed: []byte("sealed:somebody@example.com"),
	}, item.Published, epoch)
	if err != nil {
		t.Fatal(err)
	}

	// Something that is not the end: they are told, and the address stays,
	// because there is more to tell them about.
	if err := b.Items.SetStatus(it.ID, item.StatusConfirmed, epoch); err != nil {
		t.Fatal(err)
	}
	if err := b.RequestTellReporter(it.ID, "changed status", epoch); err != nil {
		t.Fatal(err)
	}
	if _, err := b.Run(context.Background(), epoch, 10); err != nil {
		t.Fatal(err)
	}

	if len(told) != 1 {
		t.Fatalf("the reporter was told %d times, want 1", len(told))
	}
	sealed, err := b.Items.SealedEmail(it.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(sealed) == 0 {
		t.Fatal("the address was forgotten while the item is still open, so nothing can tell " +
			"them about the next change")
	}

	// And now the end.
	if err := b.Items.SetStatus(it.ID, item.StatusClosed, epoch.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := b.RequestTellReporter(it.ID, "was closed", epoch.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if _, err := b.Run(context.Background(), epoch.Add(time.Hour), 10); err != nil {
		t.Fatal(err)
	}

	if len(told) != 2 {
		t.Fatalf("the reporter was told %d times in total, want 2 — they should hear about "+
			"the closure before being forgotten", len(told))
	}

	sealed, err = b.Items.SealedEmail(it.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(sealed) != 0 {
		t.Errorf("the address survived the item closing: %q\n\n"+
			"It was collected for one purpose and that purpose is over. SECURITY.md says it "+
			"is deleted, and a document describing a behaviour nobody exercises is a document "+
			"that is right until the day it is not.", sealed)
	}
}

// TestNobodyWhoLeftNoAddressIsChased.
//
// The common case, and the one that must cost nothing: no address, no relay,
// nothing to do, and no error either.
func TestNobodyWhoLeftNoAddressIsChased(t *testing.T) {
	b, _, it := setup(t, org.ForgeNone)

	called := 0
	b.Tell = func(context.Context, []byte, string, string, string) error {
		called++
		return nil
	}

	if err := b.RequestTellReporter(it.ID, "is now public", epoch); err != nil {
		t.Fatal(err)
	}
	done, err := b.Run(context.Background(), epoch, 10)
	if err != nil {
		t.Fatalf("telling nobody failed: %v", err)
	}
	if done != 1 {
		t.Errorf("the job did not finish; a reporter who left no address must not be a stuck job")
	}
	if called != 0 {
		t.Errorf("something was sent to nobody %d times", called)
	}
}
