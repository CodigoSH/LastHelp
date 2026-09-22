package landing

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

// TestAnUnsetLayoutIsStillAFrontPage.
//
// An installation that never opens the editor has a front page: it just never
// chose one. The alternative — an empty grid until somebody composes something
// — would mean a fresh instance greeting its first visitor with nothing.
//
// This used to promise that the default was pixel for pixel the page that came
// before the grid. It is not any more, deliberately: the front page was rebuilt
// on the dashboard's grid, and a promise to keep looking like the thing being
// replaced is not a promise worth keeping.
func TestAnUnsetLayoutIsStillAFrontPage(t *testing.T) {
	for _, stored := range []string{"", "   ", "\n"} {
		got, err := Parse(stored)
		if err != nil {
			t.Fatalf("%q: %v", stored, err)
		}
		if len(got.Modules) != 5 {
			t.Fatalf("%q gave %d modules, want 5", stored, len(got.Modules))
		}
		// Three points, one module each: the page says the same three things it
		// always did, and every one of them can be moved, rewritten or removed.
		for i, want := range []Kind{Hero, Intro, Intro, Intro, Organisations} {
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
		// A negative width, not a zero one: one column is the narrowest a module
		// may be now, and a zero is not refusable at all — it is how a layout
		// written before there were coordinates says "no width given", and it
		// gets filled in rather than thrown out.
		{"a module of negative width", `{"modules":[{"id":"a","kind":"hero","x":1,"y":1,"w":-1,"h":2}]}`},
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

// TestAModuleThatShowsBoardsCanBeToldWhichOnes.
//
// The scope is what makes these worth having on an installation with more than
// one organisation: without it, every "most voted" on the page shows the same
// thing. The shape is checked, the existence is not — a board can be deleted
// long after somebody composed a page that names it, and the page has to
// survive that rather than refuse to load.
func TestAModuleThatShowsBoardsCanBeToldWhichOnes(t *testing.T) {
	for _, scope := range []string{"", "acme/atlas", "a-b_c/d0"} {
		in := `{"modules":[{"id":"v","kind":"most_voted","x":1,"y":1,"w":12,"h":3,"scope":"` + scope + `"}]}`
		got, err := Parse(in)
		if err != nil {
			t.Fatalf("scope %q was refused: %v", scope, err)
		}
		if got.Modules[0].Scope != scope {
			t.Errorf("scope %q came back as %q", scope, got.Modules[0].Scope)
		}
	}
}

// TestAScopeThatNamesNothingUsableIsRefused.
//
// Both halves matter. A scope the reader cannot take apart would leave the
// module drawing every board instead of the one it names, and a scope on a
// module that cannot use one is somebody expecting a narrower page than they
// are going to get. Neither is something to discover by looking at the page.
func TestAScopeThatNamesNothingUsableIsRefused(t *testing.T) {
	cases := map[string]string{
		// A button goes to one report form, so an organisation is not an answer
		// it can act on. Every other module narrows to a set, and one
		// organisation is a set — so "acme" on a boards module is fine now, and
		// this case used to be that, refused.
		"an organisation where one board is needed": `{"modules":[{"id":"v","kind":"call_to_action",` +
			`"x":1,"y":1,"w":4,"h":2,"scope":"acme"}]}`,
		"empty half":          `{"modules":[{"id":"v","kind":"boards","x":1,"y":1,"w":12,"h":3,"scope":"acme/"}]}`,
		"not a slug":          `{"modules":[{"id":"v","kind":"boards","x":1,"y":1,"w":12,"h":3,"scope":"Acme/At las"}]}`,
		"a path, not a board": `{"modules":[{"id":"v","kind":"boards","x":1,"y":1,"w":12,"h":3,"scope":"../../etc"}]}`,
		"on a module with no use for one": `{"modules":[{"id":"t","kind":"text","x":1,"y":1,"w":12,"h":2,` +
			`"body":"hello","scope":"acme/atlas"}]}`,
	}
	for name, in := range cases {
		if _, err := Parse(in); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

// TestAPageOfWordsSaysItNeedsNothingFromTheDatabase.
//
// The front page is the one screen every stranger loads, and most installations
// will never put a data module on it. This predicate is what keeps a page of a
// mark, some words and a list of organisations from costing a board query and
// an item query on every visit, so it is the thing to guard: the response looks
// the same either way, which is exactly why reading the answer off the response
// proves nothing.
func TestAPageOfWordsSaysItNeedsNothingFromTheDatabase(t *testing.T) {
	if Default().NeedsData() {
		t.Error("the default page asked the database for something")
	}
	words := Layout{Modules: []Module{
		{ID: "t", Kind: Text, X: 1, Y: 1, W: 12, H: 2, Body: "hello"},
		{ID: "h", Kind: Hero, X: 1, Y: 3, W: 12, H: 3},
	}}
	if words.NeedsData() {
		t.Error("a page of words asked the database for something")
	}
	for _, kind := range []Kind{Boards, MostVoted} {
		page := Layout{Modules: []Module{{ID: "d", Kind: kind, X: 1, Y: 1, W: 12, H: 3}}}
		if !page.NeedsData() {
			t.Errorf("a page holding %q said it needed nothing", kind)
		}
	}
}

// TestAButtonsLabelIsBoundedAsALabel.
//
// The words of a paragraph and the words on a button are not the same bound.
// Four thousand characters is a reasonable paragraph and an unusable button, so
// the button has its own — counted in runes, because a bound about what fits on
// a button that counted bytes would cut a Japanese label at a third of the
// length of an English one.
func TestAButtonsLabelIsBoundedAsALabel(t *testing.T) {
	page := func(label string) string {
		out, _ := json.Marshal(Layout{Modules: []Module{
			{ID: "go", Kind: CallToAction, X: 1, Y: 1, W: 4, H: 1,
				Scope: "acme/atlas", Body: label},
		}})
		return string(out)
	}

	if _, err := Parse(page(strings.Repeat("ぜ", maxLabel))); err != nil {
		t.Errorf("a label of %d characters was refused: %v", maxLabel, err)
	}
	if _, err := Parse(page(strings.Repeat("ぜ", maxLabel+1))); err == nil {
		t.Error("a label past the bound was accepted")
	}
	// The same label in bytes is three times over a byte bound, and has to be
	// accepted: this is the case a byte count gets wrong.
	if _, err := Parse(page(strings.Repeat("ぜ", maxLabel))); err != nil {
		t.Errorf("a label of %d runes was measured in bytes: %v", maxLabel, err)
	}
}

// TestOnlyWhatHasSomewhereToPutWordsMayCarryThem.
//
// A module carrying words it cannot draw is somebody's writing that vanished,
// with nothing on the page to say where it went. Two kinds have somewhere to
// put them — the operator's own paragraph and the label on the button — and the
// rest are refused rather than quietly dropped.
func TestOnlyWhatHasSomewhereToPutWordsMayCarryThem(t *testing.T) {
	page := func(m Module) string {
		out, _ := json.Marshal(Layout{Modules: []Module{m}})
		return string(out)
	}

	for _, kind := range []Kind{Hero, Organisations, Boards, MostVoted, RecentlyDone} {
		m := Module{ID: "m", Kind: kind, X: 1, Y: 1, W: 12, H: 2, Body: "hello"}
		if _, err := Parse(page(m)); err == nil {
			t.Errorf("%q was allowed to carry words it cannot draw", kind)
		}
	}

	// The intro carries words now: it used to draw three fixed sentences and
	// draws one of the operator's instead.
	if _, err := Parse(page(Module{ID: "p", Kind: Intro, X: 1, Y: 1, W: 4, H: 1,
		Body: "Reporting requires no account."})); err != nil {
		t.Errorf("a point could not carry its own words: %v", err)
	}

	if _, err := Parse(page(Module{ID: "t", Kind: Text, X: 1, Y: 1, W: 12, H: 2, Body: "hello"})); err != nil {
		t.Errorf("a text module could not carry its own words: %v", err)
	}
	if _, err := Parse(page(Module{ID: "go", Kind: CallToAction, X: 1, Y: 1, W: 4, H: 1,
		Scope: "acme/atlas", Body: "Report something"})); err != nil {
		t.Errorf("a button could not carry its own label: %v", err)
	}
}

// TestWhereAModuleKeepsItsName.
//
// Four places, and the empty value is the top — so a page composed before there
// was a choice keeps the look it had rather than losing its headings to a zero
// value nobody wrote.
func TestWhereAModuleKeepsItsName(t *testing.T) {
	page := func(m Module) string {
		out, _ := json.Marshal(Layout{Modules: []Module{m}})
		return string(out)
	}

	for _, at := range []LabelAt{"", LabelTop, LabelBottom, LabelPill, LabelOff} {
		m := Module{ID: "b", Kind: Boards, X: 1, Y: 1, W: 12, H: 3, Label: at}
		got, err := Parse(page(m))
		if err != nil {
			t.Fatalf("%q was refused: %v", at, err)
		}
		if got.Modules[0].Label != at {
			t.Errorf("%q came back as %q", at, got.Modules[0].Label)
		}
	}

	// Somewhere that is not one of the four.
	bad := Module{ID: "b", Kind: Boards, X: 1, Y: 1, W: 12, H: 3, Label: "middle"}
	if _, err := Parse(page(bad)); err == nil {
		t.Error("a module was allowed to put its name nowhere")
	}

	// And a module that draws no name is not asked where to put one: a hero is
	// its own name, a paragraph has none. Accepting it would store a setting
	// whose dialog never shows it and nothing ever reads.
	for _, kind := range []Kind{Hero, Intro, Text, CallToAction} {
		m := Module{ID: "m", Kind: kind, X: 1, Y: 1, W: 12, H: 2, Label: LabelPill}
		if kind == Text || kind == CallToAction {
			m.Body = "hello"
		}
		if kind == CallToAction {
			m.Scope = "acme/atlas"
		}
		if _, err := Parse(page(m)); err == nil {
			t.Errorf("%q was asked where to put a name it does not draw", kind)
		}
	}
}

// TestASectionHoldsModulesInItsOwnGrid.
//
// A child's coordinates belong to its section, not to the page. That is what
// lets a section be dragged somewhere else without rewriting everything inside
// it, and it is also the thing most likely to be got wrong later: the two kinds
// of coordinate look identical in the stored layout.
func TestASectionHoldsModulesInItsOwnGrid(t *testing.T) {
	page := func(ms ...Module) string {
		out, _ := json.Marshal(Layout{Modules: ms})
		return string(out)
	}
	section := Module{ID: "group", Kind: Section, X: 1, Y: 1, W: 6, H: 2, Body: "My projects"}

	// A child at the section's own top-left, which in the page's coordinates is
	// where the section itself begins. Both are 1,1 and both are correct.
	inside := Module{ID: "a", Kind: Boards, X: 1, Y: 1, W: 3, H: 1, Parent: "group"}
	if _, err := Parse(page(section, inside)); err != nil {
		t.Fatalf("a child at its section's first cell was refused: %v", err)
	}

	// Two children side by side inside a six-wide section. On the page they
	// would be nowhere near the section's own column six.
	second := Module{ID: "b", Kind: MostVoted, X: 4, Y: 1, W: 3, H: 1, Parent: "group"}
	if _, err := Parse(page(section, inside, second)); err != nil {
		t.Fatalf("two children sharing a section were refused: %v", err)
	}

	// And a child that would stick out of it.
	past := Module{ID: "c", Kind: Boards, X: 5, Y: 1, W: 3, H: 1, Parent: "group"}
	if _, err := Parse(page(section, past)); err == nil {
		t.Error("a child was allowed to stick out of its section")
	}

	// Children of one section may not sit on each other.
	clash := Module{ID: "d", Kind: Boards, X: 2, Y: 1, W: 3, H: 1, Parent: "group"}
	if _, err := Parse(page(section, inside, clash)); err == nil {
		t.Error("two children were allowed to share a cell")
	}
}

// TestAChildAndAPageModuleAreNotMeasuredAgainstEachOther.
//
// The one that would be easy to get wrong in the other direction: a child at
// 1,1 of its section and a module at 1,1 of the page are not on top of each
// other, and refusing that pair would refuse almost every page with a section
// on it.
func TestAChildAndAPageModuleAreNotMeasuredAgainstEachOther(t *testing.T) {
	out, _ := json.Marshal(Layout{Modules: []Module{
		{ID: "top", Kind: Hero, X: 1, Y: 1, W: 12, H: 3},
		{ID: "group", Kind: Section, X: 1, Y: 4, W: 6, H: 2},
		{ID: "a", Kind: Boards, X: 1, Y: 1, W: 3, H: 1, Parent: "group"},
	}})
	if _, err := Parse(string(out)); err != nil {
		t.Errorf("a child was measured against the page: %v", err)
	}
}

// TestASectionDoesNotGoInsideASection.
//
// Two levels is a layout somebody arranged; three is a layout nobody can drag,
// and every rule about coordinates would need a third set.
func TestASectionDoesNotGoInsideASection(t *testing.T) {
	out, _ := json.Marshal(Layout{Modules: []Module{
		{ID: "outer", Kind: Section, X: 1, Y: 1, W: 8, H: 4},
		{ID: "inner", Kind: Section, X: 1, Y: 1, W: 2, H: 1, Parent: "outer"},
	}})
	if _, err := Parse(string(out)); err == nil {
		t.Error("a section was nested inside another")
	}
}

// TestAModuleInsideSomethingThatIsNotThere.
//
// A parent that was deleted, or a section that never existed. Refused rather
// than drawn on the page at coordinates belonging to a grid that is not there.
func TestAModuleInsideSomethingThatIsNotThere(t *testing.T) {
	gone, _ := json.Marshal(Layout{Modules: []Module{
		{ID: "a", Kind: Boards, X: 1, Y: 1, W: 2, H: 1, Parent: "group"},
	}})
	_, err := Parse(string(gone))
	if err == nil {
		t.Fatal("a module was allowed inside a section that is not on the page")
	}
	// And it has to SAY that, which is the whole of this guard: a missing
	// parent measures nothing by nothing, so the bounds check refuses it too —
	// with "sticks out of the section holding it", about a section that is not
	// there. Removing the guard leaves the layout refused and the operator
	// looking for a box that does not exist.
	if !strings.Contains(err.Error(), "not on this page") {
		t.Errorf("a missing section was reported as %q", err)
	}

	notASection, _ := json.Marshal(Layout{Modules: []Module{
		{ID: "words", Kind: Text, X: 1, Y: 1, W: 12, H: 2, Body: "hello"},
		{ID: "a", Kind: Boards, X: 1, Y: 1, W: 2, H: 1, Parent: "words"},
	}})
	if _, err := Parse(string(notASection)); err == nil {
		t.Error("a module was allowed inside something that is not a section")
	}
}

// TestAModuleMayBeOneColumnWide.
//
// The dashboard's own items go down to a single cell, and a module here can be
// one board, one organisation or one button — all of which are cards, and a
// card is what a single column holds. How small is useful is the operator's
// judgement; this only says the grid will draw it.
func TestAModuleMayBeOneColumnWide(t *testing.T) {
	out, _ := json.Marshal(Layout{Modules: []Module{
		{ID: "one", Kind: Boards, X: 1, Y: 1, W: 1, H: 1, Scope: "acme/atlas"},
		{ID: "two", Kind: Boards, X: 2, Y: 1, W: 1, H: 1, Scope: "acme/web"},
	}})
	if _, err := Parse(string(out)); err != nil {
		t.Errorf("two single-column modules side by side were refused: %v", err)
	}
}

// TestAPointIsASentenceAndNotAParagraph.
//
// Its own bound: longer than a button's label because it is a sentence, shorter
// than a paragraph because it sits beside a mark on one line. Counted in runes,
// like every other bound about what fits.
func TestAPointIsASentenceAndNotAParagraph(t *testing.T) {
	page := func(body string) string {
		out, _ := json.Marshal(Layout{Modules: []Module{
			{ID: "p", Kind: Intro, X: 1, Y: 1, W: 4, H: 1, Body: body},
		}})
		return string(out)
	}
	if _, err := Parse(page(strings.Repeat("ぜ", maxPoint))); err != nil {
		t.Errorf("a point of %d characters was refused: %v", maxPoint, err)
	}
	if _, err := Parse(page(strings.Repeat("ぜ", maxPoint+1))); err == nil {
		t.Error("a point past the bound was accepted")
	}
	// And longer than a label, which is the point of having two bounds.
	if _, err := Parse(page(strings.Repeat("a", maxLabel+10))); err != nil {
		t.Errorf("a point was measured as if it were a label: %v", err)
	}
}
