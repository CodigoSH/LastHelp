package web

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

// pinReplaceable says whether the binary can be replaced here, whatever the
// machine running the tests allows.
func pinReplaceable(t *testing.T, can bool) {
	t.Helper()
	was := replaceable
	replaceable = func() bool { return can }
	t.Cleanup(func() { replaceable = was })
}

// An installation whose binary cannot be replaced from here — one made from a
// container image, wherever it runs — is told so before anybody presses
// anything, and the server refuses the update if it is asked anyway.
func TestABinaryThatCannotBeReplacedIsNotOfferedAnUpdate(t *testing.T) {
	pinNotInAContainer(t)
	team := newInstance(t)
	team.claim()

	pinReplaceable(t, false)
	if got := decode(t, team.do(http.MethodGet, "/api/update", nil))["replaceable"]; got != false {
		t.Errorf("status of a binary that cannot be replaced: replaceable = %v", got)
	}
	resp := team.do(http.MethodPost, "/api/update", nil)
	body := decode(t, resp)
	if resp.StatusCode != http.StatusBadRequest || body["error"] != "error.update_not_replaceable" {
		t.Errorf("updating a binary that cannot be replaced: %d %v", resp.StatusCode, body)
	}

	pinReplaceable(t, true)
	if got := decode(t, team.do(http.MethodGet, "/api/update", nil))["replaceable"]; got != true {
		t.Errorf("status of a binary that can be replaced: replaceable = %v", got)
	}
}

// The question is asked of the directory itself.
func TestWritableDirAsksTheDirectory(t *testing.T) {
	dir := t.TempDir()
	if !writableDir(dir) {
		t.Fatal("a temporary directory of the test's own was not writable")
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Errorf("the check left %d files behind", len(entries))
	}
	if os.Geteuid() == 0 {
		t.Skip("root writes in a read-only directory; the refusal cannot be seen as root")
	}
	locked := filepath.Join(dir, "locked")
	if err := os.Mkdir(locked, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(locked, 0o755) })
	if writableDir(locked) {
		t.Error("a directory without write permission was called writable")
	}
}
