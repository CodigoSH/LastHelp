package bridge

import (
	"strings"
	"testing"

	"git.codigosh.com/CodigoSH/LastHelp/internal/core/item"
)

// TestWhatAStrangerWroteDoesNothingOnTheForge: each thing a forge's Markdown does with
// text — notify, fetch, hide a destination, link, run a command, pass raw markup — is
// switched off, and code is left exactly as it was written.
func TestWhatAStrangerWroteDoesNothingOnTheForge(t *testing.T) {
	cases := []struct{ name, in, want string }{
		{"a mention", "ping @alice please", `ping \@alice please`},
		{"an email address", "write to bob@example.com", `write to bob\@example.com`},
		{"an image", "![x](http://tracker.example/p.png)", `!\[x](http://tracker.example/p.png)`},
		{"a link that hides where it goes", "[docs](http://bad.example)", `\[docs](http://bad.example)`},
		{"raw markup", `<img src=x onerror=alert(1)>`, `\<img src=x onerror=alert(1)>`},
		{"a reference to another issue", "same as #12 and org/repo#7", `same as \#12 and org/repo\#7`},
		{"a command at the start of a line", "/close\n  /assign @bob", "\\/close\n  \\/assign \\@bob"},
		{"a slash in the middle of a line", "a/b and /usr/bin", "a/b and /usr/bin"},
		{"a path that starts a line is escaped too", "/usr/bin/x", `\/usr/bin/x`},
		{"a backslash, so an escape cannot be forged", `\@alice`, `\\\@alice`},
		{"a fragment of an address", "http://h/#1", "http://h/#1"},
		{"plain emphasis, which does nothing", "**bold** and *it* and - a list", "**bold** and *it* and - a list"},
		{"a heading mark that is not a reference", "# title", "# title"},
		{"a code span, as written", "use `a[0]` and `@x` and `<b>`", "use `a[0]` and `@x` and `<b>`"},
		{"a double-backtick span", "``a ` [1] @x``", "``a ` [1] @x``"},
		{"a span that never closes is just text", "a ` [1] @x", "a ` \\[1] \\@x"},
		{"a fenced block, as written", "```go\nx[0] @y\n```\nafter @z", "```go\nx[0] @y\n```\nafter \\@z"},
		{"a tilde fence", "~~~\n@y\n~~~", "~~~\n@y\n~~~"},
		{"a fence the writer left open is closed", "```\n@y [1]", "```\n@y [1]\n```"},
		{"words from another script survive", "ошибка @имя 日本語", `ошибка \@имя 日本語`},
	}
	for _, c := range cases {
		if got := neutralise(c.in); got != c.want {
			t.Errorf("%s:\n  in   %q\n  got  %q\n  want %q", c.name, c.in, got, c.want)
		}
	}
}

// TestTheIssueCarriesTheNeutralisedTextAndAnswers: the body and every answer pass
// through it — the answers are written by the same stranger — while the line this
// product appends stays as it is.
func TestTheIssueCarriesTheNeutralisedTextAndAnswers(t *testing.T) {
	body := IssueBody(item.Item{
		PublicID: "abc123",
		Body:     "it breaks, @alice knows\n```\nunclosed @x",
		Fields: []item.Field{
			{Label: "Version", Value: "1.4.2"},
			{Label: "Steps", Value: "open [this](http://bad.example)"},
		},
	})
	if !strings.Contains(body, `it breaks, \@alice knows`) {
		t.Errorf("a mention in the text reaches the forge as written:\n%s", body)
	}
	if !strings.Contains(body, "**Steps:** open \\[this](http://bad.example)") {
		t.Errorf("an answer was not neutralised:\n%s", body)
	}
	if !strings.Contains(body, "```\nunclosed @x\n```\n") {
		t.Errorf("an unclosed fence would swallow the footer:\n%s", body)
	}
	if !strings.HasSuffix(body, "---\nReported through LastHelp (abc123)\n") {
		t.Errorf("the link back to the report is gone:\n%s", body)
	}
}
