package arch

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// TestNoRuleIsDeclaredTwiceOverTheSameProperties.
//
// The stylesheet is one file of a few thousand lines, and that is the right
// shape for it — but a file that long hides a second declaration of the same
// selector as well as separate files would, and the second one wins silently.
//
// Four of these were live at once when this was written. `.theme-choice` was
// declared again seventeen hundred lines further down and turned the new tiles
// back into pills. `.dash-card > h3` set a margin that a later copy overrode,
// so the first was dead code nobody knew about. `.wizard-mark` was written a
// second time by somebody who had not seen the first. And the dark palette
// existed twice, which made an edit appear to do nothing at all.
//
// None of that is a CSS error. The page renders, the tests pass, and it is
// found by noticing that the screen disagrees with the file.
//
// Declaring a selector twice is fine when the two do different jobs — a base
// rule and its narrow-layout override inside a media query, or `:root` holding
// the light palette in one place and the dark values in another. What is not
// fine is two declarations of the same selector, in the same context, both
// setting the same property. That is an override written by accident.
func TestNoRuleIsDeclaredTwiceOverTheSameProperties(t *testing.T) {
	path := filepath.Join(repoRoot(t), "web/styles/theme.css")
	source, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	rules := parseRules(string(source))
	if len(rules) < 100 {
		t.Fatalf("only %d rules parsed out of %s: this test is reading it wrong,"+
			" and a parser that finds nothing passes everything", len(rules), path)
	}

	seen := map[string][]cssRule{}
	for _, r := range rules {
		key := r.context + "||" + r.selector
		seen[key] = append(seen[key], r)
	}

	keys := make([]string, 0, len(seen))
	for k := range seen {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	known := knownDuplicates(t)
	found := map[string]bool{}

	for _, key := range keys {
		group := seen[key]
		if len(group) < 2 {
			continue
		}
		for i := 0; i < len(group); i++ {
			for j := i + 1; j < len(group); j++ {
				clash := overlap(group[i].properties, group[j].properties)
				if len(clash) == 0 {
					continue
				}
				found[group[i].selector] = true
				if known[group[i].selector] {
					continue
				}
				t.Errorf("\n%s is declared twice and both set %s\n"+
					"  line %d and line %d%s\n\n"+
					"The second one wins and the first is dead. Merge them, or make the\n"+
					"later one a narrower selector so that what happens is written down.",
					group[i].selector, strings.Join(clash, ", "),
					group[i].line, group[j].line, contextNote(group[i].context))
			}
		}
	}

	// The list has to stay honest, the way the untranslated one does: an
	// exemption for something already fixed is how a list of exceptions becomes
	// the rule.
	for selector := range known {
		if !found[selector] {
			t.Errorf("%s is listed in %s and is no longer declared twice.\n"+
				"Remove the line: an exemption nobody needs is one nobody prunes.",
				selector, duplicatesFile)
		}
	}
}

// duplicatesFile lists what was already declared twice when this rule was
// written. Nothing may be added to it: it exists so that a stylesheet with a
// known backlog can still be guarded against the next one, and it only ever
// gets shorter.
const duplicatesFile = "web/styles/KNOWN-DUPLICATES.txt"

func knownDuplicates(t *testing.T) map[string]bool {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(repoRoot(t), duplicatesFile))
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]bool{}
		}
		t.Fatal(err)
	}
	out := map[string]bool{}
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		out[line] = true
	}
	return out
}

type cssRule struct {
	selector   string
	context    string
	line       int
	properties []string
}

func contextNote(context string) string {
	if context == "" {
		return ""
	}
	return " (inside " + context + ")"
}

// overlap returns the property names both rules set.
func overlap(a, b []string) []string {
	in := map[string]bool{}
	for _, p := range a {
		in[p] = true
	}
	var both []string
	for _, p := range b {
		if in[p] {
			both = append(both, p)
		}
	}
	sort.Strings(both)
	return both
}

// parseRules walks the stylesheet, tracking which at-rule each rule sits in so
// that a base rule and its media-query override are not mistaken for a pair.
func parseRules(css string) []cssRule {
	var out []cssRule
	var context []string
	var pending strings.Builder

	line := 1
	depth := 0
	for i := 0; i < len(css); i++ {
		c := css[i]
		if c == '\n' {
			line++
		}

		// Comments are prose and may contain braces.
		if c == '/' && i+1 < len(css) && css[i+1] == '*' {
			end := strings.Index(css[i:], "*/")
			if end < 0 {
				break
			}
			line += strings.Count(css[i:i+end], "\n")
			i += end + 1
			pending.Reset()
			continue
		}

		switch c {
		case '{':
			head := strings.TrimSpace(pending.String())
			pending.Reset()
			if strings.HasPrefix(head, "@") {
				context = append(context, head)
				depth++
				continue
			}
			body, consumed := readBody(css[i+1:])
			out = append(out, cssRule{
				selector:   normalise(head),
				context:    strings.Join(context, " "),
				line:       line,
				properties: propertiesOf(body),
			})
			line += strings.Count(css[i:i+1+consumed], "\n")
			i += consumed
		case '}':
			if depth > 0 && len(context) > 0 {
				context = context[:len(context)-1]
				depth--
			}
			pending.Reset()
		default:
			pending.WriteByte(c)
		}
	}
	return out
}

// readBody returns a rule's declarations and how far to skip.
func readBody(css string) (string, int) {
	for i := 0; i < len(css); i++ {
		if css[i] == '}' {
			return css[:i], i + 1
		}
		// A nested block means this was an at-rule after all; stop safely.
		if css[i] == '{' {
			return css[:i], i
		}
	}
	return css, len(css)
}

func propertiesOf(body string) []string {
	var props []string
	for _, part := range strings.Split(body, ";") {
		name, _, found := strings.Cut(part, ":")
		if !found {
			continue
		}
		name = strings.TrimSpace(name)
		if name == "" || strings.Contains(name, "{") {
			continue
		}
		props = append(props, name)
	}
	return props
}

// normalise makes "a,\n b" and "a, b" the same selector.
func normalise(selector string) string {
	parts := strings.Split(selector, ",")
	for i, p := range parts {
		parts[i] = strings.Join(strings.Fields(p), " ")
	}
	return strings.Join(parts, ", ")
}

var _ = fmt.Sprintf
