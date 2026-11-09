package item

// Column is one of the three a public roadmap has.
//
// Three, and no more. A roadmap with nine columns is a project management tool,
// and the person reading it wants to know one thing: is the thing I care about
// coming, being worked on, or done.
type Column string

const (
	ColumnPlanned    Column = "planned"
	ColumnInProgress Column = "in_progress"
	ColumnDone       Column = "done"
)

// Columns is the order they are drawn in.
var Columns = []Column{ColumnPlanned, ColumnInProgress, ColumnDone}

// ColumnFor maps a status onto the roadmap, or says it does not belong there.
//
// Both journeys land on the same three columns, which is what makes one board
// readable to somebody who does not know or care that a bug and an idea are
// tracked differently. A confirmed bug and a planned idea are, to a reader, the
// same news: somebody has agreed it is real and it is coming.
//
// What is deliberately absent: anything still new, and anything declined or
// not reproducible. A roadmap is what is going to happen — putting refusals on
// it turns it into a graveyard, and putting the unread queue on it publishes
// things nobody has looked at.
func ColumnFor(kind Kind, status Status) (Column, bool) {
	switch kind {
	case Idea:
		switch status {
		case StatusPlanned:
			return ColumnPlanned, true
		case StatusInProgress:
			return ColumnInProgress, true
		case StatusDone:
			return ColumnDone, true
		}
	case Bug:
		switch status {
		case StatusConfirmed:
			return ColumnPlanned, true
		case StatusInProgress:
			return ColumnInProgress, true
		case StatusClosed:
			return ColumnDone, true
		}
	}
	return "", false
}

// Roadmap returns the published items of one repository, grouped.
//
// It is one query and one pass, not three queries: the columns have to be a
// consistent snapshot of the same instant, or an item that moves while the page
// is being built appears twice or not at all.
func (s Store) Roadmap(repoID int64, limitPerColumn int) (map[Column][]Item, error) {
	return s.RoadmapAcross([]int64{repoID}, limitPerColumn)
}

// RoadmapAcross is the same for several boards at once, which is what the front
// page needs: a roadmap module there can be pointed at one board, at an
// organisation, or at everything, and three separate queries would be three
// snapshots of three different instants.
func (s Store) RoadmapAcross(repoIDs []int64, limitPerColumn int) (map[Column][]Item, error) {
	if limitPerColumn <= 0 {
		limitPerColumn = 25
	}
	if len(repoIDs) == 0 {
		out := map[Column][]Item{}
		for _, column := range Columns {
			out[column] = []Item{}
		}
		return out, nil
	}

	all, err := s.List(Filter{RepoIDs: repoIDs, SortByVote: true, Limit: 200})
	if err != nil {
		return nil, err
	}

	out := map[Column][]Item{}
	for _, column := range Columns {
		out[column] = []Item{}
	}
	for _, it := range all {
		column, ok := ColumnFor(it.Kind, it.Status)
		if !ok {
			continue
		}
		if len(out[column]) >= limitPerColumn {
			continue
		}
		out[column] = append(out[column], it)
	}
	return out, nil
}

// Lane is one stretch of the roadmap page: what is under way, what is coming,
// what was delivered, and what the team decided not to do.
//
// The three first are the roadmap's columns and agree with ColumnFor, which a
// test checks. The fourth is the page's alone: the front page's roadmap module
// stays about what is going to happen, and a refusal is not that. It is on the
// page because a report with forty votes that was turned down is news to the
// forty people who voted, and the board already lists it under Closed — the
// roadmap hiding what the board shows is the one place a person looking for
// "is mine coming?" would find no answer.
type Lane string

const (
	LaneInProgress Lane = "in_progress"
	LanePlanned    Lane = "planned"
	LaneDone       Lane = "done"
	LaneDeclined   Lane = "declined"
)

// Lanes is the order the page asks for them in.
var Lanes = []Lane{LaneInProgress, LanePlanned, LaneDone, LaneDeclined}

// LaneStatuses are the states that belong to a lane, in both vocabularies: a
// confirmed bug and a planned idea are, to a reader, the same news.
func LaneStatuses(l Lane) []Status {
	switch l {
	case LaneInProgress:
		return []Status{StatusInProgress}
	case LanePlanned:
		return []Status{StatusPlanned, StatusConfirmed}
	case LaneDone:
		return []Status{StatusDone, StatusClosed}
	case LaneDeclined:
		return []Status{StatusDeclined}
	}
	return nil
}

// LaneFor says which lane a state is in, or that it is in none: what is still
// new belongs to the queue and is not on a public page.
func LaneFor(kind Kind, status Status) (Lane, bool) {
	for _, l := range Lanes {
		for _, st := range LaneStatuses(l) {
			if st == status && validStatus(kind, status) {
				return l, true
			}
		}
	}
	return "", false
}

// Finished is whether a lane is ordered by when it ended and not by votes: what
// was delivered or turned down reads as a history, what is open as a ranking.
func (l Lane) Finished() bool { return l == LaneDone || l == LaneDeclined }

// LaneView is one lane as the page draws it: how many of each kind there are
// in all, and the first of them.
type LaneView struct {
	Lane  Lane
	Ideas int
	Bugs  int
	Items []Item
}

// Total is every item in the lane, which is more than Items holds once the lane
// is longer than the page asks for.
func (v LaneView) Total() int { return v.Ideas + v.Bugs }

// RoadmapLanes is the whole page in one pass: each lane's counts, from the same
// conditions its list is read with, so a number and the list it heads cannot
// disagree. perLane bounds each list, not the page.
func (s Store) RoadmapLanes(repoID int64, perLane int) ([]LaneView, error) {
	if perLane <= 0 || perLane > 200 {
		perLane = 100
	}
	out := make([]LaneView, 0, len(Lanes))
	for _, lane := range Lanes {
		f := Filter{RepoID: repoID, Statuses: LaneStatuses(lane), Limit: perLane}
		if lane.Finished() {
			f.SortByClosed = true
		} else {
			f.SortByVote = true
		}
		view := LaneView{Lane: lane}
		var err error
		for _, kind := range []Kind{Idea, Bug} {
			counted := f
			counted.Kind = kind
			n, err := s.Tally(counted)
			if err != nil {
				return nil, err
			}
			if kind == Idea {
				view.Ideas = n
			} else {
				view.Bugs = n
			}
		}
		if view.Items, err = s.List(f); err != nil {
			return nil, err
		}
		out = append(out, view)
	}
	return out, nil
}
