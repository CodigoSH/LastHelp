package web

import (
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"git.codigosh.com/CodigoSH/LastHelp/internal/core/authz"
	"git.codigosh.com/CodigoSH/LastHelp/internal/core/item"
)

// overview is what the panel's front page draws.
//
// Counted in the database rather than by fetching everything and counting in
// the browser: a portal that has been open for two years has a lot of reports,
// and a dashboard that gets slower the longer it has been useful is one nobody
// opens.
type overview struct {
	Organizations int            `json:"organizations"`
	Repositories  int            `json:"repositories"`
	People        int            `json:"people"`
	Pending       int            `json:"pending"`
	Kinds         map[string]int `json:"kinds"`
	Statuses      map[string]int `json:"statuses"`
	Moderation    map[string]int `json:"moderation"`
	Votes         int            `json:"votes"`
	Published     int            `json:"published"`
	// LastReport is when the most recent report arrived, in seconds. Zero means
	// none ever has — which is a state worth showing rather than a blank.
	LastReport int64 `json:"last_report"`

	// What the numbers above cannot say: whether it is getting busier, what
	// people are actually asking for, and which board the work is in.
	Series []dayCount   `json:"series"`
	Top    []itemBrief  `json:"top"`
	Recent []itemBrief  `json:"recent"`
	Boards []boardCount `json:"boards"`
}

// dayCount is one day of the activity chart.
//
// Day is the start of a UTC day, and the browser labels it in the reader's own
// locale. Bucketing by UTC means a report made late in the evening west of
// Greenwich counts towards the next day; for a thirty-day trend that is a
// rounding error, and the alternative is storing everybody's offset.
type dayCount struct {
	Day     int64 `json:"day"`
	Reports int   `json:"reports"`
	// Split by kind, because "three reports" and "three bugs" are different
	// news and the chart is the only place that can say which it was.
	Bugs  int `json:"bugs"`
	Ideas int `json:"ideas"`
}

// itemBrief is a report as a line in a list: enough to recognise it and to go
// to it, and nothing else.
type itemBrief struct {
	PublicID string `json:"public_id"`
	Title    string `json:"title"`
	Kind     string `json:"kind"`
	Status   string `json:"status"`
	// Moderation travels with it because a report that is not published is not
	// open, and a list that shows only the status says it is.
	Moderation string `json:"moderation"`
	Votes      int    `json:"votes"`
	CreatedAt  int64  `json:"created_at"`
}

// boardCount is one repository with the size of its work on it.
type boardCount struct {
	Org          string `json:"org"`
	Repo         string `json:"repo"`
	Name         string `json:"name"`
	Organization string `json:"organization"`
	Published    int    `json:"published"`
	Pending      int    `json:"pending"`
	Votes        int    `json:"votes"`
}

// handleOverview counts what this installation holds.
//
// Everything an administrator sees; for anybody else it is still their whole
// world, because the queries are not scoped — and that is why this asks for
// ManageServer rather than ViewPanel. A collaborator gets the repositories they
// triage from the screens that are already scoped to them.
func (s *Server) handleOverview(w http.ResponseWriter, r *http.Request) {
	if !s.may(w, r, authz.ManageServer, authz.Scope{}) {
		return
	}

	out := overview{
		Kinds:      map[string]int{},
		Statuses:   map[string]int{},
		Moderation: map[string]int{},
	}

	single := func(query string, into *int) bool {
		if err := s.db.QueryRow(query).Scan(into); err != nil {
			writeError(w, http.StatusInternalServerError, "error.internal")
			return false
		}
		return true
	}
	if !single(`SELECT count(*) FROM organization`, &out.Organizations) ||
		!single(`SELECT count(*) FROM repository`, &out.Repositories) ||
		!single(`SELECT count(*) FROM app_user`, &out.People) ||
		!single(`SELECT count(*) FROM item WHERE moderation_state = 'pending'`, &out.Pending) ||
		!single(`SELECT count(*) FROM item WHERE moderation_state = 'published'`, &out.Published) ||
		!single(`SELECT count(*) FROM vote`, &out.Votes) {
		return
	}

	grouped := func(query string, into map[string]int) bool {
		rows, err := s.db.Query(query)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "error.internal")
			return false
		}
		defer func() { _ = rows.Close() }()
		for rows.Next() {
			var key string
			var n int
			if err := rows.Scan(&key, &n); err != nil {
				writeError(w, http.StatusInternalServerError, "error.internal")
				return false
			}
			into[key] = n
		}
		return rows.Err() == nil
	}
	if !grouped(`SELECT kind, count(*) FROM item GROUP BY kind`, out.Kinds) ||
		!grouped(`SELECT status, count(*) FROM item WHERE moderation_state = 'published' GROUP BY status`, out.Statuses) ||
		!grouped(`SELECT moderation_state, count(*) FROM item GROUP BY moderation_state`, out.Moderation) {
		return
	}

	// COALESCE, because MAX over no rows is NULL and scanning that into an int64
	// fails — which would turn "nothing has been reported yet" into a 500.
	if err := s.db.QueryRow(`SELECT COALESCE(MAX(created_at), 0) FROM item`).Scan(&out.LastReport); err != nil {
		writeError(w, http.StatusInternalServerError, "error.internal")
		return
	}

	var err error
	if out.Series, err = s.activity(30); err != nil {
		slog.Error("reading the activity", "error", err)
		writeError(w, http.StatusInternalServerError, "error.internal")
		return
	}
	if out.Top, err = s.briefs(`kind = 'idea' AND moderation_state = 'published'`, `vote_count DESC, created_at DESC`, 5); err != nil {
		slog.Error("reading the most wanted", "error", err)
		writeError(w, http.StatusInternalServerError, "error.internal")
		return
	}
	if out.Recent, err = s.briefs(`1 = 1`, `created_at DESC`, 6); err != nil {
		slog.Error("reading the newest", "error", err)
		writeError(w, http.StatusInternalServerError, "error.internal")
		return
	}
	if out.Boards, err = s.boardCounts(8); err != nil {
		slog.Error("counting the boards", "error", err)
		writeError(w, http.StatusInternalServerError, "error.internal")
		return
	}

	writeJSON(w, http.StatusOK, out)
}

// activity is how many reports arrived on each of the last n days.
//
// Every day is in the answer, including the empty ones: a chart drawn only from
// the days something happened has no gaps in it, and therefore says a quiet
// month was a busy one.
func (s *Server) activity(days int) ([]dayCount, error) {
	const day = int64(86400)
	today := time.Now().UTC().Unix() / day * day
	from := today - int64(days-1)*day

	counted := map[int64]dayCount{}
	rows, err := s.db.Query(
		`SELECT created_at / 86400 * 86400 AS day, kind, count(*)
		   FROM item WHERE merged_into IS NULL AND created_at >= ?
		  GROUP BY day, kind`, from)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var at int64
		var kind string
		var n int
		if err := rows.Scan(&at, &kind, &n); err != nil {
			return nil, err
		}
		entry := counted[at]
		entry.Reports += n
		switch item.Kind(kind) {
		case item.Bug:
			entry.Bugs += n
		case item.Idea:
			entry.Ideas += n
		}
		counted[at] = entry
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	out := make([]dayCount, 0, days)
	for at := from; at <= today; at += day {
		entry := counted[at]
		entry.Day = at
		out = append(out, entry)
	}
	return out, nil
}

// briefs is a short list of reports, chosen and ordered by the caller.
//
// The two fragments are written here in this file and never come from a
// request: they are a `where` and an `order by` that the callers below spell
// out in full, not something a query string can reach.
func (s *Server) briefs(where, order string, limit int) ([]itemBrief, error) {
	rows, err := s.db.Query(
		`SELECT public_id, title, kind, status, moderation_state, vote_count, created_at
		   FROM item WHERE merged_into IS NULL AND (` + where + `)
		  ORDER BY ` + order + ` LIMIT ` + strconv.Itoa(limit))
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	out := []itemBrief{}
	for rows.Next() {
		var b itemBrief
		if err := rows.Scan(&b.PublicID, &b.Title, &b.Kind, &b.Status, &b.Moderation,
			&b.Votes, &b.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

// boardCounts is the work per repository, busiest first.
//
// A LEFT JOIN, so a board with nothing on it is still a row: a repository
// missing from this list because nobody has reported anything is exactly the
// one somebody needs to be reminded of.
func (s *Server) boardCounts(limit int) ([]boardCount, error) {
	rows, err := s.db.Query(
		`SELECT o.slug, r.slug, r.name, o.name,
		        COALESCE(SUM(i.moderation_state = 'published'), 0),
		        COALESCE(SUM(i.moderation_state = 'pending'), 0),
		        COALESCE(SUM(i.vote_count), 0)
		   FROM repository r
		   JOIN organization o ON o.id = r.org_id
		   LEFT JOIN item i ON i.repo_id = r.id AND i.merged_into IS NULL
		  GROUP BY r.id
		  ORDER BY 6 DESC, 5 DESC, r.name
		  LIMIT ` + strconv.Itoa(limit))
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	out := []boardCount{}
	for rows.Next() {
		var b boardCount
		if err := rows.Scan(&b.Org, &b.Repo, &b.Name, &b.Organization,
			&b.Published, &b.Pending, &b.Votes); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}
