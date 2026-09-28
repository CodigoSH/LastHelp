// Package landing is what the public front page is made of.
//
// A front page here is a website, not a dashboard. It was a dashboard's grid
// for a while — twelve columns of square cells, each module placed at a cell
// and sized in them — and that is the right tool for a private board somebody
// arranges for themselves and the wrong one for a page a stranger reads top to
// bottom on whatever screen they have: heights tied to widths cut the words
// off, a phone got the modules in whatever order they happened to stack, and
// nothing stopped two boxes ending three pixels apart.
//
// So a page is a stack of bands. Each band chooses its columns from a handful
// of layouts, each column holds modules one under another, and every module is
// as tall as what it says. There are no coordinates to get wrong, a phone reads
// the columns in order, and the choices an operator makes are the ones a
// website needs — what goes where, and in which band — rather than the ones a
// page-building product would offer.
//
// This package owns what a page may be and refuses anything else. The page
// arrives over HTTP from an administrator and is drawn on the one screen
// strangers see, so it is understood entirely or refused entirely.
package landing

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"regexp"
	"strings"
)

// Kind is what a module shows.
type Kind string

const (
	// Heading opens a page: a title, a line under it, the installation's mark,
	// and — on the default page — the search that stops a report being
	// written twice. Its words are plain, because a heading is one sentence
	// and markup in it would only be a way to make it something else.
	Heading Kind = "heading"

	// Text is whatever the operator wants to say, written where it is seen.
	// It carries markup through the policy in Clean, and a tone: plain, a
	// note, or a notice somebody should not miss.
	Text Kind = "text"

	// Search finds a report that already exists before somebody writes a new
	// one. The same search the heading can carry, as a module of its own for
	// operators who want it somewhere else.
	Search Kind = "search"

	// Boards is where to report: the boards, as cards or as a list, grouped
	// by organisation when there is more than one.
	Boards Kind = "boards"

	// Reports is a list of reports, and which list is its Show: the most
	// voted ideas, what was put right lately, the newest, or what is planned.
	// One module with a question rather than four that differ in one query.
	Reports Kind = "reports"

	// Roadmap is what is planned, in progress and done, in the three columns
	// a board's own roadmap uses.
	Roadmap Kind = "roadmap"

	// Statistics is how much came in and where it got to. It draws nothing
	// until there is something to count: three zeroes tell the first visitor
	// that nothing happens here.
	Statistics Kind = "statistics"

	// Links is a short list of places — a board's report form, its roadmap,
	// or an address outside — each drawn as a button or as a link.
	Links Kind = "links"

	// Picture is one image, with the words for somebody who cannot see it.
	Picture Kind = "image"
)

// usesData is the modules drawn from the database, whose content travels with
// the page. A page of headings, words, links and pictures reads neither the
// board table nor the item table when a stranger opens it.
func (k Kind) usesData() bool {
	switch k {
	case Boards, Reports, Roadmap, Statistics:
		return true
	}
	return false
}

// takesScope is the modules that can be narrowed to an organisation or a board.
func (k Kind) takesScope() bool { return k.usesData() }

// lists is the modules that can be told how many rows to show.
func (k Kind) lists() bool { return k == Boards || k == Reports }

// named is the modules that draw a header with their name. A heading is its own
// name, a paragraph and a picture have none, and a search box is a field.
func (k Kind) named() bool {
	switch k {
	case Boards, Reports, Roadmap, Statistics, Links:
		return true
	}
	return false
}

// Columns is how a band is divided, as the relative widths of its columns.
type Columns string

// The layouts a band may take. Six, because those are the arrangements a web
// page actually uses; a seventh would be a choice nobody can tell apart from
// one of these.
const (
	One        Columns = "1"
	Halves     Columns = "1-1"
	Thirds     Columns = "1-1-1"
	WideNarrow Columns = "2-1"
	NarrowWide Columns = "1-2"
	Quarters   Columns = "1-1-1-1"
)

// count is how many columns a layout has, and zero for a layout that is not
// one of them.
func (c Columns) count() int {
	switch c {
	case One:
		return 1
	case Halves, WideNarrow, NarrowWide:
		return 2
	case Thirds:
		return 3
	case Quarters:
		return 4
	}
	return 0
}

// The rest of a band's choices, each a short closed list. The empty value is
// the ordinary one, so a band nobody has touched says nothing about any of them.
const (
	BackgroundSoft   = "soft"
	BackgroundAccent = "accent"

	WidthFull = "full"

	SpacingCompact = "compact"
	SpacingAiry    = "airy"
)

// A module's frame. Card is the empty value: a box with a border, which is
// what a module looks like until somebody chooses otherwise.
const (
	StyleSoft  = "soft"
	StylePlain = "plain"
)

// A text module's tone.
const (
	ToneNote   = "note"
	ToneNotice = "notice"
)

// What a reports module shows. The empty value is the most voted ideas.
const (
	ShowMostVoted    = "most_voted"
	ShowRecentlyDone = "recently_done"
	ShowLatest       = "latest"
	ShowPlanned      = "planned"
)

// DisplayList draws a boards module as rows rather than cards.
const DisplayList = "list"

// AlignStart puts a heading against the start edge rather than in the middle.
const AlignStart = "start"

// Where a link goes.
const (
	// ToForm is a board's report form. Its Scope is "org/board".
	ToForm = "form"
	// ToBoard is a board's public page.
	ToBoard = "board"
	// ToRoadmap is a board's roadmap.
	ToRoadmap = "roadmap"
	// ToURL is an address outside this installation.
	ToURL = "url"
)

// Link is one place a links module, or the page's footer, points at.
type Link struct {
	Label  string `json:"label"`
	To     string `json:"to"`
	Scope  string `json:"scope,omitempty"`
	URL    string `json:"url,omitempty"`
	Button bool   `json:"button,omitempty"`
}

// Module is one block of a column.
//
// One shape for every kind, with the fields each kind reads. The alternative —
// a type per kind behind an interface — makes the JSON the browser sends
// ambiguous to decode and every change twice the work, and the validation below
// already says which fields belong to which kind.
type Module struct {
	ID   string `json:"id"`
	Kind Kind   `json:"kind"`

	// Name is the header a named module draws, and empty means the name of
	// what it shows. HideName draws no header at all.
	Name     string `json:"name,omitempty"`
	HideName bool   `json:"hide_name,omitempty"`

	// Style is the frame: card (empty), soft or plain.
	Style string `json:"style,omitempty"`

	// Body and Tone are a text module's.
	Body string `json:"body,omitempty"`
	Tone string `json:"tone,omitempty"`

	// Title, Subtitle, HideLogo, Align and WithSearch are a heading's. An
	// empty title and subtitle are the installation's name and tagline, so
	// the default page says who it belongs to without storing it twice.
	Title      string `json:"title,omitempty"`
	Subtitle   string `json:"subtitle,omitempty"`
	HideLogo   bool   `json:"hide_logo,omitempty"`
	Align      string `json:"align,omitempty"`
	WithSearch bool   `json:"with_search,omitempty"`

	// Scope narrows a data module to an organisation ("org") or a board
	// ("org/board"), and empty is everything. Rows is how many a list shows,
	// and zero is its own default. Show is which reports; Display is how the
	// boards are drawn.
	Scope   string `json:"scope,omitempty"`
	Rows    int    `json:"rows,omitempty"`
	Show    string `json:"show,omitempty"`
	Display string `json:"display,omitempty"`

	// Links is a links module's.
	Links []Link `json:"links,omitempty"`

	// Image, Alt and Caption are an image module's. Image is the address of a
	// picture this installation stores, as its upload answered.
	Image   string `json:"image,omitempty"`
	Alt     string `json:"alt,omitempty"`
	Caption string `json:"caption,omitempty"`
}

// Band is one horizontal stretch of the page.
type Band struct {
	ID string `json:"id"`

	// Columns is the layout, and Cells holds one list of modules per column,
	// in order, top to bottom.
	Columns Columns    `json:"columns"`
	Cells   [][]Module `json:"cells"`

	// Name is drawn as a pill on the band, and empty draws none — except on
	// the bands of the default page, whose names come from the visitor's
	// language. HideName is how one of those loses its pill: an empty name
	// there already means "the default one".
	Name     string `json:"name,omitempty"`
	HideName bool   `json:"hide_name,omitempty"`

	Background string `json:"background,omitempty"`
	Width      string `json:"width,omitempty"`
	Spacing    string `json:"spacing,omitempty"`
}

// Settings is what belongs to the page rather than to a band: how it reads
// when somebody shares its address, and the links in its footer.
type Settings struct {
	Title       string `json:"title,omitempty"`
	Description string `json:"description,omitempty"`
	ShareImage  string `json:"share_image,omitempty"`

	// The foot of the page: its links, one line of the operator's words, and
	// whether it carries the installation's name and mark and the credit.
	// Everything in it is the operator's to change — the default shows the
	// name and the credit, and a zero value keeps that default.
	Footer           []Link `json:"footer,omitempty"`
	FooterText       string `json:"footer_text,omitempty"`
	FooterHideBrand  bool   `json:"footer_hide_brand,omitempty"`
	FooterHideCredit bool   `json:"footer_hide_credit,omitempty"`
}

// Page is the whole front page.
type Page struct {
	Bands    []Band   `json:"bands"`
	Settings Settings `json:"settings"`
}

const (
	maxBands    = 12
	maxModules  = 24
	maxLinks    = 8
	maxTextBody = 4_000

	// The bounds on one-line words are in runes, not bytes: they are about
	// what fits on the screen, and a byte count would cut a Japanese title at
	// a third of the length of an English one.
	maxLabel    = 80
	maxTitle    = 120
	maxSubtitle = 240
	maxAlt      = 200
	maxDesc     = 200
	maxURL      = 500

	// maxRows is the most a list will show, the same bound the server uses
	// when it reads them.
	maxRows = 20
)

// ErrUnknownKind means a module nothing here can draw.
var ErrUnknownKind = errors.New("unknown module")

// Default is the front page of an installation that never composes one.
//
// It is not stored. An installation that has not touched its page gets this,
// read fresh every time, so a better default in a later release reaches every
// operator who never changed theirs.
//
// Its words are not here either. The heading is the installation's own name and
// tagline, and the band names and the three points are looked up by their ids
// in the catalogue the browser holds, in the visitor's language. A sentence in
// English stored in this function would be every page's language decided once.
//
// The order is the one a visitor needs: find what exists before writing it
// again, see where to report, see that people here are listened to, and only
// then — for the one arriving for the first time — how it works.
func Default() Page {
	return Page{Bands: []Band{
		{
			ID: "top", Columns: One, Background: BackgroundSoft, Spacing: SpacingAiry,
			Cells: [][]Module{{{ID: "heading", Kind: Heading, WithSearch: true, Style: StylePlain}}},
		},
		{
			ID: "where", Columns: One,
			Cells: [][]Module{{{ID: "where-list", Kind: Boards, HideName: true, Style: StylePlain}}},
		},
		{
			ID: "activity", Columns: Halves,
			Cells: [][]Module{
				{{ID: "most-voted", Kind: Reports, Show: ShowMostVoted, Rows: 5}},
				{{ID: "recently-done", Kind: Reports, Show: ShowRecentlyDone, Rows: 5}},
			},
		},
		{
			ID: "how", Columns: Thirds,
			Cells: [][]Module{
				{{ID: "point-1", Kind: Text}},
				{{ID: "point-2", Kind: Text}},
				{{ID: "point-3", Kind: Text}},
			},
		},
	}}
}

// Modules is every module on the page, in reading order: band by band, column
// by column, top to bottom.
func (p Page) Modules() []Module {
	var out []Module
	for _, b := range p.Bands {
		for _, cell := range b.Cells {
			out = append(out, cell...)
		}
	}
	return out
}

// NeedsData says whether drawing this page means reading the database. It
// lives here because it is a fact about what the modules are, and it cannot be
// tested from the response: the response is the same whether the query ran or
// not.
func (p Page) NeedsData() bool {
	for _, m := range p.Modules() {
		if m.Kind.usesData() {
			return true
		}
	}
	return false
}

// ShowsOne says whether a module was pointed at a single board, which decides
// how it draws: one board is that board, a set of them is a list. It follows
// what was asked for, not what came back, so a list does not turn into a card
// the day an installation happens to have one board.
func (m Module) ShowsOne() bool {
	_, _, deeper := strings.Cut(m.Scope, "/")
	return deeper
}

// Parse reads a stored page.
//
// Empty is the default page, which is every installation's until its operator
// changes something. A page stored in the grid format this replaced is the
// default page too: that format lived only in pre-release builds, and refusing
// to start over it, or converting it, would be work for installations that do
// not exist.
func Parse(stored string) (Page, error) {
	if strings.TrimSpace(stored) == "" {
		return Default(), nil
	}
	var page Page
	if err := json.Unmarshal([]byte(stored), &page); err != nil {
		return Page{}, fmt.Errorf("reading the page: %w", err)
	}
	if len(page.Bands) == 0 {
		return Default(), nil
	}
	page.cleanWords()
	if err := page.validate(); err != nil {
		return Page{}, err
	}
	return page, nil
}

// Submitted reads a page an administrator has just composed. It cleans the
// markup and validates exactly as Parse does, so there is one way in.
func Submitted(r io.Reader) (Page, error) {
	var page Page
	if err := json.NewDecoder(r).Decode(&page); err != nil {
		return Page{}, fmt.Errorf("reading the page: %w", err)
	}
	page.cleanWords()
	if err := page.validate(); err != nil {
		return Page{}, err
	}
	return page, nil
}

// Encode is the other direction, and validates for the same reason: the only
// way a bad page reaches the database is through here.
func (p Page) Encode() (string, error) {
	if err := p.validate(); err != nil {
		return "", err
	}
	out, err := json.Marshal(p)
	if err != nil {
		return "", fmt.Errorf("writing the page: %w", err)
	}
	return string(out), nil
}

// cleanWords runs the markup policy over every text module, on the way in and
// on the way out of the database: neither what a browser sent nor what an
// earlier version stored is a reason to put unchecked markup on a public page.
func (p *Page) cleanWords() {
	for bi := range p.Bands {
		for ci := range p.Bands[bi].Cells {
			for mi := range p.Bands[bi].Cells[ci] {
				m := &p.Bands[bi].Cells[ci][mi]
				if m.Kind != Text || m.Body == "" {
					continue
				}
				cleaned, err := Clean(m.Body)
				if err != nil {
					// Markup that cannot be parsed keeps nothing: the
					// alternative is putting whatever it was on the page.
					m.Body = ""
					continue
				}
				m.Body = cleaned
			}
		}
	}
}

func (p Page) validate() error {
	if len(p.Bands) == 0 {
		return errors.New("a page needs at least one band")
	}
	if len(p.Bands) > maxBands {
		return fmt.Errorf("a page may have at most %d bands", maxBands)
	}

	seen := map[string]bool{}
	claim := func(id, what string) error {
		if strings.TrimSpace(id) == "" {
			return fmt.Errorf("a %s has no id", what)
		}
		if len(id) > 64 {
			return fmt.Errorf("the %s id %q is too long", what, id)
		}
		// One id space for bands and modules: the editor finds either by id,
		// and two things answering to one name is a page where moving one
		// moves the other.
		if seen[id] {
			return fmt.Errorf("two things share the id %q", id)
		}
		seen[id] = true
		return nil
	}

	modules := 0
	for _, b := range p.Bands {
		if err := claim(b.ID, "band"); err != nil {
			return err
		}
		if err := b.validate(); err != nil {
			return err
		}
		for _, cell := range b.Cells {
			for _, m := range cell {
				modules++
				if err := claim(m.ID, "module"); err != nil {
					return err
				}
				if err := m.validate(); err != nil {
					return err
				}
			}
		}
	}
	if modules > maxModules {
		return fmt.Errorf("a page may have at most %d modules", maxModules)
	}
	return p.Settings.validate()
}

func (b Band) validate() error {
	n := b.Columns.count()
	if n == 0 {
		return fmt.Errorf("band %q has the layout %q, which is not one of the six", b.ID, b.Columns)
	}
	// A cell for every column and no more. A column with nothing in it is
	// allowed — it is where the editor offers to add one — but a module in a
	// column the layout does not have is a module nobody would ever see.
	if len(b.Cells) != n {
		return fmt.Errorf("band %q has %d columns and %d cells", b.ID, n, len(b.Cells))
	}
	if !oneOf(b.Background, "", BackgroundSoft, BackgroundAccent) {
		return fmt.Errorf("band %q has the background %q", b.ID, b.Background)
	}
	if !oneOf(b.Width, "", WidthFull) {
		return fmt.Errorf("band %q has the width %q", b.ID, b.Width)
	}
	if !oneOf(b.Spacing, "", SpacingCompact, SpacingAiry) {
		return fmt.Errorf("band %q has the spacing %q", b.ID, b.Spacing)
	}
	if runes(b.Name) > maxLabel {
		return fmt.Errorf("band %q has a name longer than %d characters", b.ID, maxLabel)
	}
	return nil
}

func (m Module) validate() error {
	switch m.Kind {
	case Heading, Text, Search, Boards, Reports, Roadmap, Statistics, Links, Picture:
	default:
		return fmt.Errorf("%w: %q", ErrUnknownKind, m.Kind)
	}

	if !oneOf(m.Style, "", StyleSoft, StylePlain) {
		return fmt.Errorf("module %q has the style %q", m.ID, m.Style)
	}
	if (m.Name != "" || m.HideName) && !m.Kind.named() {
		return fmt.Errorf("module %q is given a name it does not draw", m.ID)
	}
	if runes(m.Name) > maxLabel {
		return fmt.Errorf("module %q has a name longer than %d characters", m.ID, maxLabel)
	}

	// Each field belongs to the kinds that read it. A field on a kind that
	// ignores it is somebody expecting a page they are not going to get.
	if (m.Body != "" || m.Tone != "") && m.Kind != Text {
		return fmt.Errorf("module %q carries words it has nowhere to put", m.ID)
	}
	if len(m.Body) > maxTextBody {
		return fmt.Errorf("module %q is longer than %d characters", m.ID, maxTextBody)
	}
	if !oneOf(m.Tone, "", ToneNote, ToneNotice) {
		return fmt.Errorf("module %q has the tone %q", m.ID, m.Tone)
	}

	heading := m.Title != "" || m.Subtitle != "" || m.HideLogo || m.Align != "" || m.WithSearch
	if heading && m.Kind != Heading {
		return fmt.Errorf("module %q carries a heading's settings and is not a heading", m.ID)
	}
	if runes(m.Title) > maxTitle || runes(m.Subtitle) > maxSubtitle {
		return fmt.Errorf("module %q has a title or a subtitle that is too long", m.ID)
	}
	if !oneOf(m.Align, "", AlignStart) {
		return fmt.Errorf("module %q is aligned %q", m.ID, m.Align)
	}

	if err := m.validateScope(); err != nil {
		return err
	}
	if m.Rows < 0 || m.Rows > maxRows {
		return fmt.Errorf("module %q asks for %d rows", m.ID, m.Rows)
	}
	if m.Rows != 0 && !m.Kind.lists() {
		return fmt.Errorf("module %q asks for rows and does not list anything", m.ID)
	}
	if m.Show != "" && m.Kind != Reports {
		return fmt.Errorf("module %q chooses which reports and does not show reports", m.ID)
	}
	if !oneOf(m.Show, "", ShowMostVoted, ShowRecentlyDone, ShowLatest, ShowPlanned) {
		return fmt.Errorf("module %q shows %q", m.ID, m.Show)
	}
	if m.Display != "" && m.Kind != Boards {
		return fmt.Errorf("module %q chooses a display and is not a list of boards", m.ID)
	}
	if !oneOf(m.Display, "", DisplayList) {
		return fmt.Errorf("module %q is displayed as %q", m.ID, m.Display)
	}

	if len(m.Links) > 0 && m.Kind != Links {
		return fmt.Errorf("module %q carries links and is not a links module", m.ID)
	}
	if err := validateLinks(m.ID, m.Links); err != nil {
		return err
	}

	picture := m.Image != "" || m.Alt != "" || m.Caption != ""
	if picture && m.Kind != Picture {
		return fmt.Errorf("module %q carries a picture and is not an image", m.ID)
	}
	if m.Kind == Picture {
		// A picture needs its words. The module can be placed empty — that
		// is where the editor asks for one — but a picture without a
		// description is a hole in the page for whoever cannot see it.
		if m.Image != "" && strings.TrimSpace(m.Alt) == "" {
			return fmt.Errorf("image %q has no description", m.ID)
		}
		if m.Image != "" && !imageName.MatchString(m.Image) {
			return fmt.Errorf("image %q names %q, which is not a stored picture", m.ID, m.Image)
		}
	}
	if runes(m.Alt) > maxAlt || runes(m.Caption) > maxAlt {
		return fmt.Errorf("module %q has a description or a caption that is too long", m.ID)
	}
	return nil
}

// validateScope checks what a data module was pointed at. The shape only:
// whether that board still exists is not decided here, because it can be
// deleted long after the page was composed, and the page has to survive it.
func (m Module) validateScope() error {
	if m.Scope == "" {
		return nil
	}
	if !m.Kind.takesScope() {
		return fmt.Errorf("module %q names a place and is not a module that shows one", m.ID)
	}
	if !isScope(m.Scope, false) {
		return fmt.Errorf("module %q names %q, which is not an organisation or a board", m.ID, m.Scope)
	}
	return nil
}

func validateLinks(owner string, links []Link) error {
	if len(links) > maxLinks {
		return fmt.Errorf("%q has more than %d links", owner, maxLinks)
	}
	for i, l := range links {
		if strings.TrimSpace(l.Label) == "" || runes(l.Label) > maxLabel {
			return fmt.Errorf("link %d of %q needs a label of at most %d characters", i+1, owner, maxLabel)
		}
		switch l.To {
		case ToForm, ToBoard, ToRoadmap:
			// A form, a board and a roadmap are one board each: an
			// organisation is not a place any of them can open.
			if !isScope(l.Scope, true) || l.URL != "" {
				return fmt.Errorf("link %d of %q must name one board", i+1, owner)
			}
		case ToURL:
			if l.Scope != "" || !isWebAddress(l.URL) {
				return fmt.Errorf("link %d of %q needs an http or https address", i+1, owner)
			}
		default:
			return fmt.Errorf("link %d of %q goes to %q", i+1, owner, l.To)
		}
	}
	return nil
}

func (s Settings) validate() error {
	if runes(s.Title) > maxTitle {
		return fmt.Errorf("the page title is longer than %d characters", maxTitle)
	}
	if runes(s.Description) > maxDesc {
		return fmt.Errorf("the page description is longer than %d characters", maxDesc)
	}
	if s.ShareImage != "" && !imageName.MatchString(s.ShareImage) {
		return fmt.Errorf("the share image %q is not a stored picture", s.ShareImage)
	}
	if runes(s.FooterText) > maxSubtitle {
		return fmt.Errorf("the footer text is longer than %d characters", maxSubtitle)
	}
	return validateLinks("footer", s.Footer)
}

// isWebAddress accepts an absolute http or https address and nothing else.
// Anything that reaches an href on a public page is refused unless it is
// exactly this: javascript:, data: and friends are how a link becomes a script.
func isWebAddress(v string) bool {
	if v == "" || len(v) > maxURL {
		return false
	}
	u, err := url.Parse(v)
	if err != nil {
		return false
	}
	return (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}

// isScope is "org" or "org/board", and "org/board" alone when a board is needed.
func isScope(v string, boardOnly bool) bool {
	org, board, deeper := strings.Cut(v, "/")
	if boardOnly && !deeper {
		return false
	}
	return isSlug(org) && (!deeper || isSlug(board))
}

// isSlug is the shape of the slugs the rest of the product hands out.
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

// imageName is the address a stored picture is served at — what an upload
// answers with, and what a page keeps. Built from the one constant the server
// serves under, so the two cannot drift: a copy of that path here is a picture
// that uploads and then disappears the first time the page is read back.
var imageName = regexp.MustCompile(`^` + regexp.QuoteMeta(ImagePrefix) + `[0-9a-f]{32}\.webp$`)

func oneOf(v string, allowed ...string) bool {
	for _, a := range allowed {
		if v == a {
			return true
		}
	}
	return false
}

func runes(v string) int { return len([]rune(v)) }
