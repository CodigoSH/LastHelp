// Package landing is what the public front page is made of.
//
// The page used to be a fixed composition: a mark, a tagline, three sentences
// and the list of organisations, in that order, for everybody. It read like the
// sign-in screen because it was built like one — a screen that asks for one
// thing, wearing all that space on purpose — while what it actually has to do
// is show a visitor where to go.
//
// So it is a list of modules now, and the operator decides which and in what
// order. This package owns what a module may be, and refuses anything else:
// the layout arrives over HTTP from an administrator and is rendered on the one
// page strangers see, so "we did not recognise that, we left it out" is not an
// acceptable answer. A layout is either understood entirely or refused entirely.
package landing

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
)

// Kind is what a module shows.
type Kind string

const (
	// Hero is the mark, the name and the tagline of this installation.
	Hero Kind = "hero"

	// Intro is the three sentences about how reporting works here.
	Intro Kind = "intro"

	// Organisations is the list of places somebody can report to.
	Organisations Kind = "organisations"

	// Text is whatever the operator wants to say. Plain text, not markup:
	// this renders on a public page, and a module that accepted HTML would be
	// a way to put script on it that no amount of escaping elsewhere fixes.
	Text Kind = "text"

	// Boards is the list of boards themselves, without going through the
	// organisation that holds them. The usual installation is one person with
	// one organisation and a handful of projects, and there the organisation
	// level is a door with nothing behind it: a visitor who wants to report
	// something has to open it to find the list they were looking for.
	Boards Kind = "boards"

	// MostVoted is the ideas people asked for most.
	//
	// The front page already promises this — "ideas collect votes, which shows
	// what is most requested" — and until now there was nowhere on it that
	// showed one. A promise a page makes and does not keep reads as broken.
	MostVoted Kind = "most_voted"

	// RecentActivity is what has arrived lately.
	//
	// Useful only where there is a flow of it: on a quiet portal a list of the
	// same three reports for a month says less than nothing. It is offered and
	// not placed by default for exactly that reason.
	RecentActivity Kind = "recent_activity"

	// Roadmap is what is coming, being worked on, and done.
	//
	// The same three columns the board's own roadmap uses, because a visitor
	// who reads one and then the other should not have to learn a second
	// vocabulary.
	Roadmap Kind = "roadmap"

	// Statistics is how much has come in and how much is finished.
	//
	// The most dangerous of these on a fresh installation: three zeroes say
	// "nothing happens here" to the first person who ever looks. It draws
	// nothing at all until there is something to count.
	Statistics Kind = "statistics"

	// Section is a box that holds other modules.
	//
	// The dashboard's own, and the reason it exists there is the reason it
	// exists here: once a module shows one thing — this board, this
	// organisation — a page with eight of them is eight loose boxes, and the
	// only thing that makes them read as a group is a box around them.
	//
	// A section has its own grid inside it, as wide and as tall as the section
	// is, and its children are placed in THAT grid. A section never holds
	// another section: two levels is a layout somebody arranged, three is a
	// layout nobody can drag.
	Section Kind = "section"

	// RecentlyDone is what was put right lately.
	//
	// The strongest thing this page can say to somebody deciding whether it is
	// worth typing anything: reports here get read and acted on. It takes both
	// vocabularies that mean finished — an idea is done, a bug is closed — and
	// leaves out the two that mean refused.
	RecentlyDone Kind = "recently_done"

	// CallToAction is a button that goes straight to a board's report form.
	//
	// The one thing this page exists to make happen, and until now the only
	// route to it was the list: pick an organisation, pick a board, find the
	// button. An operator with one project should be able to put that button on
	// the front page and be done.
	CallToAction Kind = "call_to_action"
)

// usesData is the modules whose content has to be read out of the database and
// sent with the page: the boards, the reports, and the button's destination.
//
// Deliberately NOT the organisations module. It draws the same list the page
// already carries for its own navigation, so a front page of a mark, some words
// and the organisations still reads neither the board table nor the item table.
// Putting it here cost exactly that, and a test said so.
func (k Kind) usesData() bool {
	switch k {
	case Boards, MostVoted, RecentlyDone, CallToAction, RecentActivity, Roadmap, Statistics:
		return true
	}
	return false
}

// takesScope is the modules that can be pointed at one place. A wider set than
// usesData: where a module's content comes from and whether it can be narrowed
// are two different questions, and answering them with one predicate is how the
// default page started reading tables it has no use for.
func (k Kind) takesScope() bool { return k == Organisations || k.usesData() }

// usesWords is the modules that carry words of their own: the operator's own
// paragraph, and the label on the button.
func (k Kind) usesWords() bool { return k == Text || k == CallToAction || k == Section }

// usesHeader is the modules that draw a name of their own, and so are the ones
// a label position means anything to. A hero is its own name and a paragraph
// has none, so neither is asked where to put one.
func (k Kind) usesHeader() bool {
	switch k {
	case Organisations, Boards, MostVoted, RecentlyDone, RecentActivity, Roadmap, Statistics:
		return true
	}
	return false
}

// LabelAt is where a module's name sits.
//
// The four the dashboard offers on its own items, so an operator who has
// arranged one board already knows this control. What each one looks like is
// not identical across the two products and that is deliberate: a bookmark
// there is a square with an icon, so its name sits straight against it, while a
// module here is a box holding a list and its name needs a line under it or it
// reads as the first row.
type LabelAt string

const (
	// LabelTop is the default, and what every module drew before there was a
	// choice. The zero value means this, so a layout written earlier keeps the
	// look it had.
	LabelTop LabelAt = "top"

	// LabelBottom is the same header at the foot of the module.
	LabelBottom LabelAt = "bottom"

	// LabelPill is the name on the module's top border, the same pill a section
	// wears. Chosen by the operator, never given out by the product: handing it
	// to every module automatically is what made this page read as a row of
	// containers with their contents missing.
	LabelPill LabelAt = "pill"

	// LabelOff draws no name at all.
	LabelOff LabelAt = "off"
)

func (l LabelAt) valid() bool {
	switch l {
	case "", LabelTop, LabelBottom, LabelPill, LabelOff:
		return true
	}
	return false
}

// The grid, ported from the dashboard this house already runs.
//
// Twelve columns and rows of a fixed height, each module placed at a cell and
// spanning a number of them. Not a first guess: it is the same geometry the
// dashboard uses, and the point of taking it whole rather than inventing a
// smaller one is that the arithmetic, the collision rules and the look are
// already worked out and already carry the house's mark.
const (
	maxModules  = 24
	maxTextBody = 4_000

	// maxLabel is a button's words, in runes rather than bytes: the bound is
	// about what fits on a button, and a byte count would cut a Japanese label
	// at a third of the length of an English one.
	maxLabel = 80

	Columns = 12

	// The narrowest a module may be, in columns.
	//
	// One, like the dashboard's own items. It was two, on the grounds that
	// eighty pixels is not a module — which was true while every module was a
	// band of content across the page. It stopped being true when a module
	// became able to show one board, one organisation or one button: those are
	// cards, and a card in a single column is exactly what the dashboard puts
	// in one. How small is useful is the operator's judgement, not a number
	// decided here.
	MinSpan = 1
)

// Module is one block on the page, and where it sits.
//
// X and Y are one-based, the way CSS grid lines are, so the numbers stored here
// are the numbers the stylesheet uses and nothing has to add one on the way
// through. W and H are spans.
type Module struct {
	ID   string `json:"id"`
	Kind Kind   `json:"kind"`

	X int `json:"x"`
	Y int `json:"y"`
	W int `json:"w"`
	H int `json:"h"`

	// Body is the words of a Text module, and empty for every other kind.
	Body string `json:"body,omitempty"`

	// Parent is the id of the section holding this module, and empty for a
	// module sitting straight on the page.
	//
	// A child's X, Y, W and H are in ITS SECTION's grid, not the page's: a
	// child at 1,1 is the top-left cell of its section wherever that section
	// happens to be. Keeping them relative is what lets a section be dragged
	// somewhere else without rewriting everything inside it.
	Parent string `json:"parent,omitempty"`

	// Label is where this module's name sits. Empty means the top, which is
	// what every module drew before there was anywhere else to put it.
	Label LabelAt `json:"label,omitempty"`

	// Scope is which board a data module draws from, as "org/board", and empty
	// for all of them. It is stored as slugs rather than as row ids because a
	// layout is written by hand often enough — and because an id would make a
	// page silently point at whatever took that row's place.
	Scope string `json:"scope,omitempty"`

	// legacyWidth is the coarse width the first version of this stored. Read,
	// never written: it exists so a layout saved then can be placed on the grid
	// once, and it disappears the next time that layout is saved.
	legacyWidth string
}

// UnmarshalJSON keeps the old width long enough to place the module.
func (m *Module) UnmarshalJSON(data []byte) error {
	type plain Module
	var in struct {
		plain
		Width string `json:"width"`
	}
	if err := json.Unmarshal(data, &in); err != nil {
		return err
	}
	*m = Module(in.plain)
	m.legacyWidth = in.Width
	return nil
}

// Overlaps says whether two modules would sit on top of each other.
//
// The question the drag answers before it lets go: the grid has no opinion
// about two things in one cell, so this is the only thing between an operator
// and a page where a module is hidden under another one.
func (m Module) Overlaps(other Module) bool {
	return m.X < other.X+other.W && other.X < m.X+m.W &&
		m.Y < other.Y+other.H && other.Y < m.Y+m.H
}

// Layout is the whole page, in order.
type Layout struct {
	Modules []Module `json:"modules"`
}

// Default is the front page of an installation that never composes one.
//
// The spans are what each module draws, not round numbers: a row is as tall as
// a column is wide, so a span too generous shows as a band of nothing between
// the modules and a span too mean lets the words out of their box.
//
// The hero is one row now. It was three, from when it was the sign-in screen's
// composition without the card — a centred mark, a display-sized name and a
// third of the window of air. Laid along the top of its module it is a banner,
// and a banner is one row.
func Default() Layout {
	return Layout{Modules: []Module{
		{ID: "hero", Kind: Hero, X: 1, Y: 1, W: 12, H: 1},
		{ID: "intro", Kind: Intro, X: 1, Y: 2, W: 12, H: 2},
		{ID: "organisations", Kind: Organisations, X: 1, Y: 4, W: 12, H: 2},
	}}
}

// NeedsData says whether drawing this page means reading the database.
//
// It lives here rather than in the handler because it is a fact about what the
// modules are, and because the handler is where somebody would be tempted to
// answer it by looking at the response.
func (l Layout) NeedsData() bool {
	for _, m := range l.Modules {
		if m.Kind.usesData() {
			return true
		}
	}
	return false
}

// validatePlacement checks where a module sits, in whichever grid it sits in.
//
// Two grids, two sets of bounds. A module on the page is measured against the
// twelve columns of the page; a module in a section is measured against that
// section, because its coordinates are that section's. Mixing the two is how a
// child ends up drawn outside the box that is supposed to contain it — the
// dashboard this comes from checks only how MANY children there are, and lets a
// child sit past the edge of a section that shrank under it.
func (l Layout) validatePlacement(m Module) error {
	if m.X < 1 || m.Y < 1 {
		return fmt.Errorf("module %q starts at %d,%d", m.ID, m.X, m.Y)
	}
	if m.H < 1 {
		return fmt.Errorf("module %q is %d rows tall", m.ID, m.H)
	}

	if m.Parent == "" {
		if m.W < MinSpan {
			return fmt.Errorf("module %q is %d columns wide, and the narrowest a module may be is %d",
				m.ID, m.W, MinSpan)
		}
		if m.X+m.W-1 > Columns {
			// Off the right-hand edge is not a layout the grid can draw: the
			// module would be cut, and cut silently.
			return fmt.Errorf("module %q runs past column %d", m.ID, Columns)
		}
		return nil
	}

	if m.Kind == Section {
		// Two levels is a layout somebody arranged. Three is a layout nobody
		// can drag, and every rule here would need a third set of coordinates.
		return fmt.Errorf("section %q is inside another section", m.ID)
	}
	// A single column of a section is already narrow, so the page's minimum
	// does not apply in here: it is a bound about the page's twelve columns.
	if m.W < 1 {
		return fmt.Errorf("module %q is %d columns wide", m.ID, m.W)
	}

	parent, ok := l.find(m.Parent)
	if !ok {
		return fmt.Errorf("module %q is inside %q, which is not on this page", m.ID, m.Parent)
	}
	if parent.Kind != Section {
		return fmt.Errorf("module %q is inside %q, which is not a section", m.ID, m.Parent)
	}
	if m.X+m.W-1 > parent.W || m.Y+m.H-1 > parent.H {
		return fmt.Errorf("module %q sticks out of the section %q holding it", m.ID, m.Parent)
	}
	return nil
}

// find is the module with this id, if the page has one.
func (l Layout) find(id string) (Module, bool) {
	for _, m := range l.Modules {
		if m.ID == id {
			return m, true
		}
	}
	return Module{}, false
}

// validateScope checks what a data module was pointed at.
//
// Refused rather than ignored: a scope on a module that cannot use one is
// somebody expecting a narrower page than they are going to get, and an
// unreadable one is a module that would quietly draw everything instead of the
// board it names. Whether that board still exists is not decided here — it can
// be deleted long after the page was composed, and the page has to survive it.
func (m Module) validateScope() error {
	if m.Scope == "" {
		return nil
	}
	if !m.Kind.takesScope() {
		return fmt.Errorf("module %q names a place and is not a module that shows one", m.ID)
	}
	org, board, deeper := strings.Cut(m.Scope, "/")
	if !isSlug(org) || (deeper && !isSlug(board)) {
		return fmt.Errorf("module %q names %q, which is not an organisation or a board", m.ID, m.Scope)
	}
	// A button goes to one report form, so "somewhere in this organisation" is
	// not an answer it can act on. Every other module narrows to a set, and one
	// organisation is a perfectly good set.
	if m.Kind == CallToAction && !deeper {
		return fmt.Errorf("module %q is a button and %q is not one board", m.ID, m.Scope)
	}
	return nil
}

// ShowsOne says whether this module was pointed at a single thing, which is
// what decides how it draws: a module showing one board is that board, a module
// showing a set of them is a list of them.
//
// The presentation follows what was ASKED FOR, not what came back. An
// installation that happens to have one organisation should not find its list
// silently turned into a tile the day it has two.
func (m Module) ShowsOne() bool {
	if m.Scope == "" {
		return false
	}
	_, _, deeper := strings.Cut(m.Scope, "/")
	if m.Kind == Organisations {
		return !deeper
	}
	return deeper
}

// isSlug is the shape of the slugs the rest of the product hands out. Kept
// deliberately narrow: this value is read back and compared, never interpolated
// into anything, and a narrow shape is one fewer thing to reason about.
func isSlug(v string) bool {
	if v == "" || len(v) > 64 {
		return false
	}
	for _, r := range v {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-', r == '_':
		default:
			return false
		}
	}
	return true
}

// ErrUnknownKind means a module nothing here can draw.
var ErrUnknownKind = errors.New("unknown module")

// Parse reads a stored or submitted layout.
//
// An empty value is the default rather than an empty page: that is the state
// every installation starts in, and a front page with nothing on it is not a
// choice anybody made.
func Parse(stored string) (Layout, error) {
	if strings.TrimSpace(stored) == "" {
		return Default(), nil
	}

	var layout Layout
	if err := json.Unmarshal([]byte(stored), &layout); err != nil {
		return Layout{}, fmt.Errorf("reading the layout: %w", err)
	}
	layout.placeTheUnplaced()
	if err := layout.validate(); err != nil {
		return Layout{}, err
	}
	return layout, nil
}

// placeTheUnplaced gives coordinates to a layout written before there were any.
//
// The first version of this stored a coarse width — a module was the whole row
// or half of it — and nothing else. Those layouts are real: they are saved in
// installations that composed a page between one release and the next, and
// refusing to read one would mean an operator's front page coming back as an
// error rather than as their page.
//
// So a module with no size is given one: full becomes the twelve columns it
// meant, half becomes six, and they are stacked in the order they were written.
// Once read and saved again they carry coordinates like everything else, and
// this never fires for them a second time.
func (l *Layout) placeTheUnplaced() {
	row := 1
	for i := range l.Modules {
		m := &l.Modules[i]
		if m.W != 0 && m.H != 0 && m.X != 0 && m.Y != 0 {
			continue
		}
		if m.W == 0 {
			// Half is the only other width the old format had. Anything else
			// was already refused when it was written.
			m.W = Columns
			if strings.EqualFold(m.legacyWidth, "half") {
				m.W = Columns / 2
			}
		}
		if m.H == 0 {
			m.H = defaultHeight(m.Kind)
		}
		if m.X == 0 {
			m.X = 1
		}
		if m.Y == 0 {
			m.Y = row
		}
		row = m.Y + m.H

		// Used, so forgotten. Keeping it would mean a module that came from an
		// old layout never comparing equal to the same module written today.
		m.legacyWidth = ""
	}
}

// defaultHeight is how tall a module is when nobody has said.
//
// The same measured spans Default uses, so a layout written before there were
// coordinates comes back sized like one written today rather than carrying the
// taller guess the first version made.
func defaultHeight(kind Kind) int {
	switch kind {
	case Hero:
		return 3
	default:
		return 2
	}
}

// Submitted reads a layout an administrator has just composed.
//
// It exists so the forgiving step cannot be skipped. Parse does it for a layout
// coming out of the database and the handler decoded straight into the type,
// which meant a browser still running the previous bundle — one that sends a
// coarse width and no coordinates — was answered with a refusal for sending
// exactly what the release before this one taught it to send.
func Submitted(r io.Reader) (Layout, error) {
	var layout Layout
	if err := json.NewDecoder(r).Decode(&layout); err != nil {
		return Layout{}, fmt.Errorf("reading the layout: %w", err)
	}
	layout.placeTheUnplaced()
	if err := layout.validate(); err != nil {
		return Layout{}, err
	}
	return layout, nil
}

// Encode is the other direction, and validates for the same reason: the only
// way a bad layout reaches the database is through here.
func (l Layout) Encode() (string, error) {
	if err := l.validate(); err != nil {
		return "", err
	}
	out, err := json.Marshal(l)
	if err != nil {
		return "", fmt.Errorf("writing the layout: %w", err)
	}
	return string(out), nil
}

func (l Layout) validate() error {
	if len(l.Modules) == 0 {
		return errors.New("a page needs at least one module")
	}
	if len(l.Modules) > maxModules {
		return fmt.Errorf("a page may have at most %d modules", maxModules)
	}

	seen := map[string]bool{}
	for i, m := range l.Modules {
		if strings.TrimSpace(m.ID) == "" {
			return fmt.Errorf("module %d has no id", i+1)
		}
		if seen[m.ID] {
			// Two modules with one id is a page where moving one moves the
			// other, and the editor would look broken rather than the data.
			return fmt.Errorf("two modules share the id %q", m.ID)
		}
		seen[m.ID] = true

		switch m.Kind {
		case Hero, Intro, Organisations, Text, Boards, MostVoted, RecentlyDone, CallToAction,
			RecentActivity, Roadmap, Statistics, Section:
		default:
			return fmt.Errorf("%w: %q", ErrUnknownKind, m.Kind)
		}
		if err := l.validatePlacement(m); err != nil {
			return err
		}
		for _, other := range l.Modules[i+1:] {
			// Only against what shares its grid. A child and a module on the
			// page are measured in different coordinates, so asking whether
			// they overlap is asking a question with no meaning — and answering
			// it would refuse layouts that are perfectly drawable.
			if m.Parent == other.Parent && m.Overlaps(other) {
				return fmt.Errorf("modules %q and %q sit on top of each other", m.ID, other.ID)
			}
		}
		if !m.Kind.usesWords() && m.Body != "" {
			return fmt.Errorf("module %q carries words and has nowhere to put them", m.ID)
		}
		// A button's label is a label. Bounded separately from a paragraph
		// because four thousand characters on a button is not a long label, it
		// is a layout nobody can use and a page nobody can read past.
		if (m.Kind == CallToAction || m.Kind == Section) && len([]rune(m.Body)) > maxLabel {
			return fmt.Errorf("module %q has a label longer than %d characters", m.ID, maxLabel)
		}
		if len(m.Body) > maxTextBody {
			return fmt.Errorf("module %q is longer than %d characters", m.ID, maxTextBody)
		}
		if err := m.validateScope(); err != nil {
			return err
		}
		if !m.Label.valid() {
			return fmt.Errorf("module %q puts its name at %q, which is nowhere", m.ID, m.Label)
		}
		if m.Label != "" && !m.Kind.usesHeader() {
			return fmt.Errorf("module %q says where to put a name it does not draw", m.ID)
		}
	}
	return nil
}
