package web

import (
	"errors"
	"os"
	"syscall"
	"testing"
)

// TestAFailedRestartExitsNonZero.
//
// The unit says `Restart=on-failure`, and that is the whole of the recovery
// plan for an update that replaced the binary and then could not re-exec into
// it. systemd reads the exit code to decide: zero means the service did what it
// was asked and is finished, so it is left stopped.
//
// This exited zero. The person pressing the button had been told the update
// worked, the binary on disk was the new one, and the server was down with
// nothing in systemd's view suggesting anything was wrong.
func TestAFailedRestartExitsNonZero(t *testing.T) {
	for _, tc := range []struct {
		name     string
		path     func() (string, error)
		exec     func(string, []string, []string) error
		wantExit int
	}{
		{
			name:     "exec refuses",
			path:     func() (string, error) { return "/opt/lasthelp/bin/lasthelp", nil },
			exec:     func(string, []string, []string) error { return syscall.ENOEXEC },
			wantExit: 1,
		},
		{
			name:     "we cannot find ourselves",
			path:     func() (string, error) { return "", errors.New("no /proc") },
			exec:     func(string, []string, []string) error { t.Fatal("exec should not have been reached"); return nil },
			wantExit: 1,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			oldPath, oldExec, oldExit := executablePath, execSelf, exitProcess
			defer func() { executablePath, execSelf, exitProcess = oldPath, oldExec, oldExit }()

			got := 0
			called := false
			executablePath = tc.path
			execSelf = tc.exec
			exitProcess = func(code int) { got, called = code, true }

			restart()

			if !called {
				t.Fatal("it did not exit at all, so the service is left running the old binary in memory " +
					"with the new one on disk and nothing to reconcile them")
			}
			if got != tc.wantExit {
				t.Errorf("exited %d, want %d — systemd restarts on failure, and %d is not a failure",
					got, tc.wantExit, got)
			}
		})
	}
}

// TestTheUnitOnlyRestartsOnFailure is the other half, and the reason the number
// above matters. If the unit is ever changed to Restart=always, the exit code
// stops deciding anything and this pair of tests should be revisited together.
func TestTheUnitOnlyRestartsOnFailure(t *testing.T) {
	unit, err := os.ReadFile("../../deploy/lasthelp.service")
	if err != nil {
		t.Fatalf("%v — this test is reading the wrong tree", err)
	}
	if !containsLine(string(unit), "Restart=on-failure") {
		t.Fatal("the unit no longer says Restart=on-failure, so what a non-zero exit\n" +
			"buys is no longer what TestAFailedRestartExitsNonZero assumes.")
	}
}

func containsLine(body, want string) bool {
	for _, line := range splitLines(body) {
		if line == want {
			return true
		}
	}
	return false
}

func splitLines(body string) []string {
	var out []string
	start := 0
	for i := 0; i < len(body); i++ {
		if body[i] == '\n' {
			out = append(out, trimSpace(body[start:i]))
			start = i + 1
		}
	}
	out = append(out, trimSpace(body[start:]))
	return out
}

func trimSpace(s string) string {
	for len(s) > 0 && (s[0] == ' ' || s[0] == '\t' || s[0] == '\r') {
		s = s[1:]
	}
	for len(s) > 0 && (s[len(s)-1] == ' ' || s[len(s)-1] == '\t' || s[len(s)-1] == '\r') {
		s = s[:len(s)-1]
	}
	return s
}
