package arch

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestEveryCIJobPinsItsImage.
//
// `runs-on: ubuntu-latest` is not Ubuntu and it is not latest. It is a label
// the runner translates, and act_runner writes that mapping into its config the
// day it is registered and never updates it. Ours lands on a Node image built
// on Debian bullseye — 2021.
//
// This is not a theory. On 2026-09-17 a sibling project published a release
// with its binaries, its checksums and its signature, and no container image:
// the one job in that repository without a `container:` died on its first step
// with a 404 fetching ca-certificates, because bullseye is out of support and
// its package index still promises files the mirror stopped serving. Every
// other job there was pinned. That one was written months later by somebody who
// did not know to pin it, and nothing said anything until a release was short
// an artefact.
//
// So the rule is not "the jobs are pinned today", it is "a job that is not
// pinned cannot be added". What this refuses: a job with no `container:`, and a
// `container:` that names a moving tag.
func TestEveryCIJobPinsItsImage(t *testing.T) {
	root := repoRoot(t)

	dir := filepath.Join(root, ".forgejo", "workflows")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("%v — this test is reading the wrong tree", err)
	}

	// A job header, then the keys indented under it. Parsed by hand rather than
	// with a YAML library: this package has no dependencies and the shape being
	// looked for is two lines of it.
	jobLine := regexp.MustCompile(`^  ([a-zA-Z0-9_-]+):\s*$`)
	containerLine := regexp.MustCompile(`^    container:\s*(\S+)?`)
	imageLine := regexp.MustCompile(`^      image:\s*(\S+)`)

	// A tag that names an exact build. `node:22-bookworm` counts: the series is
	// the decision and Debian's own patches inside it are not ours to pin.
	// What does not count is a bare `latest`, or no tag at all.
	moving := regexp.MustCompile(`(^|:)latest$|^[^:]+$`)

	checked := 0
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".yml") {
			continue
		}
		body, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		lines := strings.Split(string(body), "\n")

		inJobs := false
		for i, line := range lines {
			if line == "jobs:" {
				inJobs = true
				continue
			}
			if !inJobs {
				continue
			}
			m := jobLine.FindStringSubmatch(line)
			if m == nil {
				continue
			}
			job := m[1]

			// Everything indented under this job, up to the next one.
			image := ""
			for _, next := range lines[i+1:] {
				if jobLine.MatchString(next) || next == "jobs:" {
					break
				}
				if c := containerLine.FindStringSubmatch(next); c != nil {
					if c[1] != "" {
						image = c[1]
					}
					continue
				}
				if img := imageLine.FindStringSubmatch(next); img != nil && image == "" {
					image = img[1]
				}
			}

			checked++
			switch {
			case image == "":
				t.Errorf("\n%s: job %q has no container:, so it runs on whatever the\n"+
					"runner translates its label to — which is a line in a config file on\n"+
					"a server that nothing versions, and here that is Debian 11.", entry.Name(), job)
			case moving.MatchString(image):
				t.Errorf("\n%s: job %q runs on %q, which moves. Name the image and its tag.",
					entry.Name(), job, image)
			}
		}
	}

	if checked == 0 {
		t.Fatal("no jobs were found in .forgejo/workflows, which means this test is\n" +
			"looking at the wrong thing rather than that everything is pinned.")
	}
}
