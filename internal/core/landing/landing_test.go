package landing

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

// TestAnUnsetLayoutIsThePageThatWasThereBefore.
//
// The condition for shipping this at all: an installation that never opens the
// editor must not notice it exists. If the default drifted from what the page
// used to be, every existing instance would find its front page rearranged by
// an update it did not ask for.
func TestAnUnsetLayoutIsThePageThatWasThereBefore(t *testing.T) {
	for _, stored := range []string{"", "   ", "\n"} {
		got, err := Parse(stored)
		if err != nil {
			t.Fatalf("%q: %v", stored, err)
		}
		if len(got.Modules) != 3 {
			t.Fatalf("%q gave %d modules, want 3", stored, len(got.Modules))
		}
		for i, want := range []Kind{Hero, Intro, Organisations} {
			if got.Modules[i].Kind != want {
				t.Errorf("%q: module %d is %q, want %q", stored, i+1, got.Modules[i].Kind, want)
			}
		}
	}
}

// TestAModuleNobodyCanDrawIsRefused, rather than quietly dropped.
//
// This arrives over HTTP and renders on the page strangers see. Silently
// leaving out what was not understood would mean an operator saving a page,
// seeing part of it, and having no way to tell whether the rest failed to save
// or failed to draw.
func TestAModuleNobodyCanDrawIsRefused(t *testing.T) {
	_, err := Parse(`{"modules":[{"id":"a","kind":"iframe","width":"full"}]}`)
	if !errors.Is(err, ErrUnknownKind) {
		t.Errorf("an unknown module came back as %v, want ErrUnknownKind", err)
	}

	// And one bad module refuses the whole layout, not just itself.
	mixed, err := Parse(`{"modules":[
		{"id":"a","kind":"hero","width":"full"},
		{"id":"b","kind":"iframe","width":"full"}]}`)
	if err == nil {
		t.Errorf("a layout with one bad module was accepted as %d modules", len(mixed.Modules))
	}
}

func TestTheThingsAPageCannotBe(t *testing.T) {
	for _, tc := range []struct{ name, layout string }{
		{"no modules at all", `{"modules":[]}`},
		{"a module with no id", `{"modules":[{"id":"","kind":"hero","width":"full"}]}`},
		{"two modules sharing an id", `{"modules":[
			{"id":"a","kind":"hero","width":"full"},
			{"id":"a","kind":"intro","width":"full"}]}`},
		{"a width nothing can lay out", `{"modules":[{"id":"a","kind":"hero","width":"three-quarters"}]}`},
		{"words on a module that has none", `{"modules":[{"id":"a","kind":"hero","width":"full","body":"hello"}]}`},
		{"not json at all", `hello`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := Parse(tc.layout); err == nil {
				t.Error("accepted")
			}
		})
	}
}

// TestALayoutSurvivesTheRoundTrip: what was saved is what comes back, in order.
func TestALayoutSurvivesTheRoundTrip(t *testing.T) {
	original := Layout{Modules: []Module{
		{ID: "words", Kind: Text, Width: Half, Body: "We read everything."},
		{ID: "orgs", Kind: Organisations, Width: Half},
		{ID: "top", Kind: Hero, Width: Full},
	}}

	stored, err := original.Encode()
	if err != nil {
		t.Fatal(err)
	}
	back, err := Parse(stored)
	if err != nil {
		t.Fatal(err)
	}
	if len(back.Modules) != len(original.Modules) {
		t.Fatalf("%d modules came back, want %d", len(back.Modules), len(original.Modules))
	}
	for i := range original.Modules {
		if back.Modules[i] != original.Modules[i] {
			t.Errorf("module %d came back as %+v, want %+v", i+1, back.Modules[i], original.Modules[i])
		}
	}
}

// TestEncodeRefusesWhatParseWouldRefuse. The two are one rule, not two: a
// layout that can be written and not read is a page that breaks on the next
// visit rather than when it was saved.
func TestEncodeRefusesWhatParseWouldRefuse(t *testing.T) {
	bad := Layout{Modules: []Module{{ID: "a", Kind: "carousel", Width: Full}}}
	if _, err := bad.Encode(); err == nil {
		t.Fatal("an unknown module was written")
	}

	tooLong := Layout{Modules: []Module{{ID: "a", Kind: Text, Width: Full, Body: strings.Repeat("x", maxTextBody+1)}}}
	if _, err := tooLong.Encode(); err == nil {
		t.Error("a text module longer than the limit was written")
	}
}

// TestTextIsStoredAsWordsAndNotAsMarkup.
//
// Plain text is the decision, and this is what holds it: the body goes in and
// comes out unchanged, tags and all, so whatever renders it has to escape them.
// The day somebody makes this module accept markup, this test is the one that
// has to be argued with.
func TestTextIsStoredAsWordsAndNotAsMarkup(t *testing.T) {
	const sneaky = `<script>alert(1)</script>`
	stored, err := Layout{Modules: []Module{
		{ID: "a", Kind: Text, Width: Full, Body: sneaky},
	}}.Encode()
	if err != nil {
		t.Fatal(err)
	}

	// Stored as data, not as a fragment of a page.
	var raw map[string]any
	if err := json.Unmarshal([]byte(stored), &raw); err != nil {
		t.Fatal(err)
	}
	back, err := Parse(stored)
	if err != nil {
		t.Fatal(err)
	}
	if back.Modules[0].Body != sneaky {
		t.Errorf("the words changed on the way through: %q", back.Modules[0].Body)
	}
}
