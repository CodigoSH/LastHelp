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
	_, err := Parse(`{"modules":[{"id":"a","kind":"iframe","x":1,"y":1,"w":12,"h":2}]}`)
	if !errors.Is(err, ErrUnknownKind) {
		t.Errorf("an unknown module came back as %v, want ErrUnknownKind", err)
	}

	// And one bad module refuses the whole layout, not just itself.
	mixed, err := Parse(`{"modules":[
		{"id":"a","kind":"hero","x":1,"y":1,"w":12,"h":4},
		{"id":"b","kind":"iframe","x":1,"y":5,"w":12,"h":2}]}`)
	if err == nil {
		t.Errorf("a layout with one bad module was accepted as %d modules", len(mixed.Modules))
	}
}

func TestTheThingsAPageCannotBe(t *testing.T) {
	for _, tc := range []struct{ name, layout string }{
		{"no modules at all", `{"modules":[]}`},
		{"a module with no id", `{"modules":[{"id":"","kind":"hero","x":1,"y":1,"w":12,"h":4}]}`},
		{"two modules sharing an id", `{"modules":[
			{"id":"a","kind":"hero","x":1,"y":1,"w":12,"h":4},
			{"id":"a","kind":"intro","x":1,"y":5,"w":12,"h":2}]}`},
		{"a module off the right-hand edge", `{"modules":[{"id":"a","kind":"hero","x":9,"y":1,"w":8,"h":2}]}`},
		{"a module narrower than the smallest", `{"modules":[{"id":"a","kind":"hero","x":1,"y":1,"w":1,"h":2}]}`},
		{"two modules in the same cells", `{"modules":[
			{"id":"a","kind":"hero","x":1,"y":1,"w":6,"h":2},
			{"id":"b","kind":"intro","x":4,"y":1,"w":6,"h":2}]}`},
		{"words on a module that has none", `{"modules":[{"id":"a","kind":"hero","x":1,"y":1,"w":12,"h":4,"body":"hello"}]}`},
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
		{ID: "words", Kind: Text, X: 1, Y: 1, W: 6, H: 2, Body: "We read everything."},
		{ID: "orgs", Kind: Organisations, X: 7, Y: 1, W: 6, H: 2},
		{ID: "top", Kind: Hero, X: 1, Y: 3, W: 12, H: 4},
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
	bad := Layout{Modules: []Module{{ID: "a", Kind: "carousel", X: 1, Y: 1, W: 12, H: 2}}}
	if _, err := bad.Encode(); err == nil {
		t.Fatal("an unknown module was written")
	}

	tooLong := Layout{Modules: []Module{{ID: "a", Kind: Text, X: 1, Y: 1, W: 12, H: 2, Body: strings.Repeat("x", maxTextBody+1)}}}
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
		{ID: "a", Kind: Text, X: 1, Y: 1, W: 12, H: 2, Body: sneaky},
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

// TestAPageComposedBeforeThereWasAGridStillComesBack.
//
// The first version of this stored a coarse width and no position. Those
// layouts exist in installations that arranged a page between one release and
// the next, and the grid arriving must not turn somebody's front page into an
// error. Full becomes the twelve columns it meant, half becomes six, and they
// keep the order they were written in.
func TestAPageComposedBeforeThereWasAGridStillComesBack(t *testing.T) {
	old := `{"modules":[
		{"id":"words","kind":"text","width":"half","body":"We read everything."},
		{"id":"orgs","kind":"organisations","width":"full"}]}`

	got, err := Parse(old)
	if err != nil {
		t.Fatalf("a page composed before the grid came back as an error: %v", err)
	}
	if len(got.Modules) != 2 {
		t.Fatalf("%d modules came back, want 2", len(got.Modules))
	}

	words, orgs := got.Modules[0], got.Modules[1]
	if words.W != Columns/2 {
		t.Errorf("half became %d columns, want %d", words.W, Columns/2)
	}
	if orgs.W != Columns {
		t.Errorf("full became %d columns, want %d", orgs.W, Columns)
	}
	if words.Body != "We read everything." {
		t.Errorf("the words were lost on the way: %q", words.Body)
	}
	if !(words.Y < orgs.Y) {
		t.Errorf("the order changed: words at row %d, organisations at row %d", words.Y, orgs.Y)
	}
	if words.Overlaps(orgs) {
		t.Error("the two ended up on top of each other")
	}

	// And once saved again it is an ordinary layout: the old width is gone and
	// nothing has to guess a second time.
	again, err := got.Encode()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(again, `"width"`) {
		t.Errorf("the old width was written back out: %s", again)
	}
	reread, err := Parse(again)
	if err != nil {
		t.Fatal(err)
	}
	if reread.Modules[0] != got.Modules[0] {
		t.Errorf("a second read moved it: %+v then %+v", got.Modules[0], reread.Modules[0])
	}
}
