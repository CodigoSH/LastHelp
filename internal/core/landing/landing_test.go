package landing

import (
	"strings"
	"testing"
)

// page is a valid one-band page holding these modules in one column, for the
// tests that are about a module rather than about bands.
func page(modules ...Module) Page {
	return Page{Bands: []Band{{ID: "b", Columns: One, Cells: [][]Module{modules}}}}
}

func mustRefuse(t *testing.T, why string, p Page) {
	t.Helper()
	if _, err := p.Encode(); err == nil {
		t.Errorf("accepted a page that %s", why)
	}
}

func mustAccept(t *testing.T, why string, p Page) {
	t.Helper()
	if _, err := p.Encode(); err != nil {
		t.Errorf("refused a page that %s: %v", why, err)
	}
}

// TestAnUnsetPageIsTheDefaultPage.
//
// Every installation starts here, and a front page with nothing on it is not a
// choice anybody made. The default is read, never stored, so a better one in a
// later release reaches everybody who never changed theirs.
func TestAnUnsetPageIsTheDefaultPage(t *testing.T) {
	got, err := Parse("")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Bands) != len(Default().Bands) {
		t.Fatalf("an unset page came back with %d bands", len(got.Bands))
	}
	if _, err := Default().Encode(); err != nil {
		t.Fatalf("the default page is not a page this package accepts: %v", err)
	}
}

// TestTheDefaultPageOpensWithTheSearch.
//
// The order is the one a visitor needs: find what exists before writing it
// again, then where to report, then that people here are listened to, and the
// explanation last. The first thing on the page is a heading that searches.
func TestTheDefaultPageOpensWithTheSearch(t *testing.T) {
	d := Default()
	first := d.Bands[0].Cells[0][0]
	if first.Kind != Heading || !first.WithSearch {
		t.Fatalf("the default page opens with %q (search %v)", first.Kind, first.WithSearch)
	}
	var kinds []string
	for _, m := range d.Modules() {
		kinds = append(kinds, string(m.Kind))
	}
	want := "heading boards reports reports text text text"
	if got := strings.Join(kinds, " "); got != want {
		t.Errorf("the default page reads %q, want %q", got, want)
	}
	// Its words are the installation's and the catalogue's, not stored here.
	for _, m := range d.Modules() {
		if m.Body != "" || m.Title != "" || m.Subtitle != "" || m.Name != "" {
			t.Errorf("the default module %q carries words of its own", m.ID)
		}
	}
}

// TestAPageInTheGridFormatComesBackAsTheDefault.
//
// The grid lived only in pre-release builds. A page stored in it is not
// converted and not refused: it is the default page, and the server starts.
func TestAPageInTheGridFormatComesBackAsTheDefault(t *testing.T) {
	old := `{"modules":[{"id":"hero","kind":"hero","x":1,"y":1,"w":12,"h":2}]}`
	got, err := Parse(old)
	if err != nil {
		t.Fatalf("a page in the old format refused to open: %v", err)
	}
	if len(got.Bands) != len(Default().Bands) || got.Bands[0].ID != "top" {
		t.Errorf("a page in the old format did not come back as the default: %+v", got.Bands)
	}
}

// TestAPageSurvivesTheRoundTrip.
func TestAPageSurvivesTheRoundTrip(t *testing.T) {
	in := Page{
		Bands: []Band{
			{ID: "a", Columns: WideNarrow, Name: "News", Background: BackgroundAccent, Width: WidthFull, Spacing: SpacingCompact,
				Cells: [][]Module{
					{{ID: "t", Kind: Text, Body: "<p>Hello</p>", Tone: ToneNotice}},
					{{ID: "l", Kind: Links, Name: "Elsewhere", Links: []Link{
						{Label: "Docs", To: ToURL, URL: "https://docs.example.com"},
						{Label: "Report", To: ToForm, Scope: "acme/atlas", Button: true},
					}}},
				}},
			{ID: "where", Columns: One, HideName: true, Cells: [][]Module{{}}},
		},
		Settings: Settings{Title: "Acme", Description: "Where Acme listens.",
			Footer: []Link{{Label: "Code", To: ToURL, URL: "https://git.example.com/acme"}}},
	}
	encoded, err := in.Encode()
	if err != nil {
		t.Fatal(err)
	}
	out, err := Parse(encoded)
	if err != nil {
		t.Fatal(err)
	}
	again, _ := out.Encode()
	if again != encoded {
		t.Errorf("the page changed on the way through:\n%s\n%s", encoded, again)
	}
}

// TestAModuleNobodyCanDrawIsRefused, and so are the kinds of the grid.
func TestAModuleNobodyCanDrawIsRefused(t *testing.T) {
	for _, kind := range []Kind{"carousel", "hero", "section", "most_voted", "call_to_action", ""} {
		mustRefuse(t, "holds a "+string(kind), page(Module{ID: "m", Kind: kind}))
	}
}

// TestABandIsOneOfTheSixLayoutsWithACellPerColumn.
func TestABandIsOneOfTheSixLayoutsWithACellPerColumn(t *testing.T) {
	for _, c := range []Columns{One, Halves, Thirds, WideNarrow, NarrowWide, Quarters} {
		cells := make([][]Module, c.count())
		mustAccept(t, "is laid out "+string(c), Page{Bands: []Band{{ID: "b", Columns: c, Cells: cells}}})
	}
	mustRefuse(t, "has a seventh layout", Page{Bands: []Band{{ID: "b", Columns: "3-1", Cells: make([][]Module, 2)}}})
	// A module in a column the layout does not have is one nobody would see.
	mustRefuse(t, "has more cells than columns", Page{Bands: []Band{{ID: "b", Columns: Halves, Cells: make([][]Module, 3)}}})
	mustRefuse(t, "has fewer cells than columns", Page{Bands: []Band{{ID: "b", Columns: Thirds, Cells: make([][]Module, 2)}}})
	// An empty column is allowed: it is where the editor offers to add one.
	mustAccept(t, "has an empty column", Page{Bands: []Band{{ID: "b", Columns: Halves,
		Cells: [][]Module{{{ID: "t", Kind: Text}}, {}}}}})
}

// TestABandsChoicesAreShortClosedLists.
func TestABandsChoicesAreShortClosedLists(t *testing.T) {
	band := func(edit func(*Band)) Page {
		b := Band{ID: "b", Columns: One, Cells: make([][]Module, 1)}
		edit(&b)
		return Page{Bands: []Band{b}}
	}
	mustRefuse(t, "has a background of its own invention", band(func(b *Band) { b.Background = "#ff0000" }))
	mustRefuse(t, "has a width of its own invention", band(func(b *Band) { b.Width = "900px" }))
	mustRefuse(t, "has a spacing of its own invention", band(func(b *Band) { b.Spacing = "huge" }))
	mustRefuse(t, "names a band past the bound", band(func(b *Band) { b.Name = strings.Repeat("ぜ", maxLabel+1) }))
	// Counted in characters: a name that fits in Japanese is not refused for
	// its bytes.
	mustAccept(t, "names a band at the bound in Japanese", band(func(b *Band) { b.Name = strings.Repeat("ぜ", maxLabel) }))
}

// TestAPageHasBounds.
func TestAPageHasBounds(t *testing.T) {
	mustRefuse(t, "has no bands", Page{})

	var bands []Band
	for i := 0; i <= maxBands; i++ {
		bands = append(bands, Band{ID: "b" + string(rune('a'+i)), Columns: One, Cells: make([][]Module, 1)})
	}
	mustRefuse(t, "has more bands than the bound", Page{Bands: bands})

	var many []Module
	for i := 0; i <= maxModules; i++ {
		many = append(many, Module{ID: "m" + string(rune('a'+i)), Kind: Search})
	}
	mustRefuse(t, "has more modules than the bound", page(many...))

	mustRefuse(t, "gives a band and a module the same id", Page{Bands: []Band{
		{ID: "x", Columns: One, Cells: [][]Module{{{ID: "x", Kind: Search}}}},
	}})
	mustRefuse(t, "gives two modules in different bands one id", Page{Bands: []Band{
		{ID: "a", Columns: One, Cells: [][]Module{{{ID: "m", Kind: Search}}}},
		{ID: "b", Columns: One, Cells: [][]Module{{{ID: "m", Kind: Text}}}},
	}})
}

// TestEachFieldBelongsToTheKindsThatReadIt.
//
// A field on a kind that ignores it is somebody expecting a page they are not
// going to get, so it is refused rather than dropped.
func TestEachFieldBelongsToTheKindsThatReadIt(t *testing.T) {
	cases := map[string]Module{
		"words on a search":               {ID: "m", Kind: Search, Body: "<p>x</p>"},
		"a tone on a heading":             {ID: "m", Kind: Heading, Tone: ToneNote},
		"a title on a text":               {ID: "m", Kind: Text, Title: "x"},
		"a search on a text":              {ID: "m", Kind: Text, WithSearch: true},
		"rows on a roadmap":               {ID: "m", Kind: Roadmap, Rows: 3},
		"a choice of reports on a boards": {ID: "m", Kind: Boards, Show: ShowLatest},
		"a display on reports":            {ID: "m", Kind: Reports, Display: DisplayList},
		"links on a text":                 {ID: "m", Kind: Text, Links: []Link{{Label: "x", To: ToURL, URL: "https://x.dev"}}},
		"a picture on a text":             {ID: "m", Kind: Text, Image: ImagePrefix + strings.Repeat("a", 32) + ".webp", Alt: "x"},
		"a name on a text":                {ID: "m", Kind: Text, Name: "x"},
		"a name on a heading":             {ID: "m", Kind: Heading, HideName: true},
		"a scope on a text":               {ID: "m", Kind: Text, Scope: "acme"},
	}
	for why, m := range cases {
		mustRefuse(t, "puts "+why, page(m))
	}
}

// TestTheClosedChoicesOfAModule.
func TestTheClosedChoicesOfAModule(t *testing.T) {
	mustRefuse(t, "frames a module with a style of its own", page(Module{ID: "m", Kind: Text, Style: "glass"}))
	mustRefuse(t, "gives a text a tone of its own", page(Module{ID: "m", Kind: Text, Tone: "shout"}))
	mustRefuse(t, "shows a list of its own invention", page(Module{ID: "m", Kind: Reports, Show: "trending"}))
	mustRefuse(t, "draws boards as a grid of its own", page(Module{ID: "m", Kind: Boards, Display: "masonry"}))
	mustRefuse(t, "aligns a heading right", page(Module{ID: "m", Kind: Heading, Align: "right"}))
	mustRefuse(t, "asks for more rows than the bound", page(Module{ID: "m", Kind: Reports, Rows: maxRows + 1}))
	for _, show := range []string{"", ShowMostVoted, ShowRecentlyDone, ShowLatest, ShowPlanned} {
		mustAccept(t, "shows "+show, page(Module{ID: "m", Kind: Reports, Show: show}))
	}
}

// TestAScopeIsAnOrganisationOrABoard.
func TestAScopeIsAnOrganisationOrABoard(t *testing.T) {
	mustAccept(t, "points at an organisation", page(Module{ID: "m", Kind: Boards, Scope: "acme"}))
	mustAccept(t, "points at a board", page(Module{ID: "m", Kind: Reports, Scope: "acme/atlas"}))
	for _, bad := range []string{"Acme", "acme/", "/atlas", "acme/atlas/x", "acme atlas", "../x"} {
		mustRefuse(t, "points at "+bad, page(Module{ID: "m", Kind: Boards, Scope: bad}))
	}
	if !(Module{Scope: "acme/atlas"}).ShowsOne() || (Module{Scope: "acme"}).ShowsOne() {
		t.Error("ShowsOne does not tell one board from a set")
	}
}

// TestALinkGoesSomewhereItCanGo.
//
// Anything that reaches an href on a public page is refused unless it is an
// absolute http or https address or one board of this installation: a
// javascript: or data: link is how a link becomes a script.
func TestALinkGoesSomewhereItCanGo(t *testing.T) {
	link := func(l Link) Page { return page(Module{ID: "m", Kind: Links, Links: []Link{l}}) }

	mustAccept(t, "links to an https address", link(Link{Label: "x", To: ToURL, URL: "https://example.com/a?b=c"}))
	mustAccept(t, "links to an http address", link(Link{Label: "x", To: ToURL, URL: "http://intranet.local"}))
	mustAccept(t, "links to a board's form", link(Link{Label: "x", To: ToForm, Scope: "acme/atlas", Button: true}))
	mustAccept(t, "links to a board's roadmap", link(Link{Label: "x", To: ToRoadmap, Scope: "acme/atlas"}))

	for _, bad := range []string{
		"javascript:alert(1)", "JavaScript:alert(1)", "data:text/html,<script>", "/relative",
		"//example.com", "example.com", "ftp://example.com", "https://", "",
	} {
		mustRefuse(t, "links to "+bad, link(Link{Label: "x", To: ToURL, URL: bad}))
	}
	mustRefuse(t, "links a form to an organisation", link(Link{Label: "x", To: ToForm, Scope: "acme"}))
	mustRefuse(t, "links a board and an address at once", link(Link{Label: "x", To: ToBoard, Scope: "acme/atlas", URL: "https://x.dev"}))
	mustRefuse(t, "links somewhere of its own invention", link(Link{Label: "x", To: "mailto", URL: "mailto:a@b.c"}))
	mustRefuse(t, "has a link with no label", link(Link{Label: " ", To: ToURL, URL: "https://x.dev"}))

	var many []Link
	for i := 0; i <= maxLinks; i++ {
		many = append(many, Link{Label: "x", To: ToURL, URL: "https://x.dev"})
	}
	mustRefuse(t, "has more links than the bound", page(Module{ID: "m", Kind: Links, Links: many}))
	// The footer is held to the same rules.
	mustRefuse(t, "puts a script in the footer", Page{Bands: page().Bands,
		Settings: Settings{Footer: []Link{{Label: "x", To: ToURL, URL: "javascript:alert(1)"}}}})
}

// TestAPictureNeedsItsWords.
//
// An image module may be placed empty — that is where the editor asks for one —
// but a picture without a description is a hole in the page for somebody who
// cannot see it.
func TestAPictureNeedsItsWords(t *testing.T) {
	name := ImagePrefix + strings.Repeat("a1", 16) + ".webp"
	mustAccept(t, "places an empty image", page(Module{ID: "m", Kind: Picture}))
	mustAccept(t, "has a described picture", page(Module{ID: "m", Kind: Picture, Image: name, Alt: "The dashboard"}))
	mustRefuse(t, "has a picture with no description", page(Module{ID: "m", Kind: Picture, Image: name}))
	mustRefuse(t, "names a picture that is not a stored one", page(Module{ID: "m", Kind: Picture, Image: "../../etc/passwd", Alt: "x"}))
	mustRefuse(t, "shares the page with a picture that is not a stored one",
		Page{Bands: page().Bands, Settings: Settings{ShareImage: "https://elsewhere/x.png"}})
}

// TestThePagesOwnWordsAreBounded, in characters.
func TestThePagesOwnWordsAreBounded(t *testing.T) {
	mustRefuse(t, "has a title past the bound", Page{Bands: page().Bands, Settings: Settings{Title: strings.Repeat("ぜ", maxTitle+1)}})
	mustAccept(t, "has a title at the bound in Japanese", Page{Bands: page().Bands, Settings: Settings{Title: strings.Repeat("ぜ", maxTitle)}})
	mustRefuse(t, "has a description past the bound", Page{Bands: page().Bands, Settings: Settings{Description: strings.Repeat("x", maxDesc+1)}})
	mustRefuse(t, "has a heading title past the bound", page(Module{ID: "m", Kind: Heading, Title: strings.Repeat("x", maxTitle+1)}))
}

// TestTheWordsOnThePageAreCleanedComingAndGoing.
//
// Both ways in, because neither what a browser sent nor what an earlier
// version stored is a reason to put unchecked markup on a public page.
func TestTheWordsOnThePageAreCleanedComingAndGoing(t *testing.T) {
	dirty := `{"bands":[{"id":"b","columns":"1","cells":[[{"id":"t","kind":"text","body":"<p>hi</p><script>alert(1)</script>"}]]}]}`

	submitted, err := Submitted(strings.NewReader(dirty))
	if err != nil {
		t.Fatal(err)
	}
	if body := submitted.Modules()[0].Body; strings.Contains(body, "script") {
		t.Errorf("a submitted page kept a script: %q", body)
	}
	parsed, err := Parse(dirty)
	if err != nil {
		t.Fatal(err)
	}
	if body := parsed.Modules()[0].Body; strings.Contains(body, "script") || !strings.Contains(body, "hi") {
		t.Errorf("a stored page kept a script or lost its words: %q", body)
	}
}

// TestAPageOfWordsNeedsNothingFromTheDatabase.
//
// A page of headings, words, links and pictures shown to a stranger reads
// neither the board table nor the item table.
func TestAPageOfWordsNeedsNothingFromTheDatabase(t *testing.T) {
	words := page(
		Module{ID: "h", Kind: Heading, WithSearch: true},
		Module{ID: "t", Kind: Text},
		Module{ID: "l", Kind: Links},
		Module{ID: "i", Kind: Picture},
		Module{ID: "s", Kind: Search},
	)
	if words.NeedsData() {
		t.Error("a page of words says it needs the database")
	}
	for _, k := range []Kind{Boards, Reports, Roadmap, Statistics} {
		if !page(Module{ID: "m", Kind: k}).NeedsData() {
			t.Errorf("a page with %s says it needs nothing", k)
		}
	}
	if !Default().NeedsData() {
		t.Error("the default page shows boards and reports and says it needs nothing")
	}
}

// TestEveryPictureAPageUsesIsFound, which is what keeps the sweep from
// deleting one that is still on the screen.
func TestEveryPictureAPageUsesIsFound(t *testing.T) {
	inWords := strings.Repeat("1", 32)
	asModule := strings.Repeat("2", 32)
	shared := strings.Repeat("3", 32)
	p := Page{
		Bands: []Band{{ID: "b", Columns: Halves, Cells: [][]Module{
			{{ID: "t", Kind: Text, Body: `<p><img src="/api/landing/images/` + inWords + `" alt=""></p>`}},
			{{ID: "i", Kind: Picture, Image: ImagePrefix + asModule + ".webp", Alt: "x"}},
		}}},
		Settings: Settings{ShareImage: ImagePrefix + shared + ".webp"},
	}
	found := strings.Join(p.imageIDs(), " ")
	for _, id := range []string{inWords, asModule, shared} {
		if !strings.Contains(found, id) {
			t.Errorf("the picture %s is on the page and was not found: %s", id[:4], found)
		}
	}
}
