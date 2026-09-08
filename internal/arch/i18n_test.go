package arch

import (
	"encoding/json"
	"errors"
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
	pending := untranslated(t)
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
			if strings.TrimSpace(table[key]) == "" && !pending[key] {
				missing = append(missing, key)
			}
		}
		if len(missing) > 0 {
			t.Errorf("%s is missing %d key(s): %v\n\n"+
				"Translate them, or — while the screen they belong to is still\n"+
				"changing — list them in %s and translate them before release.",
				code, len(missing), sorted(missing), untranslatedFile)
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
			// A catalogue that has not translated this yet is the other rule's
			// business, and it holds the list of what is still waiting. This
			// one is about translations that dropped a value on the way.
			if strings.TrimSpace(table[key]) == "" {
				continue
			}
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

// untranslatedFile lists the keys whose screens are still being designed.
//
// Translating a sentence that is about to be rewritten is work done twice, so
// while a screen is in flux its keys wait here. This is not a way to skip the
// rule: the list is in the repository, it shows up in every diff, and the test
// below refuses to let it hold a key that is already translated everywhere —
// which is what stops it quietly becoming permanent.
const untranslatedFile = "web/public/locales/UNTRANSLATED.txt"

func untranslated(t *testing.T) map[string]bool {
	t.Helper()

	raw, err := os.ReadFile(filepath.Join(repoRoot(t), untranslatedFile))
	if errors.Is(err, os.ErrNotExist) {
		return map[string]bool{}
	}
	if err != nil {
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

// TestTheUntranslatedListIsHonest.
//
// A list of exceptions that nobody prunes becomes the rule. Every key waiting
// here has to still be waiting somewhere, and has to still exist: a key that is
// finished, or one that was deleted, has no business keeping its exemption.
func TestTheUntranslatedListIsHonest(t *testing.T) {
	pending := untranslated(t)
	if len(pending) == 0 {
		return
	}

	all := catalogues(t)
	master := all["en"]

	for key := range pending {
		if _, known := master[key]; !known {
			t.Errorf("\n%s lists %q, which no longer exists in en.json.\n\n"+
				"Remove it: an exemption for a key nobody has is how this list\n"+
				"stops meaning anything.", untranslatedFile, key)
			continue
		}

		stillMissing := false
		for code, table := range all {
			if code != "en" && strings.TrimSpace(table[key]) == "" {
				stillMissing = true
				break
			}
		}
		if !stillMissing {
			t.Errorf("\n%s lists %q, which every catalogue already has.\n\n"+
				"Remove it. The list is for work still to do.", untranslatedFile, key)
		}
	}
}

// TestEveryErrorTheServerSendsHasWords.
//
// The server answers a refusal with a key — "error.already_set_up" — and the
// page looks it up. When the key is not in the catalogue there is nothing to
// look up, so the raw key is what somebody reads on the screen.
//
// Nothing caught this. i18n:scan reads the frontend and finds keys used there
// that do not exist; the keys the server sends never appear in the frontend at
// all, so seventeen of them accumulated with no text behind them, and the first
// anyone knew was "error.already_set_up" printed in an interface.
//
// The two sides are matched here, where both can be read at once.
func TestEveryErrorTheServerSendsHasWords(t *testing.T) {
	master, ok := catalogues(t)["en"]
	if !ok {
		t.Fatal("there is no en.json")
	}

	root := repoRoot(t)
	// The literal as it is written in the source. Every one of these reaches a
	// browser: writeError sends it, and http.Error sends it for the middleware
	// that runs before writeError exists.
	pattern := regexp.MustCompile(`"(error\.[a-z0-9_]+)"`)

	found := map[string][]string{}
	err := filepath.Walk(root, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return relErr
		}
		rel = filepath.ToSlash(rel)
		if info.IsDir() {
			if rel != "." && (rel == ".git" || rel == "node_modules" || rel == "web" || rel == "bin" || rel == "docs") {
				return filepath.SkipDir
			}
			return nil
		}
		// Tests say what an endpoint answered, which is not the same as an
		// endpoint that can answer it.
		if !strings.HasSuffix(rel, ".go") || strings.HasSuffix(rel, "_test.go") {
			return nil
		}
		body, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		for _, match := range pattern.FindAllStringSubmatch(string(body), -1) {
			found[match[1]] = append(found[match[1]], rel)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	// The guard the other rules here carry: a walk that stops matching finds
	// nothing and reports nothing, which reads exactly like a clean result.
	if len(found) < 10 {
		t.Fatalf("only %d error keys found in the server: this rule is not reading the code", len(found))
	}

	var missing []string
	for key, where := range found {
		if strings.TrimSpace(master[key]) == "" {
			missing = append(missing, key+" ("+where[0]+")")
		}
	}
	if len(missing) > 0 {
		t.Errorf("%d error(s) the server can send have no text in en.json:\n  %s\n\n"+
			"Somebody reading the screen sees the key. Add the words, or stop sending the key.",
			len(missing), strings.Join(sorted(missing), "\n  "))
	}
}
