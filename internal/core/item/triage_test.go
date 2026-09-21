package item

import (
	"strings"
	"testing"
	"time"
)

// TestWhatIsFinishedComesBackNewestFirst.
//
// The front page can show what was recently put right, which is the strongest
// thing it can say to somebody deciding whether reporting here is worth their
// time. Three things have to hold for that to be true rather than decorative:
// it is ordered by when the work finished and not by when it arrived, it takes
// the two vocabularies that mean finished — an idea is done, a bug is closed —
// and it leaves out the two that mean refused.
func TestWhatIsFinishedComesBackNewestFirst(t *testing.T) {
	s := store(t)

	// Reported in this order, finished in the opposite one, so an ordering that
	// quietly fell back to arrival time would come back exactly reversed.
	first, _ := report(t, s, Idea, "dark mode", "the white is bright at night")
	second, _ := report(t, s, Bug, "search ignores accents", "nothing comes back")
	third, _ := report(t, s, Idea, "keyboard shortcuts", "for the queue")
	refused, _ := report(t, s, Idea, "a themed mascot", "with a hat")
	cannot, _ := report(t, s, Bug, "crashes sometimes", "not sure when")
	open, _ := report(t, s, Idea, "an export button", "as csv")

	for _, it := range []Item{first, second, third, refused, cannot, open} {
		if err := s.setModeration(it.ID, Published, epoch); err != nil {
			t.Fatal(err)
		}
	}

	finish := func(it Item, status Status, at time.Time) {
		t.Helper()
		if err := s.SetStatus(it.ID, status, at); err != nil {
			t.Fatal(err)
		}
	}
	finish(third, StatusDone, epoch.Add(1*time.Hour))
	finish(second, StatusClosed, epoch.Add(2*time.Hour))
	finish(first, StatusDone, epoch.Add(3*time.Hour))
	finish(refused, StatusDeclined, epoch.Add(4*time.Hour))
	finish(cannot, StatusNotReproducible, epoch.Add(5*time.Hour))
	finish(open, StatusPlanned, epoch.Add(6*time.Hour))

	got, err := s.List(Filter{
		RepoID:       1,
		Statuses:     []Status{StatusDone, StatusClosed},
		SortByClosed: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	var titles []string
	for _, it := range got {
		titles = append(titles, it.Title)
	}
	want := []string{"dark mode", "search ignores accents", "keyboard shortcuts"}
	if strings.Join(titles, " | ") != strings.Join(want, " | ") {
		t.Errorf("finished reports came back as %v, want %v", titles, want)
	}
	if len(got) > 0 && got[0].ClosedAt.IsZero() {
		t.Error("a finished report came back without a time it finished")
	}
}

// TestCountingWhatCameInAndWhereItGot.
//
// Three numbers a front page can show, and the one thing that would make them
// lie: counting something refused as either open or done. A report that was
// declined is not waiting on anybody and was not put right, so it is in
// "received" and in neither of the others.
func TestCountingWhatCameInAndWhereItGot(t *testing.T) {
	s := store(t)

	open1, _ := report(t, s, Idea, "dark mode", "the white is bright")
	open2, _ := report(t, s, Bug, "search ignores accents", "nothing comes back")
	done1, _ := report(t, s, Idea, "keyboard shortcuts", "for the queue")
	done2, _ := report(t, s, Bug, "the sidebar forgets", "on every reload")
	refused, _ := report(t, s, Idea, "a themed mascot", "with a hat")
	waiting, _ := report(t, s, Idea, "an export button", "as csv")

	// Everything but the last one is let through. The last stays in the queue,
	// so it must not be counted as received: nobody has read it.
	for _, it := range []Item{open1, open2, done1, done2, refused} {
		if err := s.setModeration(it.ID, Published, epoch); err != nil {
			t.Fatal(err)
		}
	}
	_ = waiting

	for _, at := range []struct {
		it     Item
		status Status
	}{
		{open1, StatusPlanned},
		{open2, StatusConfirmed},
		{done1, StatusDone},
		{done2, StatusClosed},
		{refused, StatusDeclined},
	} {
		if err := s.SetStatus(at.it.ID, at.status, epoch); err != nil {
			t.Fatal(err)
		}
	}

	got, err := s.Count([]int64{1})
	if err != nil {
		t.Fatal(err)
	}
	want := Counts{Received: 5, Open: 2, Done: 2}
	if got != want {
		t.Errorf("counted %+v, want %+v", got, want)
	}

	// No boards is not an error and not a guess: it is nought of everything.
	if empty, err := s.Count(nil); err != nil || empty != (Counts{}) {
		t.Errorf("counting nothing gave %+v, %v", empty, err)
	}
}
