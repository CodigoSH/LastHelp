package item

import "testing"

// TestBothJourneysLandOnTheSameThreeColumns.
//
// A reader does not know or care that a bug and an idea are tracked
// differently. A confirmed bug and a planned idea are the same news to them:
// somebody agreed it is real and it is coming.
func TestBothJourneysLandOnTheSameThreeColumns(t *testing.T) {
	cases := []struct {
		kind   Kind
		status Status
		want   Column
		on     bool
	}{
		{Idea, StatusPlanned, ColumnPlanned, true},
		{Bug, StatusConfirmed, ColumnPlanned, true},
		{Idea, StatusInProgress, ColumnInProgress, true},
		{Bug, StatusInForge, ColumnInProgress, true},
		{Idea, StatusDone, ColumnDone, true},
		{Bug, StatusClosed, ColumnDone, true},

		// Deliberately not on the roadmap.
		{Idea, StatusNew, "", false},
		{Bug, StatusNew, "", false},
		{Idea, StatusDeclined, "", false},
		{Bug, StatusNotReproducible, "", false},
	}

	for _, c := range cases {
		got, on := ColumnFor(c.kind, c.status)
		if got != c.want || on != c.on {
			t.Errorf("%s/%s → %q,%v; want %q,%v", c.kind, c.status, got, on, c.want, c.on)
		}
	}
}

// TestARoadmapIsWhatIsGoingToHappen.
//
// Not a graveyard of refusals, and not the unread queue. Both are easy to end
// up publishing by accident — the first by treating "declined" as a status like
// any other, the second by forgetting that the default listing is published
// only.
func TestARoadmapIsWhatIsGoingToHappen(t *testing.T) {
	s := store(t)

	published := func(kind Kind, title string, status Status) {
		t.Helper()
		it, _, err := s.Create(New{RepoID: 1, Kind: kind, Title: title}, Published, epoch)
		if err != nil {
			t.Fatal(err)
		}
		if status != StatusNew {
			if err := s.SetStatus(it.ID, status, epoch); err != nil {
				t.Fatal(err)
			}
		}
	}

	published(Idea, "dark mode", StatusPlanned)
	published(Bug, "crash on import", StatusConfirmed)
	published(Idea, "export", StatusInProgress)
	published(Idea, "shipped thing", StatusDone)
	published(Idea, "not happening", StatusDeclined)
	published(Idea, "nobody has read this", StatusNew)

	// Something waiting for a human must never appear.
	if _, _, err := s.Create(New{RepoID: 1, Kind: Idea, Title: "still in the queue"}, Pending, epoch); err != nil {
		t.Fatal(err)
	}

	board, err := s.Roadmap(1, 25)
	if err != nil {
		t.Fatal(err)
	}

	if got := len(board[ColumnPlanned]); got != 2 {
		t.Errorf("planned has %d, want the idea and the confirmed bug", got)
	}
	if got := len(board[ColumnInProgress]); got != 1 {
		t.Errorf("in progress has %d", got)
	}
	if got := len(board[ColumnDone]); got != 1 {
		t.Errorf("done has %d", got)
	}

	for column, items := range board {
		for _, it := range items {
			if it.Title == "not happening" {
				t.Errorf("a declined item is on the roadmap, in %s", column)
			}
			if it.Title == "nobody has read this" {
				t.Errorf("something nobody has triaged is on the roadmap, in %s", column)
			}
			if it.Title == "still in the queue" {
				t.Errorf("something waiting for moderation is on the roadmap, in %s", column)
			}
		}
	}
}

func TestEveryColumnIsPresentEvenWhenEmpty(t *testing.T) {
	s := store(t)

	board, err := s.Roadmap(1, 25)
	if err != nil {
		t.Fatal(err)
	}
	for _, column := range Columns {
		if _, ok := board[column]; !ok {
			t.Errorf("column %q is missing from an empty roadmap; the page would draw two columns "+
				"and the reader would wonder which one they are looking at", column)
		}
	}
}
