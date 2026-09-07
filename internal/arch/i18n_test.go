package arch

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// localesDir is where the catalogues live, and where the build copies them from.
const localesDir = "web/public/locales"

func catalogues(t *testing.T) map[string]map[string]string {
	t.Helper()
	root := repoRoot(t)

	entries, err := os.ReadDir(filepath.Join(root, localesDir))
	if err != nil {
		t.Fatalf("reading %s: %v", localesDir, err)
	}

	out := map[string]map[string]string{}
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(root, localesDir, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		var table map[string]string
		if err := json.Unmarshal(raw, &table); err != nil {
			t.Fatalf("%s is not valid JSON: %v", e.Name(), err)
		}
		out[strings.TrimSuffix(e.Name(), ".json")] = table
	}
	return out
}

// TestTheTwoListsOfLanguagesAgree.
//
// There are two, and there have to be: the server stamps lang and dir into the
// first paint, before any script has run, and the frontend re-stamps them when
// somebody changes language without reloading. Two hand-written lists drift the
// first time one of them gains a language — and the symptom is an Arabic page
// rendering left to right for the first frame, which nobody writing the second
// list would ever see.
func TestTheTwoListsOfLanguagesAgree(t *testing.T) {
	root := repoRoot(t)

	goSource, err := os.ReadFile(filepath.Join(root, "internal/web/language.go"))
	if err != nil {
		t.Fatal(err)
	}
	tsSource, err := os.ReadFile(filepath.Join(root, "web/src/locales/metadata.ts"))
	if err != nil {
		t.Fatal(err)
	}

	goRTL := sorted(extractQuoted(t, string(goSource), `rtlLocales = \[\]string\{([^}]*)\}`))
	tsRTL := sorted(extractQuoted(t, string(tsSource), `rtlLocales = \[([^\]]*)\]`))

	if strings.Join(goRTL, ",") != strings.Join(tsRTL, ",") {
		t.Errorf("\nthe two lists of right-to-left languages disagree\n"+
			"  internal/web/language.go: %v\n"+
			"  web/src/locales/metadata.ts: %v\n\n"+
			"The server stamps dir into the first paint and the frontend re-stamps it on a\n"+
			"language change. When they disagree, a page renders one way and then flips.",
			goRTL, tsRTL)
	}

	goAll := sorted(extractQuoted(t, string(goSource), `locales = \[\]string\{([\s\S]*?)\}`))
	var tsAll []string
	for code := range catalogues(t) {
		tsAll = append(tsAll, code)
	}
	tsAll = sorted(tsAll)

	if strings.Join(goAll, ",") != strings.Join(tsAll, ",") {
		t.Errorf("\nthe server's language list does not match the catalogues on disk\n"+
			"  internal/web/language.go: %v\n"+
			"  %s: %v\n\n"+
			"The server negotiates the first paint out of its list. A language it does not\n"+
			"know is a language nobody is ever served, catalogue or no catalogue.",
			goAll, localesDir, tsAll)
	}
}

// TestEveryCatalogueHasEveryKey.
//
// A missing key falls back to English, which is exactly the thing this house
// refuses: an English sentence inside a catalogue that is not English looks
// finished to every automatic check and looks neglected to the one person who
// reads that language.
func TestEveryCatalogueHasEveryKey(t *testing.T) {
	all := catalogues(t)
	master, ok := all["en"]
	if !ok {
		t.Fatal("there is no en.json; English is the table every other one falls back to")
	}

	for code, table := range all {
		if code == "en" {
			continue
		}
		var missing []string
		for key := range master {
			if strings.TrimSpace(table[key]) == "" {
				missing = append(missing, key)
			}
		}
		if len(missing) > 0 {
			t.Errorf("%s is missing %d key(s): %v", code, len(missing), sorted(missing))
		}

		var extra []string
		for key := range table {
			if _, known := master[key]; !known {
				extra = append(extra, key)
			}
		}
		if len(extra) > 0 {
			t.Errorf("%s has %d key(s) en.json does not: %v", code, len(extra), sorted(extra))
		}
	}
}

// TestPlaceholdersSurviveTranslation.
//
// A translation that dropped {count} renders a sentence with a hole where the
// number should be, and nothing fails: the string is present, non-empty and in
// the right language.
func TestPlaceholdersSurviveTranslation(t *testing.T) {
	all := catalogues(t)
	master := all["en"]
	placeholder := regexp.MustCompile(`\{([a-zA-Z0-9_]+)\}`)

	for key, english := range master {
		want := sorted(placeholder.FindAllString(english, -1))
		if len(want) == 0 {
			continue
		}
		for code, table := range all {
			got := sorted(placeholder.FindAllString(table[key], -1))
			if strings.Join(got, ",") != strings.Join(want, ",") {
				t.Errorf("%s / %s has placeholders %v, English has %v", code, key, got, want)
			}
		}
	}
}

// TestCountersAreWrittenAsLabelColonNumber.
//
// With twenty languages — Russian and Polish have three plural forms, Arabic
// six — pluralising properly is a project of its own. The house rule is to
// write around it: "Votes: {count}" reads correctly for 1 and for 24 in all
// twenty, where "{count} votes" gives "1 votes".
//
// This catches the shape that needs plurals: a count at the start of the
// English string.
func TestCountersAreWrittenAsLabelColonNumber(t *testing.T) {
	master := catalogues(t)["en"]
	leading := regexp.MustCompile(`^\{[a-zA-Z0-9_]+\}\s+\w`)

	for key, english := range master {
		if leading.MatchString(english) {
			t.Errorf("\n%s is written as %q\n\n"+
				"A string that starts with a count needs plural forms in twenty languages,\n"+
				"and gives \"1 votes\" without them. Write it as \"Label: {count}\" instead,\n"+
				"which is correct for every number in every one of them.", key, english)
		}
	}
}

func extractQuoted(t *testing.T, source, pattern string) []string {
	t.Helper()
	block := regexp.MustCompile(pattern).FindStringSubmatch(source)
	if block == nil {
		t.Fatalf("could not find /%s/ — the file changed shape and this rule stopped checking anything", pattern)
	}
	var out []string
	for _, m := range regexp.MustCompile(`"([a-z]{2})"`).FindAllStringSubmatch(block[1], -1) {
		out = append(out, m[1])
	}
	if len(out) == 0 {
		t.Fatalf("/%s/ matched but contained no language codes", pattern)
	}
	return out
}

func sorted(in []string) []string {
	out := append([]string(nil), in...)
	sort.Strings(out)
	return out
}
