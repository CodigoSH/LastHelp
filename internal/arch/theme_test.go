package arch

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// TestBothWaysIntoDarkCarryTheSamePalette.
//
// Dark is switched on from two places: the system preference
// (`prefers-color-scheme`) and an explicit choice (`data-theme="dark"`). For a
// while each one carried its own copy of the values, and the copies drifted —
// a change went into one of them and the interface did not move, because the
// screenshots emulate the system preference and were reading the other.
//
// Nothing complains about that. The page renders, both themes look plausible,
// and the difference only shows to whoever happens to have set the opposite
// one. So the values live once in --dark-*, and this checks that both
// selectors point at exactly the same set: same names, no extras, no omissions.
func TestBothWaysIntoDarkCarryTheSamePalette(t *testing.T) {
	source, err := os.ReadFile(filepath.Join(repoRoot(t), "web/styles/theme.css"))
	if err != nil {
		t.Fatal(err)
	}
	css := string(source)

	media := block(t, css, "@media (prefers-color-scheme: dark) {")
	explicit := block(t, css, ":root[data-theme=\"dark\"] {")

	if len(media) == 0 {
		t.Fatal("the prefers-color-scheme block sets no tokens; this test is reading the wrong thing")
	}

	if strings.Join(media, ",") != strings.Join(explicit, ",") {
		t.Errorf("\nthe two ways into dark do not set the same tokens.\n\n"+
			"  prefers-color-scheme: %v\n  data-theme=\"dark\":    %v\n\n"+
			"Whichever one is short is the one somebody will be looking at when a\n"+
			"change appears to do nothing.", media, explicit)
	}

	// And neither side may hold a value of its own: both defer to --dark-*, or
	// there are two copies again and this test is only watching the names.
	//
	// Checked inside each block rather than across the file. The first version
	// searched the whole stylesheet, so a literal put back in one block passed
	// as long as the other still deferred — which is exactly the mutation it
	// exists to catch, and it survived it.
	for _, where := range []string{"@media (prefers-color-scheme: dark) {", ":root[data-theme=\"dark\"] {"} {
		for _, name := range tokensIn(t, css, where) {
			if !strings.Contains(text(t, css, where), "--"+name+": var(--dark-"+name+")") {
				t.Errorf("in %s, --%s is given a literal value instead of var(--dark-%s):\n"+
					"that is the second copy coming back.", where, name, name)
			}
		}
	}
}

// tokensIn is block, named for what it returns.
func tokensIn(t *testing.T, css, opener string) []string { return block(t, css, opener) }

// text returns the source of the first block opened by `opener`.
func text(t *testing.T, css, opener string) string {
	t.Helper()
	at := strings.Index(css, opener)
	if at < 0 {
		t.Fatalf("theme.css no longer contains %q", opener)
	}
	rest := css[at:]
	if end := strings.Index(rest, "\n}\n\n"); end > 0 {
		rest = rest[:end]
	}
	return rest
}

// block returns the token names set inside the first block opened by `opener`.
func block(t *testing.T, css, opener string) []string {
	t.Helper()
	at := strings.Index(css, opener)
	if at < 0 {
		t.Fatalf("theme.css no longer contains %q", opener)
	}
	rest := css[at:]
	// Far enough to cover the block and no further: the next top-level rule.
	if end := strings.Index(rest, "\n}\n\n"); end > 0 {
		rest = rest[:end]
	}
	found := regexp.MustCompile(`--([a-z0-9-]+):`).FindAllStringSubmatch(rest, -1)
	var names []string
	for _, m := range found {
		if strings.HasPrefix(m[1], "dark-") {
			continue
		}
		names = append(names, m[1])
	}
	sort.Strings(names)
	return names
}
