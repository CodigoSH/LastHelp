package item

import (
	"testing"
	"time"
)

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
		{Bug, StatusInProgress, ColumnInProgress, true},
		{Idea, StatusDone, ColumnDone, true},
		{Bug, StatusClosed, ColumnDone, true},

		// Deliberately not on the roadmap.
		{Idea, StatusNew, "", false},
		{Bug, StatusNew, "", false},
		{Idea, StatusDeclined, "", false},
		{Bug, StatusDeclined, "", false},
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

// TestTheLanesAreTheColumnsAndTheDeclinedOnes.
//
// The page's lanes and the front page's columns are two lists of the same
// states, so one test ties them: wherever a status is on the roadmap it is in
// the lane of the same name, and the one lane that is not a column is exactly
// what the roadmap deliberately leaves out.
func TestTheLanesAreTheColumnsAndTheDeclinedOnes(t *testing.T) {
	for _, kind := range []Kind{Idea, Bug} {
		for _, status := range StatusesFor(kind) {
			lane, inLane := LaneFor(kind, status)
			column, onRoadmap := ColumnFor(kind, status)
			switch {
			case onRoadmap && (!inLane || string(lane) != string(column)):
				t.Errorf("%s/%s is column %q but lane %q (%v)", kind, status, column, lane, inLane)
			case !onRoadmap && status == StatusDeclined && lane != LaneDeclined:
				t.Errorf("%s/%s is %q, want the declined lane", kind, status, lane)
			case !onRoadmap && status != StatusDeclined && inLane:
				t.Errorf("%s/%s is in lane %q but is on neither the roadmap nor declined", kind, status, lane)
			}
		}
	}
	// A bug cannot be "planned" and an idea cannot be "confirmed": the vocabularies stay apart.
	if _, ok := LaneFor(Bug, StatusPlanned); ok {
		t.Error("a bug in an idea's state is in a lane")
	}
}

// TestALaneCountsWhatItListsAndSaysHowManyThereAre.
func TestALaneCountsWhatItListsAndSaysHowManyThereAre(t *testing.T) {
	s := store(t)
	made := func(kind Kind, title string, status Status, votes int, at int64) {
		t.Helper()
		it, _, err := s.Create(New{RepoID: 1, Kind: kind, Title: title}, Published, epoch)
		if err != nil {
			t.Fatal(err)
		}
		if err := s.SetStatus(it.ID, status, time.Unix(at, 0)); err != nil {
			t.Fatal(err)
		}
		if _, err := s.DB.Exec(`UPDATE item SET vote_count = ? WHERE id = ?`, votes, it.ID); err != nil {
			t.Fatal(err)
		}
	}
	made(Idea, "few votes", StatusPlanned, 3, 100)
	made(Idea, "many votes", StatusPlanned, 40, 100)
	made(Bug, "confirmed bug", StatusConfirmed, 0, 100)
	made(Idea, "shipped early", StatusDone, 5, 200)
	made(Bug, "fixed late", StatusClosed, 0, 900)
	made(Idea, "turned down", StatusDeclined, 12, 500)
	made(Bug, "could not reproduce", StatusDeclined, 0, 700)
	// Not on the page at all.
	made(Idea, "still new", StatusNew, 99, 100)
	if _, _, err := s.Create(New{RepoID: 1, Kind: Idea, Title: "waiting for a person"}, Pending, epoch); err != nil {
		t.Fatal(err)
	}

	views, err := s.RoadmapLanes(1, 100)
	if err != nil {
		t.Fatal(err)
	}
	by := map[Lane]LaneView{}
	for _, v := range views {
		by[v.Lane] = v
	}
	if len(views) != 4 {
		t.Fatalf("%d lanes, want all four even when empty", len(views))
	}

	planned := by[LanePlanned]
	if planned.Ideas != 2 || planned.Bugs != 1 || len(planned.Items) != 3 {
		t.Errorf("planned: %d ideas, %d bugs, %d listed", planned.Ideas, planned.Bugs, len(planned.Items))
	}
	// A ranking: the most wanted first.
	if planned.Items[0].Title != "many votes" {
		t.Errorf("planned starts with %q, want the most voted", planned.Items[0].Title)
	}
	// A history: the latest first, whatever the votes.
	if done := by[LaneDone]; len(done.Items) != 2 || done.Items[0].Title != "fixed late" {
		t.Errorf("done: %v", done.Items)
	}
	if declined := by[LaneDeclined]; declined.Ideas != 1 || declined.Bugs != 1 || declined.Items[0].Title != "could not reproduce" {
		t.Errorf("declined: %+v", declined)
	}
	// Every number is the length of its list when nothing is cut.
	for _, v := range views {
		if v.Total() != len(v.Items) {
			t.Errorf("lane %s says %d and lists %d", v.Lane, v.Total(), len(v.Items))
		}
	}

	// Cut short, a lane still says how many there are in all.
	short, err := s.RoadmapLanes(1, 1)
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range short {
		if v.Lane == LanePlanned && (v.Total() != 3 || len(v.Items) != 1) {
			t.Errorf("a lane cut to one says %d and lists %d", v.Total(), len(v.Items))
		}
	}
}
