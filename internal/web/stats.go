package web

import (
	"database/sql"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"git.codigosh.com/CodigoSH/LastHelp/internal/core/authz"
	"git.codigosh.com/CodigoSH/LastHelp/internal/core/item"
	"git.codigosh.com/CodigoSH/LastHelp/internal/core/org"
	"git.codigosh.com/CodigoSH/LastHelp/internal/platform/notify"
	"git.codigosh.com/CodigoSH/LastHelp/internal/web/middleware"
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
	// StatusesByKind is the same count split by kind, because a bug and an idea
	// do not share a vocabulary: "fixed" is not something an idea becomes, and
	// one list of both says nothing about either. Statuses stays for a browser
	// that has not reloaded since the split.
	StatusesByKind map[string]map[string]int `json:"statuses_by_kind"`
	Moderation     map[string]int            `json:"moderation"`
	Votes          int                       `json:"votes"`
	Published      int                       `json:"published"`
	// LastReport is when the most recent report arrived, in seconds. Zero means
	// none ever has — which is a state worth showing rather than a blank.
	LastReport int64 `json:"last_report"`

	// Server says the numbers are the whole server's, which only its
	// administrator is shown; otherwise they are the reader's boards, and the
	// server-wide cards — people — are not drawn.
	Server bool `json:"server"`

	// What a team looking after the queue needs and a count cannot say. The
	// waiting figures are the ones the menu's badge shows — one place decides
	// what waits for this person — and the oldest of them says for how long.
	WaitingReports  int   `json:"waiting_reports"`
	WaitingMessages int   `json:"waiting_messages"`
	WaitingOldest   int64 `json:"waiting_oldest"`
	// Unanswered is what is published, not finished, and has no word from the
	// team: the people who wrote and heard nothing. How long the longest has
	// waited, and how much the most wanted of them is.
	Unanswered          int   `json:"unanswered"`
	UnansweredOldest    int64 `json:"unanswered_oldest"`
	UnansweredMostVotes int   `json:"unanswered_most_votes"`
	// How the last thirty days of reports were answered: how many were, out of
	// how many were published, and how long the middle one waited, in seconds.
	Answerable int   `json:"answerable"`
	Answered   int   `json:"answered"`
	FirstReply int64 `json:"first_reply"`
	// NotOnForge is confirmed bugs on a repository whose organisation has a
	// forge and that nobody has been given an issue for.
	NotOnForge int `json:"not_on_forge"`
	// FailedJobs is what the server gave up on — a notification, an issue — and
	// is for the administrator to look at, so only the administrator is told.
	FailedJobs int `json:"failed_jobs"`

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
	// The same summary for everybody with a role, over what they may see: the
	// whole server for its administrator, the boards they work on for anybody
	// else. It used to be the administrator's alone, and a team member's
	// overview was a list of where they had roles — which said what they were
	// and not how their boards were doing.
	actor, _ := middleware.ActorFrom(r)
	if !s.may(w, r, authz.ViewPanel, authz.Scope{}) {
		return
	}
	server := actor.Can(authz.ManageServer, authz.Scope{})

	// Which boards, as a fragment over item.repo_id. Built from integers this
	// server read from its own database, never from the request.
	scope, orgs := "1 = 1", map[int64]bool{}
	var repoIDs []int64
	if !server {
		boards, err := s.scopedBoards(actor)
		if err != nil {
			slog.Error("listing the boards somebody may triage", "error", err)
			writeError(w, http.StatusInternalServerError, "error.internal")
			return
		}
		ids := []string{"0"}
		for _, b := range boards {
			ids = append(ids, strconv.FormatInt(b.repo.ID, 10))
			repoIDs = append(repoIDs, b.repo.ID)
			orgs[b.org.ID] = true
		}
		scope = "repo_id IN (" + strings.Join(ids, ",") + ")"
	}

	if server {
		all, err := s.db.Query(`SELECT id FROM repository`)
		if err != nil {
			slog.Error("listing the repositories", "error", err)
			writeError(w, http.StatusInternalServerError, "error.internal")
			return
		}
		for all.Next() {
			var id int64
			if err := all.Scan(&id); err == nil {
				repoIDs = append(repoIDs, id)
			}
		}
		_ = all.Close()
	}

	out := overview{
		Kinds:    map[string]int{},
		Statuses: map[string]int{},
		StatusesByKind: map[string]map[string]int{
			string(item.Idea): {},
			string(item.Bug):  {},
		},
		Moderation: map[string]int{},
		Server:     server,
	}

	single := func(query string, into *int) bool {
		if err := s.db.QueryRow(query).Scan(into); err != nil {
			writeError(w, http.StatusInternalServerError, "error.internal")
			return false
		}
		return true
	}
	if server {
		if !single(`SELECT count(*) FROM organization`, &out.Organizations) ||
			!single(`SELECT count(*) FROM repository`, &out.Repositories) ||
			!single(`SELECT count(*) FROM app_user`, &out.People) {
			return
		}
	} else {
		out.Organizations = len(orgs)
		if !single(`SELECT count(*) FROM repository WHERE `+strings.Replace(scope, "repo_id", "id", 1), &out.Repositories) {
			return
		}
	}
	if !single(`SELECT count(*) FROM item WHERE moderation_state = 'pending' AND `+scope, &out.Pending) ||
		!single(`SELECT count(*) FROM item WHERE moderation_state = 'published' AND `+scope, &out.Published) ||
		!single(`SELECT count(*) FROM vote JOIN item ON item.id = vote.item_id WHERE `+scope, &out.Votes) {
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
	if !grouped(`SELECT kind, count(*) FROM item WHERE `+scope+` GROUP BY kind`, out.Kinds) ||
		!grouped(`SELECT status, count(*) FROM item WHERE moderation_state = 'published' AND `+scope+` GROUP BY status`, out.Statuses) ||
		!grouped(`SELECT moderation_state, count(*) FROM item WHERE `+scope+` GROUP BY moderation_state`, out.Moderation) {
		return
	}

	split, err := s.db.Query(`SELECT kind, status, count(*) FROM item
		 WHERE moderation_state = 'published' AND ` + scope + ` GROUP BY kind, status`)
	if err != nil {
		slog.Error("counting the statuses by kind", "error", err)
		writeError(w, http.StatusInternalServerError, "error.internal")
		return
	}
	defer func() { _ = split.Close() }()
	for split.Next() {
		var kind, state string
		var n int
		if err := split.Scan(&kind, &state, &n); err != nil {
			writeError(w, http.StatusInternalServerError, "error.internal")
			return
		}
		if out.StatusesByKind[kind] == nil {
			out.StatusesByKind[kind] = map[string]int{}
		}
		out.StatusesByKind[kind][state] = n
	}
	if err := split.Err(); err != nil {
		writeError(w, http.StatusInternalServerError, "error.internal")
		return
	}

	// COALESCE, because MAX over no rows is NULL and scanning that into an int64
	// fails — which would turn "nothing has been reported yet" into a 500.
	if err := s.db.QueryRow(`SELECT COALESCE(MAX(created_at), 0) FROM item WHERE ` + scope).Scan(&out.LastReport); err != nil {
		writeError(w, http.StatusInternalServerError, "error.internal")
		return
	}

	if out.Series, err = s.activity(overviewDays(r), scope); err != nil {
		slog.Error("reading the activity", "error", err)
		writeError(w, http.StatusInternalServerError, "error.internal")
		return
	}
	if err := s.supportFigures(&out, actor, repoIDs, server); err != nil {
		slog.Error("reading what the team needs to know", "error", err)
		writeError(w, http.StatusInternalServerError, "error.internal")
		return
	}
	if out.Top, err = s.briefs(`kind = 'idea' AND moderation_state = 'published' AND `+scope, `vote_count DESC, created_at DESC`, 5); err != nil {
		slog.Error("reading the most wanted", "error", err)
		writeError(w, http.StatusInternalServerError, "error.internal")
		return
	}
	if out.Recent, err = s.briefs(scope, `created_at DESC`, 6); err != nil {
		slog.Error("reading the newest", "error", err)
		writeError(w, http.StatusInternalServerError, "error.internal")
		return
	}
	if out.Boards, err = s.boardCounts(8, strings.Replace(scope, "repo_id", "r.id", 1)); err != nil {
		slog.Error("counting the boards", "error", err)
		writeError(w, http.StatusInternalServerError, "error.internal")
		return
	}

	writeJSON(w, http.StatusOK, out)
}

// supportFigures fills in what a team answering people needs. Each of them is
// read by the item store from the same conditions the listings use, so a
// number here and the list it leads to cannot disagree about what is counted.
func (s *Server) supportFigures(out *overview, actor authz.Actor, repoIDs []int64, server bool) error {
	waiting := s.pendingFor(actor)
	out.WaitingReports, out.WaitingMessages = waiting["reports"], waiting["messages"]

	var err error
	if out.WaitingOldest, err = s.items().OldestWaiting(repoIDs); err != nil {
		return err
	}

	facts, err := s.items().Facts(item.Filter{
		RepoIDs: repoIDs, Moderation: item.Published, Statuses: item.OpenStatuses(), Unanswered: true,
	})
	if err != nil {
		return err
	}
	out.Unanswered, out.UnansweredOldest, out.UnansweredMostVotes = facts.Count, facts.Oldest, facts.MostVoted

	replies, err := s.items().FirstReplies(repoIDs, s.now().Add(-30*24*time.Hour).Unix())
	if err != nil {
		return err
	}
	out.Answerable, out.Answered, out.FirstReply = replies.Published, replies.Answered, replies.Median

	if out.NotOnForge, err = s.items().NotOnForge(repoIDs, string(org.ForgeNone)); err != nil {
		return err
	}
	if server {
		failed, err := s.outbox().Abandoned()
		if err != nil {
			return err
		}
		out.FailedJobs = len(failed)
	}
	return nil
}

// overviewDays is how far back the chart goes. Three ranges and no others: the
// number comes from the request, and a query string that could ask for ten
// thousand days is a query that builds ten thousand rows.
func overviewDays(r *http.Request) int {
	switch r.URL.Query().Get("days") {
	case "7":
		return 7
	case "90":
		return 90
	}
	return 30
}

// activity is how many reports arrived on each of the last n days.
//
// Every day is in the answer, including the empty ones: a chart drawn only from
// the days something happened has no gaps in it, and therefore says a quiet
// month was a busy one.
func (s *Server) activity(days int, scope string) ([]dayCount, error) {
	const day = int64(86400)
	today := time.Now().UTC().Unix() / day * day
	from := today - int64(days-1)*day

	counted := map[int64]dayCount{}
	rows, err := s.db.Query(
		`SELECT created_at / 86400 * 86400 AS day, kind, count(*)
		   FROM item WHERE merged_into IS NULL AND created_at >= ? AND `+scope+`
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
func (s *Server) boardCounts(limit int, scope string) ([]boardCount, error) {
	rows, err := s.db.Query(
		`SELECT o.slug, r.slug, r.name, o.name,
		        COALESCE(SUM(i.moderation_state = 'published'), 0),
		        COALESCE(SUM(i.moderation_state = 'pending'), 0),
		        COALESCE(SUM(i.vote_count), 0)
		   FROM repository r
		   JOIN organization o ON o.id = r.org_id
		   LEFT JOIN item i ON i.repo_id = r.id AND i.merged_into IS NULL
		  WHERE ` + scope + `
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

// boardDetail is one repository in the detail behind the overview: what the
// tile on the page could not hold.
type boardDetail struct {
	Org          string `json:"org"`
	Repo         string `json:"repo"`
	Name         string `json:"name"`
	Organization string `json:"organization"`

	Published       int   `json:"published"`
	Votes           int   `json:"votes"`
	WaitingReports  int   `json:"waiting_reports"`
	WaitingMessages int   `json:"waiting_messages"`
	WaitingOldest   int64 `json:"waiting_oldest"`

	Unanswered       int   `json:"unanswered"`
	UnansweredOldest int64 `json:"unanswered_oldest"`
	Answerable       int   `json:"answerable"`
	Answered         int   `json:"answered"`
	FirstReply       int64 `json:"first_reply"`

	// The last thirty days of what arrived, by kind.
	Ideas int `json:"ideas"`
	Bugs  int `json:"bugs"`

	StatusesByKind map[string]map[string]int `json:"statuses_by_kind"`

	// Where the work goes from here: the forge of its organisation, how many of
	// its reports are linked to an issue, and what its team is told through.
	// The kind of channel and never its address or its token.
	Forge   string `json:"forge"`
	Linked  int    `json:"linked"`
	Channel string `json:"channel"`
}

// handleOverviewBoards is the detail the overview's "see more" opens: one row
// per repository this account may triage, with the same figures the page shows
// in total. Each is read the way the total is — by the item store, from the
// listing's own conditions — so the rows add up to the number they open from.
func (s *Server) handleOverviewBoards(w http.ResponseWriter, r *http.Request) {
	actor, ok := middleware.ActorFrom(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "error.unauthorized")
		return
	}
	if !s.may(w, r, authz.ViewPanel, authz.Scope{}) {
		return
	}

	boards, err := s.scopedBoards(actor)
	if err != nil {
		slog.Error("listing the boards somebody may triage", "error", err)
		writeError(w, http.StatusInternalServerError, "error.internal")
		return
	}
	// Bounded: a detail of fifty repositories is more than a person reads, and
	// each costs a few queries.
	if len(boards) > 50 {
		boards = boards[:50]
	}

	out := make([]boardDetail, 0, len(boards))
	if len(boards) == 0 {
		writeJSON(w, http.StatusOK, map[string]any{"boards": out})
		return
	}
	ids := make([]int64, 0, len(boards))
	index := map[int64]int{}
	for i, b := range boards {
		ids = append(ids, b.repo.ID)
		index[b.repo.ID] = i
		out = append(out, boardDetail{
			Org: b.org.Slug, Repo: b.repo.Slug, Name: b.repo.Name, Organization: b.org.Name,
			StatusesByKind: map[string]map[string]int{string(item.Idea): {}, string(item.Bug): {}},
			Forge:          string(b.org.Forge),
			Channel:        string(notify.Parse(b.repo.NotifyConfig).Kind),
		})
	}
	fail := func(what string, err error) {
		slog.Error("reading the detail of the boards: "+what, "error", err)
		writeError(w, http.StatusInternalServerError, "error.internal")
	}

	// What waits, per board: the same count the badge uses.
	pending, err := s.items().PendingByBoard()
	if err != nil {
		fail("what waits", err)
		return
	}
	for _, p := range pending {
		if i, found := index[p.RepoID]; found {
			out[i].WaitingReports, out[i].WaitingMessages = p.Reports, p.Messages
		}
	}

	since := s.now().Add(-30 * 24 * time.Hour).Unix()
	for i, id := range ids {
		one := []int64{id}
		if out[i].WaitingOldest, err = s.items().OldestWaiting(one); err != nil {
			fail("the oldest wait", err)
			return
		}
		facts, err := s.items().Facts(item.Filter{RepoIDs: one, Moderation: item.Published, Statuses: item.OpenStatuses(), Unanswered: true})
		if err != nil {
			fail("the unanswered", err)
			return
		}
		out[i].Unanswered, out[i].UnansweredOldest = facts.Count, facts.Oldest
		replies, err := s.items().FirstReplies(one, since)
		if err != nil {
			fail("the replies", err)
			return
		}
		out[i].Answerable, out[i].Answered, out[i].FirstReply = replies.Published, replies.Answered, replies.Median
	}

	in := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")
	args := make([]any, 0, len(ids)+1)
	for _, id := range ids {
		args = append(args, id)
	}

	// each runs one grouped query and hands its rows over, and says so when the
	// reading failed halfway: a half-read answer is a number that is quietly
	// too small.
	each := func(query string, args []any, row func(*sql.Rows) error) error {
		rows, err := s.db.Query(query, args...)
		if err != nil {
			return err
		}
		defer func() { _ = rows.Close() }()
		for rows.Next() {
			if err := row(rows); err != nil {
				return err
			}
		}
		return rows.Err()
	}

	// Published and votes, the two kinds' arrivals this month, the statuses by
	// kind and the issues linked: each one grouped query for every board.
	if err := each(`SELECT repo_id, COALESCE(SUM(moderation_state = 'published'), 0), COALESCE(SUM(vote_count), 0)
		FROM item WHERE merged_into IS NULL AND repo_id IN (`+in+`) GROUP BY repo_id`, args, func(rows *sql.Rows) error {
		var id int64
		var published, votes int
		if err := rows.Scan(&id, &published, &votes); err != nil {
			return err
		}
		out[index[id]].Published, out[index[id]].Votes = published, votes
		return nil
	}); err != nil {
		fail("published and votes", err)
		return
	}

	if err := each(`SELECT repo_id, kind, count(*) FROM item
		WHERE merged_into IS NULL AND created_at >= ? AND repo_id IN (`+in+`) GROUP BY repo_id, kind`,
		append([]any{since}, args...), func(rows *sql.Rows) error {
			var id int64
			var kind string
			var n int
			if err := rows.Scan(&id, &kind, &n); err != nil {
				return err
			}
			switch item.Kind(kind) {
			case item.Idea:
				out[index[id]].Ideas = n
			case item.Bug:
				out[index[id]].Bugs = n
			}
			return nil
		}); err != nil {
		fail("what arrived", err)
		return
	}

	if err := each(`SELECT repo_id, kind, status, count(*) FROM item
		WHERE merged_into IS NULL AND moderation_state = 'published' AND repo_id IN (`+in+`)
		GROUP BY repo_id, kind, status`, args, func(rows *sql.Rows) error {
		var id int64
		var kind, state string
		var n int
		if err := rows.Scan(&id, &kind, &state, &n); err != nil {
			return err
		}
		if out[index[id]].StatusesByKind[kind] == nil {
			out[index[id]].StatusesByKind[kind] = map[string]int{}
		}
		out[index[id]].StatusesByKind[kind][state] = n
		return nil
	}); err != nil {
		fail("the statuses", err)
		return
	}

	if err := each(`SELECT repo_id, count(*) FROM issue_link WHERE repo_id IN (`+in+`) GROUP BY repo_id`, args, func(rows *sql.Rows) error {
		var id int64
		var n int
		if err := rows.Scan(&id, &n); err != nil {
			return err
		}
		out[index[id]].Linked = n
		return nil
	}); err != nil {
		fail("the linked issues", err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"boards": out})
}
