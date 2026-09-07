package arch

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

var envVar = regexp.MustCompile(`"(LASTHELP_[A-Z0-9_]+)"`)

// TestEverySettingIsDocumented.
//
// The failure this prevents has happened in this house: ten environment
// variables the binary read and no document mentioned. Nothing breaks — the
// defaults work — so nobody notices, and the operator who needed one of them
// had no way to find out it existed.
//
// The list is derived from the one package allowed to read the environment, so
// it cannot go stale on the code's side.
func TestEverySettingIsDocumented(t *testing.T) {
	root := repoRoot(t)

	source, err := os.ReadFile(filepath.Join(root, "internal/platform/config/config.go"))
	if err != nil {
		t.Fatal(err)
	}

	seen := map[string]bool{}
	for _, match := range envVar.FindAllStringSubmatch(string(source), -1) {
		seen[match[1]] = true
	}
	if len(seen) == 0 {
		t.Fatal("no settings were found in config.go; this rule is checking nothing")
	}

	example, err := os.ReadFile(filepath.Join(root, ".env.example"))
	if err != nil {
		t.Fatal(err)
	}

	var missing []string
	for name := range seen {
		if !strings.Contains(string(example), name) {
			missing = append(missing, name)
		}
	}
	sort.Strings(missing)

	if len(missing) > 0 {
		t.Errorf("\n.env.example does not mention %d setting(s) the binary reads: %v\n\n"+
			"Nothing breaks when one is missing — the default works — which is exactly why\n"+
			"nobody notices, and why the operator who needed it never found out it existed.",
			len(missing), missing)
	}
}

// TestTheExampleDoesNotInventSettings is the same rule in the other direction.
//
// A documented variable the binary does not read is worse than an undocumented
// one: somebody sets it, nothing happens, and they conclude the feature is
// broken rather than that the document is.
func TestTheExampleDoesNotInventSettings(t *testing.T) {
	root := repoRoot(t)

	source, err := os.ReadFile(filepath.Join(root, "internal/platform/config/config.go"))
	if err != nil {
		t.Fatal(err)
	}
	example, err := os.ReadFile(filepath.Join(root, ".env.example"))
	if err != nil {
		t.Fatal(err)
	}

	declared := regexp.MustCompile(`(?m)^#?\s*(LASTHELP_[A-Z0-9_]+)=`)

	var invented []string
	for _, match := range declared.FindAllStringSubmatch(string(example), -1) {
		if !strings.Contains(string(source), `"`+match[1]+`"`) {
			invented = append(invented, match[1])
		}
	}
	sort.Strings(invented)

	if len(invented) > 0 {
		t.Errorf("\n.env.example documents %d setting(s) the binary never reads: %v\n\n"+
			"Somebody sets one, nothing happens, and they conclude the feature is broken\n"+
			"rather than the document.", len(invented), invented)
	}
}

// TestTheReadmeStatusSectionExists.
//
// The README says what actually works. That section is the first thing somebody
// reads to decide whether to trust this, and it is the first thing to go stale
// — so its absence, at least, fails here.
func TestTheReadmeSaysWhatWorks(t *testing.T) {
	readme, err := os.ReadFile(filepath.Join(repoRoot(t), "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(readme), "## Status") {
		t.Error("the README has no Status section; it is what somebody reads to decide whether " +
			"this does what it claims, and a README describing a server nobody wrote is the " +
			"first thing that makes people stop trusting one")
	}
}
