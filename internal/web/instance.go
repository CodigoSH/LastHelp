package web

import (
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"time"

	"git.codigosh.com/CodigoSH/LastHelp/internal/core/authz"
	"git.codigosh.com/CodigoSH/LastHelp/internal/core/item"
	"git.codigosh.com/CodigoSH/LastHelp/internal/core/landing"
	"git.codigosh.com/CodigoSH/LastHelp/internal/core/org"
	"git.codigosh.com/CodigoSH/LastHelp/internal/core/settings"
	"git.codigosh.com/CodigoSH/LastHelp/internal/web/middleware"
)

// instanceSettings is what an operator can change about the installation
// itself, as opposed to about one organisation.
type instanceSettings struct {
	Name string `json:"name"`
	// Logo is a data URL. Bounded because it is written into every page.
	Logo   string `json:"logo"`
	Accent string `json:"accent"`
}

// maxLogoBytes keeps the logo to something that belongs inline. Anything larger
// is a file, and a file is a thing to serve, cache and back up separately.
const maxLogoBytes = 96 << 10

// accentPattern is a hex colour and nothing else: this value goes into a style
// attribute, so anything that is not exactly six hex digits is refused rather
// than escaped and hoped about.
var accentPattern = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

// maxInstanceNameLength is a heading, not a paragraph. Bounded here rather than
// only in the browser, where a bound is a suggestion.
const maxInstanceNameLength = 60

// handleGetInstance answers what this installation calls itself.
//
// Readable by anybody, because the name, the mark and the colour are on every
// public page anyway — a visitor who could not read them would be shown a page
// wearing somebody else's branding. Only changing them is the administrator's.
func (s *Server) handleGetInstance(w http.ResponseWriter, r *http.Request) {
	store := s.settings()
	name, err := store.Get(settings.InstanceName)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "error.internal")
		return
	}
	logo, err := store.Get(settings.InstanceLogo)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "error.internal")
		return
	}
	accent, err := store.Get(settings.InstanceAccent)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "error.internal")
		return
	}
	writeJSON(w, http.StatusOK, instanceSettings{Name: name, Logo: logo, Accent: accent})
}

// handleSetInstance changes it.
func (s *Server) handleSetInstance(w http.ResponseWriter, r *http.Request) {
	if !s.may(w, r, authz.ManageServer, authz.Scope{}) {
		return
	}

	// Large enough for the logo it is allowed to carry, plus the rest of the
	// object. It was panelBodyLimit — 16 KiB — while a logo may be 96, so the
	// body was cut off mid-JSON and the decode failed: finishing the wizard
	// with any logo over about 12 KB answered "the request was not understood",
	// which is true and useless. Two bounds on the same value, in two files,
	// disagreeing.
	r.Body = http.MaxBytesReader(w, r.Body, maxLogoBytes+(8<<10))
	var in instanceSettings
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		// A body that ran past the bound is an oversized logo, not a malformed
		// request: the only thing in here that can be large is the image.
		if tooLarge(err) {
			writeError(w, http.StatusBadRequest, "error.image_too_big")
			return
		}
		writeError(w, http.StatusBadRequest, "error.bad_request")
		return
	}

	name := strings.TrimSpace(in.Name)
	if len([]rune(name)) > maxInstanceNameLength {
		writeError(w, http.StatusBadRequest, "error.bad_request")
		return
	}
	logo := strings.TrimSpace(in.Logo)
	if logo != "" && (!strings.HasPrefix(logo, "data:image/") || len(logo) > maxLogoBytes) {
		writeError(w, http.StatusBadRequest, "error.image_too_big")
		return
	}
	accent := strings.TrimSpace(in.Accent)
	if accent != "" && !accentPattern.MatchString(accent) {
		writeError(w, http.StatusBadRequest, "error.bad_request")
		return
	}

	store := s.settings()
	for key, value := range map[string]string{
		settings.InstanceName:   name,
		settings.InstanceLogo:   logo,
		settings.InstanceAccent: accent,
	} {
		if err := store.Set(key, value); err != nil {
			writeError(w, http.StatusInternalServerError, "error.internal")
			return
		}
	}
	writeJSON(w, http.StatusOK, instanceSettings{Name: name, Logo: logo, Accent: accent})
}

// handleGetLanding answers what the front page is made of.
//
// Open, like the name and the mark: this is the page itself. Whoever may
// compose it is also given the board list the editor's pickers need.
func (s *Server) handleGetLanding(w http.ResponseWriter, r *http.Request) {
	composer := false
	if actor, ok := middleware.ActorFrom(r); ok {
		composer = actor.Can(authz.ManageServer, authz.Scope{})
	}

	stored, err := s.settings().Get(settings.LandingLayout)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "error.internal")
		return
	}

	page, err := landing.Parse(stored)
	if err != nil {
		// Something unreadable is in the settings row. The front page is not
		// the place to find that out, so it falls back to the default and says
		// so where somebody will see it.
		slog.Error("the stored front page could not be read", "error", err)
		page = landing.Default()
	}

	body := map[string]any{"bands": page.Bands, "settings": page.Settings}
	boards, needed := s.boardsForLanding(page, composer)
	if needed {
		body["content"] = landingContentFrom(page, boards, s.describeBoards, s.reportsFor, s.comingIn, s.countedIn)
	}
	if composer {
		body["boards"] = s.describeBoards(boards)
		// A share picture is announced only at an absolute address, so the
		// editor says so when none is configured rather than letting the
		// operator choose one nobody will ever see.
		body["share_ready"] = s.cfg.PublicURL != ""
	}
	writeJSON(w, http.StatusOK, body)
}

// boardsForLanding reads the boards only when somebody on this request needs
// them: a page of words and pictures shown to a stranger touches neither the
// board table nor the item table.
func (s *Server) boardsForLanding(page landing.Page, composer bool) ([]org.Board, bool) {
	if !page.NeedsData() && !composer {
		return nil, false
	}
	boards, err := s.orgs().ListBoards()
	if err != nil {
		slog.Error("listing boards for the front page", "error", err)
		return nil, false
	}
	return boards, page.NeedsData()
}

// landingContentFrom is what the data modules of a page have to show, keyed by
// module id: two modules of one kind side by side — most voted for this board,
// most voted for that one — are the point. Nothing in here fails the request; a
// front page that errors because one module could not be filled is a front page
// that goes down when a board is deleted.
func landingContentFrom(page landing.Page, boards []org.Board,
	describe func([]org.Board) []map[string]any,
	reports func([]org.Board, string) []map[string]any,
	coming func([]org.Board) map[string]any,
	counted func([]org.Board) map[string]any) map[string]any {
	content := map[string]any{}
	for _, m := range page.Modules() {
		switch m.Kind {
		case landing.Boards:
			content[m.ID] = cut(describe(inScope(boards, m.Scope)), m.Rows)
		case landing.Reports:
			content[m.ID] = cut(reports(inScope(boards, m.Scope), m.Show), m.Rows)
		case landing.Roadmap:
			content[m.ID] = coming(inScope(boards, m.Scope))
		case landing.Statistics:
			content[m.ID] = counted(inScope(boards, m.Scope))
		}
	}
	return content
}

// cut is how many of them the operator asked for, and all of them when they
// never said. Done here so a module that shows three does not carry eight
// across the wire to throw five away.
func cut(rows []map[string]any, howMany int) []map[string]any {
	if howMany <= 0 || howMany >= len(rows) {
		return rows
	}
	return rows[:howMany]
}

// inScope narrows the boards to what a module was pointed at. A scope naming a
// board that no longer exists comes back empty rather than as everything: the
// operator asked for one board, and the whole installation silently taking its
// place is a page answering a question nobody asked.
func inScope(boards []org.Board, scope string) []org.Board {
	if scope == "" {
		return boards
	}
	orgSlug, boardSlug, deeper := strings.Cut(scope, "/")
	out := make([]org.Board, 0, 1)
	for _, b := range boards {
		if b.OrgSlug != orgSlug {
			continue
		}
		if !deeper || b.Slug == boardSlug {
			out = append(out, b)
		}
	}
	return out
}

func (s *Server) describeBoards(boards []org.Board) []map[string]any {
	ids := make([]int64, 0, len(boards))
	for _, b := range boards {
		ids = append(ids, b.ID)
	}
	total, err := s.items().TotalByKind(ids)
	if err != nil {
		// The cards are still worth drawing: they say what a board takes, and
		// the numbers are the part that is missing.
		slog.Error("counting reports for the front page", "error", err)
	}

	out := make([]map[string]any, 0, len(boards))
	for _, b := range boards {
		out = append(out, map[string]any{
			"org":           b.OrgSlug,
			"org_name":      b.OrgName,
			"slug":          b.Slug,
			"name":          b.Name,
			"accepts_bugs":  b.AcceptsBugs,
			"accepts_ideas": b.AcceptsIdeas,
			"icon":          b.Icon,
			// Everything published, open or answered: the board's two tabs added
			// together. Only the kinds the board takes are drawn, so a missing one
			// is not a zero.
			"total_bugs":  total[b.ID][item.Bug],
			"total_ideas": total[b.ID][item.Idea],
		})
	}
	return out
}

// reportsOnTheFrontPage is how many reports one module carries at most: enough
// to fill a tall column, few enough that the one page strangers load stays
// cheap.
const reportsOnTheFrontPage = 8

// reportsFor answers a reports module, by which list it shows.
//
//   - most voted: ideas only, because only an idea can be voted for;
//   - recently resolved: both vocabularies that mean finished — an idea done, a
//     bug closed — and neither that means refused, newest first;
//   - latest: what was published most recently;
//   - planned: what the team has said it will do.
//
// Published only in every case, the default the item store keeps: the front
// page must never be where something unreviewed appears.
func (s *Server) reportsFor(boards []org.Board, show string) []map[string]any {
	switch show {
	case landing.ShowRecentlyDone:
		return s.reportsIn(boards, item.Filter{
			Statuses:     []item.Status{item.StatusDone, item.StatusClosed},
			SortByClosed: true,
		})
	case landing.ShowLatest:
		return s.reportsIn(boards, item.Filter{})
	case landing.ShowPlanned:
		return s.reportsIn(boards, item.Filter{Statuses: []item.Status{item.StatusPlanned}})
	default:
		return s.reportsIn(boards, item.Filter{Kind: item.Idea, SortByVote: true})
	}
}

// comingIn is the roadmap: what is planned, in progress, and done, in the
// columns a board's own roadmap uses.
func (s *Server) comingIn(boards []org.Board) map[string]any {
	out := map[string]any{}
	for _, column := range item.Columns {
		out[string(column)] = []map[string]any{}
	}
	if len(boards) == 0 {
		return out
	}

	ids := make([]int64, 0, len(boards))
	byID := make(map[int64]org.Board, len(boards))
	for _, b := range boards {
		ids = append(ids, b.ID)
		byID[b.ID] = b
	}

	columns, err := s.items().RoadmapAcross(ids, roadmapOnTheFrontPage)
	if err != nil {
		slog.Error("reading the roadmap for the front page", "error", err)
		return out
	}
	for column, items := range columns {
		rows := make([]map[string]any, 0, len(items))
		for _, it := range items {
			rows = append(rows, map[string]any{
				"public_id": it.PublicID,
				"title":     it.Title,
				"kind":      string(it.Kind),
				"votes":     it.VoteCount,
				"board":     byID[it.RepoID].Name,
			})
		}
		out[string(column)] = rows
	}
	return out
}

// roadmapOnTheFrontPage is how many of each column one module carries.
const roadmapOnTheFrontPage = 4

// countedIn is how much came in and where it got to, and nothing at all when
// nothing has been published: three zeroes tell the first person who ever looks
// that nothing happens here, which is both untrue and the worst thing this page
// could say.
func (s *Server) countedIn(boards []org.Board) map[string]any {
	ids := make([]int64, 0, len(boards))
	for _, b := range boards {
		ids = append(ids, b.ID)
	}
	counts, err := s.items().Count(ids)
	if err != nil {
		slog.Error("counting reports for the front page", "error", err)
		return nil
	}
	if counts.Received == 0 {
		return nil
	}
	return map[string]any{
		"received": counts.Received,
		"open":     counts.Open,
		"done":     counts.Done,
	}
}

// reportsIn lists published reports on these boards, with the filter already
// asked.
func (s *Server) reportsIn(boards []org.Board, ask item.Filter) []map[string]any {
	if len(boards) == 0 {
		return []map[string]any{}
	}
	byID := make(map[int64]org.Board, len(boards))
	ask.RepoIDs = make([]int64, 0, len(boards))
	for _, b := range boards {
		ask.RepoIDs = append(ask.RepoIDs, b.ID)
		byID[b.ID] = b
	}
	ask.Limit = reportsOnTheFrontPage

	items, err := s.items().List(ask)
	if err != nil {
		slog.Error("listing reports for the front page", "error", err)
		return []map[string]any{}
	}

	out := make([]map[string]any, 0, len(items))
	for _, it := range items {
		row := map[string]any{
			"public_id": it.PublicID,
			"title":     it.Title,
			"kind":      string(it.Kind),
			"status":    string(it.Status),
			"votes":     it.VoteCount,
			"board":     byID[it.RepoID].Name,
		}
		if !it.ClosedAt.IsZero() {
			row["closed_at"] = it.ClosedAt.Unix()
		}
		out = append(out, row)
	}
	return out
}

// handleSearch is the search on the front page: reports that already exist,
// across every board, so a visitor finds theirs before writing it again.
//
// Published only — the item store's own default — and a handful at most: this
// is a list somebody reads while typing, not the board. Two characters before
// anything is asked, because one matches half the titles on any board and says
// nothing. The boards come back as well, so the last line can always be the
// way to write a new report.
func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if runes := []rune(q); len(runes) < 2 || len(runes) > 100 {
		writeJSON(w, http.StatusOK, map[string]any{"items": []any{}, "boards": []any{}})
		return
	}
	boards, err := s.orgs().ListBoards()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "error.internal")
		return
	}
	hits := []map[string]any{}
	if len(boards) > 0 {
		byID := make(map[int64]org.Board, len(boards))
		ids := make([]int64, 0, len(boards))
		for _, b := range boards {
			ids = append(ids, b.ID)
			byID[b.ID] = b
		}
		items, err := s.items().List(item.Filter{RepoIDs: ids, Search: q, AsTyped: true, SortByVote: true, Limit: searchHits})
		if err != nil {
			slog.Error("searching the front page", "error", err)
			writeError(w, http.StatusInternalServerError, "error.internal")
			return
		}
		for _, it := range items {
			hits = append(hits, map[string]any{
				"public_id": it.PublicID,
				"title":     it.Title,
				"kind":      string(it.Kind),
				"status":    string(it.Status),
				"votes":     it.VoteCount,
				"board":     byID[it.RepoID].Name,
			})
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": hits, "boards": s.describeBoards(boards)})
}

// searchHits is how many reports one search answers with.
const searchHits = 6

// handleSaveLanding stores the front page, and it is what visitors see from
// then on — edited in place, each change kept as
// it is made, with no second copy to publish or forget about.
//
// Validated by the domain, which refuses a page it cannot draw entirely rather
// than keeping the parts it recognised: saving half of what somebody composed
// and showing them the rest missing is how an editor teaches people not to
// trust it.
//
// A page that is exactly the default is stored as no page at all: the default
// is read fresh rather than stored, so an operator who restored it gets the
// better one a later release brings, like everybody who never changed it.
func (s *Server) handleSaveLanding(w http.ResponseWriter, r *http.Request) {
	if !s.may(w, r, authz.ManageServer, authz.Scope{}) {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, panelBodyLimit)
	page, err := landing.Submitted(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "error.bad_layout")
		return
	}
	encoded, err := page.Encode()
	if err != nil {
		writeError(w, http.StatusBadRequest, "error.bad_layout")
		return
	}
	if isDefaultPage(page) {
		encoded = ""
	}
	if err := s.settings().Set(settings.LandingLayout, encoded); err != nil {
		slog.Error("storing the front page", "error", err)
		writeError(w, http.StatusInternalServerError, "error.internal")
		return
	}
	s.sweepLandingImages()
	writeJSON(w, http.StatusOK, map[string]any{"bands": page.Bands, "settings": page.Settings})
}

// handleRestoreLandingDefault puts the default page back in front of visitors.
func (s *Server) handleRestoreLandingDefault(w http.ResponseWriter, r *http.Request) {
	if !s.may(w, r, authz.ManageServer, authz.Scope{}) {
		return
	}
	if err := s.settings().Set(settings.LandingLayout, ""); err != nil {
		writeError(w, http.StatusInternalServerError, "error.internal")
		return
	}
	s.sweepLandingImages()
	page := landing.Default()
	writeJSON(w, http.StatusOK, map[string]any{"bands": page.Bands, "settings": page.Settings})
}

// isDefaultPage says whether a page is the default one, compared as it would
// be stored.
func isDefaultPage(page landing.Page) bool {
	a, errA := page.Encode()
	b, errB := landing.Default().Encode()
	return errA == nil && errB == nil && a == b
}

// sweepLandingImages forgets the pictures the front page no longer refers to.
// After the page is stored, never before: a sweep that ran first and then
// failed to store would delete pictures the page still shows. Not
// worth failing the request over — what is left behind is bytes nobody sees.
func (s *Server) sweepLandingImages() {
	stored, err := s.settings().Get(settings.LandingLayout)
	if err != nil {
		slog.Warn("reading the front page to sweep its pictures", "error", err)
		return
	}
	page, err := landing.Parse(stored)
	if err != nil {
		// A page that cannot be read cannot say which pictures it uses, so
		// nothing is swept rather than everything.
		slog.Warn("reading the front page to sweep its pictures", "error", err)
		return
	}
	if err := landing.ForgetUnusedImages(s.db, page); err != nil {
		slog.Warn("clearing unused front page pictures", "error", err)
	}
}

// handleAddLandingImage takes one picture for the front page.
//
// WebP only, and the browser is what makes it one: it scales and re-encodes
// before sending, so the operator picks whatever came off their phone and what
// arrives is a fraction of it. Checking the bytes rather than the declared type
// is the part that matters here — a caller writes its own headers.
func (s *Server) handleAddLandingImage(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, landing.MaxImageBytes))
	if err != nil {
		writeError(w, http.StatusRequestEntityTooLarge, "error.image_too_large")
		return
	}

	at, err := landing.StoreImage(s.db, body, time.Now())
	if errors.Is(err, landing.ErrNotAPicture) {
		writeError(w, http.StatusBadRequest, "error.not_an_image")
		return
	}
	if err != nil {
		slog.Error("storing a front page picture", "error", err)
		writeError(w, http.StatusInternalServerError, "error.internal")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"url": at})
}

// handleGetLandingImage serves one.
//
// Public, because the front page is: a picture on it that needed a session
// would draw as a broken box for every visitor. The name is thirty-two random
// hex characters and carries no meaning, so knowing one tells nobody anything
// they could not already see on the page.
func (s *Server) handleGetLandingImage(w http.ResponseWriter, r *http.Request) {
	img, err := landing.ReadImage(s.db, r.PathValue("id"))
	if errors.Is(err, sql.ErrNoRows) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		slog.Error("reading a front page picture", "error", err)
		writeError(w, http.StatusInternalServerError, "error.internal")
		return
	}
	w.Header().Set("Content-Type", img.ContentType)
	// The name is derived from nothing and never reused, so the bytes behind it
	// cannot change. A year, immutable, is the honest cache for that — and it
	// is most of why putting a picture on the front page is cheap.
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	_, _ = w.Write(img.Bytes)
}
