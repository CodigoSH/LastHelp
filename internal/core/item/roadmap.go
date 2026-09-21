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
		case StatusInForge:
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
