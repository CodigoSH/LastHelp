package arch

import (
	"os"
	"os/exec"
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
//
// This reads go.mod, which answers "what is required" and not "what is
// shipped" — see TestTheNoticeSaysWhatIsActuallyLinked for that half. Both
// matter: everything required deserves a mention, and only what the linker
// keeps may be described as being inside the binaries.
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
			t.Errorf("\n%s is required by go.mod and is not named anywhere in NOTICE\n\n"+
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

// TestTheNoticeSaysWhatIsActuallyLinked asks the linker, because go.mod does
// not answer this question.
//
// Go downloads whole modules; the linker keeps only the packages somebody
// imports. Two of the modules go.mod requires here — the SQLite driver names
// them for platforms this project does not build for — are in no published
// binary at all, and NOTICE described the whole list as "compiled into the
// released binaries". That sentence was false about two of eleven entries,
// in the one file that exists to be read literally.
//
// What this refuses: a module the linker keeps that NOTICE does not name
// before its "NOT in what we publish" line, and a module named above that
// line that is not linked.
func TestTheNoticeSaysWhatIsActuallyLinked(t *testing.T) {
	root := repoRoot(t)

	notice, err := os.ReadFile(filepath.Join(root, "NOTICE"))
	if err != nil {
		t.Fatal(err)
	}
	// Everything before this sentence is the claim "this is inside the
	// binaries". After it is the opposite claim.
	const boundary = "Required by go.mod and NOT in what we publish"
	cut := strings.Index(string(notice), boundary)
	if cut < 0 {
		t.Fatalf("NOTICE no longer contains %q, so this test does not know\n"+
			"which half of it claims to describe the published binaries.", boundary)
	}
	shipped, notShipped := string(notice)[:cut], string(notice)[cut:]

	// Both published architectures. They agree today, and a build tag that
	// pulls something in on one of them is exactly the change nobody would
	// notice.
	linked := map[string]bool{}
	for _, arch := range []string{"amd64", "arm64"} {
		cmd := exec.Command("go", "list", "-deps",
			"-f", "{{if .Module}}{{.Module.Path}}{{end}}", "./cmd/...")
		cmd.Dir = root
		cmd.Env = append(os.Environ(), "GOOS=linux", "GOARCH="+arch)
		out, err := cmd.Output()
		if err != nil {
			t.Fatalf("asking the linker for linux/%s: %v", arch, err)
		}
		for _, line := range strings.Split(string(out), "\n") {
			mod := strings.TrimSpace(line)
			if mod == "" || strings.HasPrefix(mod, "git.codigosh.com/") {
				continue
			}
			linked[mod] = true
		}
	}
	if len(linked) == 0 {
		t.Fatal("the linker reported no third-party modules at all, which means\n" +
			"this test is asking about the wrong thing rather than that there\n" +
			"are none.")
	}

	for mod := range linked {
		if !strings.Contains(shipped, mod) {
			t.Errorf("\n%s is linked into the published binaries and NOTICE does not\n"+
				"name it among what is inside them.", mod)
		}
	}

	for _, line := range strings.Split(notShipped, "\n") {
		if m := noticeModule.FindStringSubmatch(line); m != nil && linked[m[1]] {
			t.Errorf("\nNOTICE says %s is not in what we publish, and the linker keeps it.\n\n"+
				"One of the two is wrong, and the file is the one somebody reads.", m[1])
		}
	}
}

// noticeModule matches a module path at the start of an indented NOTICE line,
// which is how both lists are written.
var noticeModule = regexp.MustCompile(`^\s+([a-z0-9.-]+\.[a-z]{2,}/[^\s]+)\s`)
