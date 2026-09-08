package arch

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestTheInstallerIsPOSIXShellAndNotBash.
//
// The installer says `#!/bin/sh` and the documentation pipes it into `sudo sh`.
// On the machines people actually self-host on, `sh` is not bash: it is dash on
// Debian and Ubuntu and BusyBox on Alpine, and neither has arrays, `local`,
// `[[`, `echo -e` or `pipefail`.
//
// A bashism here is not a subtle bug. It is a syntax error on line one, before
// a single check runs, so the installer fails identically on a good release and
// a tampered one — and it does it on three of the most common distributions
// while working perfectly on a developer's Fedora, where `/bin/sh` is bash.
//
// This is written as a grep on purpose. Running the script under a real POSIX
// shell would be the stronger test and it is not available everywhere this
// suite runs: `sh -n` on a machine whose `sh` is bash proves nothing at all,
// which is exactly how a broken installer stays green.
func TestTheInstallerIsPOSIXShellAndNotBash(t *testing.T) {
	path := filepath.Join(repoRoot(t), "install.sh")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	source := string(raw)

	if first, _, _ := strings.Cut(source, "\n"); first != "#!/bin/sh" {
		t.Errorf("install.sh starts with %q, not #!/bin/sh.\n"+
			"The documented command pipes it into `sh`, so the shebang is not what\n"+
			"chooses the shell — but it is what tells the next reader which one to\n"+
			"write for.", first)
	}

	// Each of these is something bash accepts and dash and BusyBox refuse.
	bashisms := []struct {
		name    string
		pattern *regexp.Regexp
	}{
		{"an array assignment, `name=(`", regexp.MustCompile(`(?m)^\s*[A-Za-z_][A-Za-z0-9_]*=\(`)},
		{"an array expansion, `${name[@]}`", regexp.MustCompile(`\$\{[A-Za-z_][A-Za-z0-9_]*\[[@*]\]\}`)},
		{"`[[ ... ]]`", regexp.MustCompile(`\[\[ `)},
		{"`local`", regexp.MustCompile(`(?m)^\s*local\s`)},
		{"`function name()`", regexp.MustCompile(`(?m)^\s*function\s+[A-Za-z_]`)},
		{"`echo -e`", regexp.MustCompile(`echo\s+-e\b`)},
		{"`set -o pipefail`", regexp.MustCompile(`pipefail`)},
		{"a here-string, `<<<`", regexp.MustCompile(`<<<`)},
		{"`$'...'`", regexp.MustCompile(`\$'`)},
		{"`==` inside `[ ]`", regexp.MustCompile(`\[ [^]]*[^=!<>]==`)},
		{"a brace expansion, `{a,b}`", regexp.MustCompile(`\{[A-Za-z0-9]+,[A-Za-z0-9]+\}`)},
		{"`+=`", regexp.MustCompile(`(?m)^\s*[A-Za-z_][A-Za-z0-9_]*\+=`)},
	}

	// Comments are prose and may name any of these while explaining why they are
	// absent, which is what the header of the installer does.
	var code []string
	for _, line := range strings.Split(source, "\n") {
		if trimmed := strings.TrimSpace(line); strings.HasPrefix(trimmed, "#") {
			continue
		}
		code = append(code, line)
	}

	for _, b := range bashisms {
		for _, line := range code {
			if b.pattern.MatchString(line) {
				t.Errorf("install.sh line %q uses %s.\n\n"+
					"That is bash. The installer runs under dash on Debian and Ubuntu and\n"+
					"under BusyBox on Alpine, where this is a syntax error before the first\n"+
					"check runs. Check a change with `dash -n install.sh` or\n"+
					"`podman run --rm -v $PWD/install.sh:/i.sh:ro alpine sh -n /i.sh`.",
					strings.TrimSpace(line), b.name)
				break
			}
		}
	}
}

// TestTheInstallerAndTheReleaseAgreeOnWhatIsPublished.
//
// The installer downloads the service unit from the release rather than writing
// one of its own, so the unit has to be in the release. Nothing else notices if
// it is not: the build succeeds, the release publishes, and the failure lands on
// whoever runs the install command.
func TestTheInstallerAndTheReleaseAgreeOnWhatIsPublished(t *testing.T) {
	root := repoRoot(t)

	unit := filepath.Join(root, "deploy", "lasthelp.service")
	if _, err := os.Stat(unit); err != nil {
		t.Fatalf("install.sh downloads lasthelp.service from the release, and deploy/lasthelp.service is missing: %v", err)
	}

	raw, err := os.ReadFile(filepath.Join(root, "scripts", "build-artefacts.sh"))
	if err != nil {
		t.Fatal(err)
	}
	// Comments stripped first. The header of that script explains why the unit
	// ships with the release, so a check that reads the whole file passes on a
	// build step somebody commented out — which is exactly the mutation this is
	// here to catch, and it did pass it once.
	var lines []string
	for _, line := range strings.Split(string(raw), "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		lines = append(lines, line)
	}
	build := strings.Join(lines, "\n")

	if !strings.Contains(build, "deploy/lasthelp.service") {
		t.Error("build-artefacts.sh no longer puts deploy/lasthelp.service in the release,\n" +
			"and install.sh downloads it from there. An install would stop at\n" +
			"\"the release has no service unit\".")
	}
	if !strings.Contains(build, "lasthelp.service > SHA256SUMS") {
		t.Error("the service unit is not in SHA256SUMS.\n" +
			"It is the sandbox the service runs inside; shipping it unsigned means the\n" +
			"one file that decides what a compromised process may do is the one file\n" +
			"nobody checked.")
	}
}
