package arch

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// A version written in two places drifts, and nothing notices until the day it
// matters: the artefact that was tested and the artefact that was published
// stop being built by the same toolchain, and no test can see it because both
// of them pass.
//
// It had already happened here. The three workflow steps said `bun-version:
// latest` while the Dockerfile said `oven/bun:1-alpine`, so a release was
// built with whichever Bun existed that morning.
//
// What this refuses: a version of Go, Bun or staticcheck that disagrees with
// the others, and a pin that floats — `latest`, a bare major, or a series
// without its patch.

type pin struct {
	file    string
	what    string
	pattern *regexp.Regexp
}

var pins = []pin{
	{"go.mod", "go", regexp.MustCompile(`(?m)^go (\S+)`)},
	{"Dockerfile", "go", regexp.MustCompile(`FROM golang:(\S+?)-alpine`)},
	{".forgejo/workflows/ci.yml", "go", regexp.MustCompile(`go-version: '?"?([^'"\s]+)`)},
	{".forgejo/workflows/release.yml", "go", regexp.MustCompile(`go-version: '?"?([^'"\s]+)`)},

	{"Dockerfile", "bun", regexp.MustCompile(`FROM oven/bun:(\S+?)-alpine`)},
	{".forgejo/workflows/ci.yml", "bun", regexp.MustCompile(`bun-version: '?"?([^'"\s]+)`)},
	{".forgejo/workflows/release.yml", "bun", regexp.MustCompile(`bun-version: '?"?([^'"\s]+)`)},

	{"scripts/release.sh", "staticcheck", regexp.MustCompile(`STATICCHECK_VERSION="(\S+?)"`)},
}

// exactVersion is what a pin has to look like: three numbers, optionally with
// a leading v. `1.26`, `latest` and `1` are all refused — they name a moving
// target, which is the whole point.
var exactVersion = regexp.MustCompile(`^v?\d+\.\d+\.\d+$`)

func TestEveryToolVersionIsPinnedAndAgrees(t *testing.T) {
	root := repoRoot(t)

	found := map[string]map[string][]string{}
	for _, p := range pins {
		body, err := os.ReadFile(filepath.Join(root, p.file))
		if err != nil {
			t.Fatalf("%s: %v — this test is reading the wrong tree", p.file, err)
		}
		matches := p.pattern.FindAllStringSubmatch(string(body), -1)
		if len(matches) == 0 {
			t.Fatalf("no %s version found in %s. Either the pin was removed or "+
				"this test is looking for a spelling that no longer exists — "+
				"a pin nobody checks is the thing this exists to prevent.", p.what, p.file)
		}
		for _, m := range matches {
			version := m[1]
			if !exactVersion.MatchString(version) {
				t.Errorf("%s pins %s at %q, which moves. Name the exact version.",
					p.file, p.what, version)
			}
			if found[p.what] == nil {
				found[p.what] = map[string][]string{}
			}
			found[p.what][version] = append(found[p.what][version], p.file)
		}
	}

	if len(found) != 3 {
		t.Fatalf("read %d tools, want 3 (go, bun, staticcheck)", len(found))
	}

	for what, versions := range found {
		if len(versions) == 1 {
			continue
		}
		var said []string
		for v, files := range versions {
			said = append(said, v+" in "+strings.Join(files, ", "))
		}
		t.Errorf("%s is pinned at %d different versions: %s",
			what, len(versions), strings.Join(said, "; "))
	}
}
