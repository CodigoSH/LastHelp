package landing

import (
	"strings"
	"testing"
)

// TestTheWhitelistIsExactlyThis.
//
// A whitelist nobody asserts loses entries in silence. This compares the
// policy element by element rather than checking that a few favourites are in
// it: admitting something new, or dropping something the editor's toolbar
// writes, has to be a change somebody made on purpose and defended in a diff.
func TestTheWhitelistIsExactlyThis(t *testing.T) {
	wantTags := []string{
		"a", "blockquote", "br", "code", "em", "h1", "h2", "h3", "li", "ol",
		"p", "s", "span", "strong", "u", "ul",
	}
	wantClasses := []string{
		"align-center", "align-end", "align-justify", "align-start",
		"ink-accent", "ink-danger", "ink-dim", "ink-info", "ink-plain",
		"ink-success", "ink-warning",
	}
	wantSchemes := []string{"http", "https", "mailto"}

	same(t, "tags", AllowedTags, wantTags)
	same(t, "classes", AllowedClasses, wantClasses)
	same(t, "schemes", AllowedSchemes, wantSchemes)
}

func same(t *testing.T, what string, got map[string]bool, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s: the policy holds %d entries and the test names %d", what, len(got), len(want))
	}
	for _, name := range want {
		if !got[name] {
			t.Errorf("%s: %q is named by the test and missing from the policy", what, name)
		}
	}
}

// TestCleanRefusesTheWaysScriptReachesAPage.
//
// Each of these is a real technique rather than a variation on one: a tag, an
// attribute, a URL scheme, and a tag that a filter which only reads tag names
// lets through because the danger is in what it contains.
func TestCleanRefusesTheWaysScriptReachesAPage(t *testing.T) {
	for _, c := range []struct {
		name  string
		in    string
		gone  []string
		stays string
	}{
		{
			name:  "a script tag takes its contents with it",
			in:    `<p>Before</p><script>alert(1)</script><p>After</p>`,
			gone:  []string{"script", "alert"},
			stays: "Before",
		},
		{
			name: "an event handler is not an attribute we keep",
			in:   `<p onclick="alert(1)" onmouseover="alert(2)">Words</p>`,
			gone: []string{"onclick", "onmouseover", "alert"},
			// The paragraph survives; only the way in is removed.
			stays: "<p>Words</p>",
		},
		{
			name:  "a javascript href is not a link",
			in:    `<a href="javascript:alert(1)">Click</a>`,
			gone:  []string{"javascript", "href"},
			stays: "Click",
		},
		{
			name:  "nor is a data href",
			in:    `<a href="data:text/html;base64,PHNjcmlwdD4=">Click</a>`,
			gone:  []string{"data:", "href"},
			stays: "Click",
		},
		{
			name:  "a style block cannot repaint the page",
			in:    `<style>body{display:none}</style><p>Words</p>`,
			gone:  []string{"style", "display"},
			stays: "Words",
		},
		{
			name:  "a style attribute is not kept either",
			in:    `<p style="position:fixed;inset:0">Words</p>`,
			gone:  []string{"style", "fixed"},
			stays: "<p>Words</p>",
		},
		{
			name:  "an iframe does not bring another page onto this one",
			in:    `<iframe src="https://example.com"></iframe><p>Words</p>`,
			gone:  []string{"iframe", "example.com"},
			stays: "Words",
		},
		{
			name:  "an svg carries script in browsers and is refused whole",
			in:    `<svg><script>alert(1)</script></svg><p>Words</p>`,
			gone:  []string{"svg", "alert"},
			stays: "Words",
		},
		{
			name:  "a class outside the set is dropped and the tag stays",
			in:    `<span class="ink-accent something-else">Words</span>`,
			gone:  []string{"something-else"},
			stays: `<span class="ink-accent">Words</span>`,
		},
		{
			name:  "an image would show as a broken box under this page's policy",
			in:    `<p>Look <img src="https://example.com/x.png" onerror="alert(1)"></p>`,
			gone:  []string{"img", "onerror", "example.com"},
			stays: "Look",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, err := Clean(c.in)
			if err != nil {
				t.Fatal(err)
			}
			for _, forbidden := range c.gone {
				if strings.Contains(strings.ToLower(out), strings.ToLower(forbidden)) {
					t.Errorf("%q survived cleaning:\n  in:  %s\n  out: %s", forbidden, c.in, out)
				}
			}
			if c.stays != "" && !strings.Contains(out, c.stays) {
				t.Errorf("cleaning lost the words:\n  want to contain: %s\n  out:             %s", c.stays, out)
			}
		})
	}
}

// TestCleanKeepsWhatSomebodyWrote.
//
// The other half, and the one that catches a policy tightened until the module
// is useless: everything the editor's toolbar can produce has to survive a
// round trip unchanged.
func TestCleanKeepsWhatSomebodyWrote(t *testing.T) {
	for _, c := range []struct{ name, in, want string }{
		{"a heading", `<h2>What we do</h2>`, `<h2>What we do</h2>`},
		{"emphasis", `<p><strong>bold</strong> and <em>slanted</em></p>`, `<p><strong>bold</strong> and <em>slanted</em></p>`},
		{"a list", `<ul><li>One</li><li>Two</li></ul>`, `<ul><li>One</li><li>Two</li></ul>`},
		{"a numbered list", `<ol><li>One</li></ol>`, `<ol><li>One</li></ol>`},
		{"a quote", `<blockquote>Said</blockquote>`, `<blockquote>Said</blockquote>`},
		{"a break", `<p>One<br>Two</p>`, `<p>One<br>Two</p>`},
		{
			"an alignment, which cannot be an inline style here",
			`<p class="align-center">Middle</p>`,
			`<p class="align-center">Middle</p>`,
		},
		{
			"a colour, for the same reason",
			`<p><span class="ink-accent">Lit</span></p>`,
			`<p><span class="ink-accent">Lit</span></p>`,
		},
		{
			"a link out, told not to hand this page over",
			`<p><a href="https://codigosh.com">There</a></p>`,
			`<p><a href="https://codigosh.com" rel="noopener noreferrer">There</a></p>`,
		},
		{
			"a link to somewhere in this installation",
			`<p><a href="/orgs/codigosh">Here</a></p>`,
			`<p><a href="/orgs/codigosh" rel="noopener noreferrer">Here</a></p>`,
		},
		{
			"an address",
			`<p><a href="mailto:hi@codigosh.com">Write</a></p>`,
			`<p><a href="mailto:hi@codigosh.com" rel="noopener noreferrer">Write</a></p>`,
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, err := Clean(c.in)
			if err != nil {
				t.Fatal(err)
			}
			if out != c.want {
				t.Errorf("round trip changed it:\n  want: %s\n  got:  %s", c.want, out)
			}
		})
	}
}

// TestCleanSpeaksOneDialect.
//
// A browser's editing commands still emit the presentational tags of 1997, and
// which one arrives depends on whose browser typed it. Folding them here means
// the same sentence is stored one way whoever wrote it — and it means the
// stylesheet has one selector to dress rather than three.
func TestCleanSpeaksOneDialect(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{`<b>bold</b>`, `<strong>bold</strong>`},
		{`<i>slanted</i>`, `<em>slanted</em>`},
		{`<strike>gone</strike>`, `<s>gone</s>`},
		{`<div>a block</div>`, `<p>a block</p>`},
	} {
		out, err := Clean(c.in)
		if err != nil {
			t.Fatal(err)
		}
		if out != c.want {
			t.Errorf("%s should be stored as %s, got %s", c.in, c.want, out)
		}
	}
}

// TestCleanSurvivesMarkupThatDoesNotClose.
//
// The reason this parses and re-serialises instead of filtering a stream of
// tokens: handed something unbalanced, a stream filter emits its own tags and
// lets the page close them somewhere else. Whatever arrives, what comes out
// has to close what it opens.
func TestCleanSurvivesMarkupThatDoesNotClose(t *testing.T) {
	out, err := Clean(`<p>One<p>Two<strong>Three`)
	if err != nil {
		t.Fatal(err)
	}
	if opens, closes := strings.Count(out, "<p>"), strings.Count(out, "</p>"); opens != closes {
		t.Fatalf("%d paragraphs opened and %d closed: %s", opens, closes, out)
	}
	if !strings.Contains(out, "</strong>") {
		t.Fatalf("the emphasis was left open: %s", out)
	}
}
