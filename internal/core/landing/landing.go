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
)

// Width is how much of the row a module takes.
//
// Deliberately coarse. Pixel geometry is what the next piece of this is for,
// and a first version that stored exact coordinates would have to migrate them
// the day the grid arrives.
type Width string

const (
	Full Width = "full"
	Half Width = "half"
)

const (
	maxModules  = 24
	maxTextBody = 4_000
)

// Module is one block on the page.
type Module struct {
	ID    string `json:"id"`
	Kind  Kind   `json:"kind"`
	Width Width  `json:"width"`

	// Body is the words of a Text module, and empty for every other kind.
	Body string `json:"body,omitempty"`
}

// Layout is the whole page, in order.
type Layout struct {
	Modules []Module `json:"modules"`
}

// Default is the page as it was before anybody could compose it.
//
// It matters that this is exactly what the old page showed: an installation
// that never opens the editor must not notice that the editor exists.
func Default() Layout {
	return Layout{Modules: []Module{
		{ID: "hero", Kind: Hero, Width: Full},
		{ID: "intro", Kind: Intro, Width: Full},
		{ID: "organisations", Kind: Organisations, Width: Full},
	}}
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
		case Hero, Intro, Organisations, Text:
		default:
			return fmt.Errorf("%w: %q", ErrUnknownKind, m.Kind)
		}
		switch m.Width {
		case Full, Half:
		default:
			return fmt.Errorf("module %q has an unknown width %q", m.ID, m.Width)
		}
		if m.Kind != Text && m.Body != "" {
			return fmt.Errorf("module %q carries words and is not a text module", m.ID)
		}
		if len(m.Body) > maxTextBody {
			return fmt.Errorf("module %q is longer than %d characters", m.ID, maxTextBody)
		}
	}
	return nil
}
