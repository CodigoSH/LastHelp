package web

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"regexp"
	"strings"

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
// Open, like the name and the mark: this is the page itself, and a visitor who
// could not read it would be shown nothing. An unset layout comes back as the
// default rather than as empty — an installation that never opened the editor
// has a front page, it just never chose one.
func (s *Server) handleGetLanding(w http.ResponseWriter, r *http.Request) {
	stored, err := s.settings().Get(settings.LandingLayout)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "error.internal")
		return
	}

	layout, err := landing.Parse(stored)
	if err != nil {
		// Something unreadable is in the settings row. The front page is not
		// the place to find that out, so it falls back to the page everybody
		// had before and says so where somebody will see it.
		slog.Error("the stored front page layout could not be read", "error", err)
		layout = landing.Default()
	}
	// The board list travels with the page for whoever is going to compose it:
	// the dialog's board picker cannot be built out of what the page happens to
	// be showing, or adding a most-voted module to a page with no boards module
	// would offer an empty list.
	composer := false
	if actor, ok := middleware.ActorFrom(r); ok {
		composer = actor.Can(authz.ManageServer, authz.Scope{})
	}

	body := map[string]any{"modules": layout.Modules}
	boards, needed := s.boardsForLanding(layout, composer)
	if needed {
		body["content"] = landingContentFrom(layout, boards, s.mostVotedIn, s.recentlyDoneIn)
	}
	if composer {
		body["boards"] = describeBoards(boards)
	}
	writeJSON(w, http.StatusOK, body)
}

// boardsForLanding reads the boards only when somebody on this request needs
// them: a page of words and pictures shown to a stranger touches neither the
// board table nor the item table.
func (s *Server) boardsForLanding(layout landing.Layout, composer bool) ([]org.Board, bool) {
	if !layout.NeedsData() && !composer {
		return nil, false
	}
	boards, err := s.orgs().ListBoards()
	if err != nil {
		slog.Error("listing boards for the front page", "error", err)
		return nil, false
	}
	return boards, layout.NeedsData()
}

// landingContentFrom is what the data modules of a layout have to show.
//
// Keyed by module id rather than by kind, because two modules of the same kind
// on one page are the point: most voted for this board beside most voted for
// that one. Nothing in here fails the request — a front page that answers with
// an error because one module could not be filled is a front page that goes
// down when a board is deleted.
func landingContentFrom(layout landing.Layout, boards []org.Board,
	voted, done func([]org.Board) []map[string]any) map[string]any {
	content := map[string]any{}
	for _, m := range layout.Modules {
		switch m.Kind {
		case landing.Boards:
			content[m.ID] = describeBoards(inScope(boards, m.Scope))
		case landing.MostVoted:
			content[m.ID] = voted(inScope(boards, m.Scope))
		case landing.RecentlyDone:
			content[m.ID] = done(inScope(boards, m.Scope))
		case landing.CallToAction:
			content[m.ID] = whereItGoes(inScope(boards, m.Scope))
		}
	}
	return content
}

// inScope narrows a list of boards to what a module was pointed at.
//
// A scope naming a board that no longer exists comes back empty rather than as
// everything: an operator who asked for one board and had it deleted should see
// an empty module, not the whole installation silently taking its place.
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
		// A scope naming only an organisation is every board in it, which is
		// what somebody who runs one organisation and four projects actually
		// wants to put on a page.
		if !deeper || b.Slug == boardSlug {
			out = append(out, b)
		}
	}
	return out
}

func describeBoards(boards []org.Board) []map[string]any {
	out := make([]map[string]any, 0, len(boards))
	for _, b := range boards {
		out = append(out, map[string]any{
			"org":           b.OrgSlug,
			"org_name":      b.OrgName,
			"slug":          b.Slug,
			"name":          b.Name,
			"accepts_bugs":  b.AcceptsBugs,
			"accepts_ideas": b.AcceptsIdeas,
		})
	}
	return out
}

// reportsOnTheFrontPage is how many reports one such module carries.
//
// Enough to fill a tall module and be worth scrolling, small enough that the
// query stays cheap on the one page strangers load. The module scrolls rather
// than clipping, so a short module shows fewer without the rest disappearing.
const reportsOnTheFrontPage = 8

// whereItGoes is the board a button points at.
//
// An unscoped button goes to the first board, which is what makes it useful the
// moment it is added rather than after a trip to its dialog: the installation
// this module is for has one board, and on that one there is no other answer.
// With no boards at all it points nowhere and the module draws nothing, because
// a button to a report form that does not exist is worse than no button.
func whereItGoes(boards []org.Board) map[string]any {
	if len(boards) == 0 {
		return nil
	}
	b := boards[0]
	return map[string]any{
		"org":  b.OrgSlug,
		"slug": b.Slug,
		"name": b.Name,
		"href": "/" + b.OrgSlug + "/" + b.Slug + "/new",
	}
}

// recentlyDoneIn is what was put right lately, newest first.
//
// Both vocabularies that mean finished and neither that means refused: showing
// "declined" under a heading about what was done would be the page saying the
// opposite of what happened.
func (s *Server) recentlyDoneIn(boards []org.Board) []map[string]any {
	return s.reportsIn(boards, item.Filter{
		Statuses:     []item.Status{item.StatusDone, item.StatusClosed},
		SortByClosed: true,
	})
}

// mostVotedIn is the ideas people asked for most.
//
// Ideas only, because only an idea can be voted for. A "most voted" that
// silently included bugs would be ranking things that all have nought.
func (s *Server) mostVotedIn(boards []org.Board) []map[string]any {
	return s.reportsIn(boards, item.Filter{Kind: item.Idea, SortByVote: true})
}

// reportsIn answers one of the report modules.
//
// The boards and the bound are the same for all of them and the question is
// not, so the filter arrives already asked. Published only, which is the
// default the item store deliberately keeps: the front page must not be the
// place something unreviewed appears.
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
			// Seconds, like every other time this API sends: the browser side
			// formats them in the language of the application, and a second
			// shape would mean a second formatter to keep in step.
			row["closed_at"] = it.ClosedAt.Unix()
		}
		out = append(out, row)
	}
	return out
}

// handleSetLanding replaces it.
//
// Validated by the domain, which refuses a layout it cannot draw entirely
// rather than saving the parts it recognised. Saving half of what somebody
// composed and showing them the rest missing is how an editor teaches people
// not to trust it.
func (s *Server) handleSetLanding(w http.ResponseWriter, r *http.Request) {
	if !s.may(w, r, authz.ManageServer, authz.Scope{}) {
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, panelBodyLimit)
	in, err := landing.Submitted(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "error.bad_layout")
		return
	}

	encoded, err := in.Encode()
	if err != nil {
		writeError(w, http.StatusBadRequest, "error.bad_layout")
		return
	}
	if err := s.settings().Set(settings.LandingLayout, encoded); err != nil {
		slog.Error("storing the front page layout", "error", err)
		writeError(w, http.StatusInternalServerError, "error.internal")
		return
	}
	writeJSON(w, http.StatusOK, in)
}
