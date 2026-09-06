package arch

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var requireLine = regexp.MustCompile(`^\s*([a-z0-9./-]+\.[a-z]{2,}/[^\s]+)\s+v[^\s]+`)

// TestEveryRedistributedModuleIsInTheNotice.
//
// NOTICE is what a company's legal review reads before allowing this in the
// door, and it is the one file nobody remembers to update. A dependency added
// on a Tuesday ships inside every binary from then on, attribution or not.
//
// It also guards the other direction: while there are no dependencies, NOTICE
// says so in words, and this fails if that sentence survives the first one.
func TestEveryRedistributedModuleIsInTheNotice(t *testing.T) {
	root := repoRoot(t)

	goMod, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	notice, err := os.ReadFile(filepath.Join(root, "NOTICE"))
	if err != nil {
		t.Fatal(err)
	}

	var modules []string
	inBlock := false
	for _, line := range strings.Split(string(goMod), "\n") {
		trimmed := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(trimmed, "require ("):
			inBlock = true
			continue
		case inBlock && trimmed == ")":
			inBlock = false
			continue
		}

		candidate := trimmed
		if strings.HasPrefix(trimmed, "require ") {
			candidate = strings.TrimPrefix(trimmed, "require ")
		} else if !inBlock {
			continue
		}
		if m := requireLine.FindStringSubmatch(candidate); m != nil {
			modules = append(modules, m[1])
		}
	}

	for _, mod := range modules {
		if !strings.Contains(string(notice), mod) {
			t.Errorf("\n%s is compiled into the released binaries and is not in NOTICE\n\n"+
				"NOTICE is what somebody reads before allowing this inside their\n"+
				"organisation, and it is the file nobody remembers. A dependency\n"+
				"added today ships in every binary from now on, attributed or not.", mod)
		}
	}

	// The placeholder has to stop being true the moment it stops being true.
	if len(modules) > 0 && strings.Contains(string(notice), "None yet") {
		t.Errorf("\nNOTICE still says \"None yet\" but go.mod requires %d module(s)\n\n"+
			"The sentence was accurate when it was written. A document that was\n"+
			"true once and is not corrected is how a NOTICE starts lying.", len(modules))
	}
}
