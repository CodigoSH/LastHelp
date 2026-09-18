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

// usesData is the modules that draw something out of the database rather than
// out of their own configuration. They are the only ones a scope means anything
// to — including the button, which needs the board list to know where it goes.
func (k Kind) usesData() bool {
	switch k {
	case Boards, MostVoted, RecentlyDone, CallToAction:
		return true
	}
	return false
}

// usesWords is the modules that carry words of their own: the operator's own
// paragraph, and the label on the button.
func (k Kind) usesWords() bool { return k == Text || k == CallToAction }

// usesHeader is the modules that draw a name of their own, and so are the ones
// a label position means anything to. A hero is its own name and a paragraph
// has none, so neither is asked where to put one.
func (k Kind) usesHeader() bool {
	switch k {
	case Organisations, Boards, MostVoted, RecentlyDone:
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

	// A module may not be narrower than this. One column of a twelve column
	// grid is about eighty pixels, which is not a module, it is a mistake.
	MinSpan = 2
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
// The spans are the measured height of what each module draws, not round
// numbers: a row is as tall as a column is wide, so a span too generous shows
// as a band of nothing between the modules and a span too mean lets the words
// out of their box. Measured at a 1080px grid, where a row is 75px: the hero
// draws 250, the three cards 103, the list of organisations 115.
func Default() Layout {
	return Layout{Modules: []Module{
		{ID: "hero", Kind: Hero, X: 1, Y: 1, W: 12, H: 3},
		{ID: "intro", Kind: Intro, X: 1, Y: 4, W: 12, H: 2},
		{ID: "organisations", Kind: Organisations, X: 1, Y: 6, W: 12, H: 2},
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
	if !m.Kind.usesData() {
		return fmt.Errorf("module %q names a board and is not a module that shows one", m.ID)
	}
	org, board, found := strings.Cut(m.Scope, "/")
	if !found || !isSlug(org) || !isSlug(board) {
		return fmt.Errorf("module %q names %q, which is not an organisation and a board", m.ID, m.Scope)
	}
	return nil
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
		case Hero, Intro, Organisations, Text, Boards, MostVoted, RecentlyDone, CallToAction:
		default:
			return fmt.Errorf("%w: %q", ErrUnknownKind, m.Kind)
		}
		if m.X < 1 || m.X > Columns {
			return fmt.Errorf("module %q starts at column %d, outside a %d column grid", m.ID, m.X, Columns)
		}
		if m.Y < 1 {
			return fmt.Errorf("module %q starts at row %d", m.ID, m.Y)
		}
		if m.W < MinSpan || m.H < 1 {
			return fmt.Errorf("module %q is %dx%d, and the smallest a module may be is %dx1",
				m.ID, m.W, m.H, MinSpan)
		}
		if m.X+m.W-1 > Columns {
			// Off the right-hand edge is not a layout the grid can draw: the
			// module would be cut, and cut silently.
			return fmt.Errorf("module %q runs past column %d", m.ID, Columns)
		}
		for _, other := range l.Modules[i+1:] {
			if m.Overlaps(other) {
				return fmt.Errorf("modules %q and %q sit on top of each other", m.ID, other.ID)
			}
		}
		if !m.Kind.usesWords() && m.Body != "" {
			return fmt.Errorf("module %q carries words and has nowhere to put them", m.ID)
		}
		// A button's label is a label. Bounded separately from a paragraph
		// because four thousand characters on a button is not a long label, it
		// is a layout nobody can use and a page nobody can read past.
		if m.Kind == CallToAction && len([]rune(m.Body)) > maxLabel {
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
