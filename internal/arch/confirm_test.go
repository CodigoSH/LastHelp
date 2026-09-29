package arch

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// TestNothingAsksThroughTheBrowsersConfirm.
//
// Every question before something that cannot be undone goes through the
// product's own dialog (web/src/confirm.ts).
// window.confirm() is a grey box in the browser's voice, and ten of them had
// piled up — deleting a report, a comment, an account, an organisation, a
// repository, restoring a backup — while the product's dialog sat unused.
//
// What it catches: a bare `confirm(` or `window.confirm(` in code under
// web/src. What it does not: comments (removed before looking), and a method
// that happens to be called confirm on some object (`x.confirm(` is left
// alone, because that is not the browser's).
func TestNothingAsksThroughTheBrowsersConfirm(t *testing.T) {
	root := filepath.Join(repoRoot(t), "web", "src")
	call := regexp.MustCompile(`(^|[^.\w])(window\.)?confirm\(`)
	comment := regexp.MustCompile(`^\s*(//|\*|/\*)`)

	var files int
	var found []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".ts") || strings.HasSuffix(path, ".test.ts") {
			return nil
		}
		files++
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for i, line := range strings.Split(string(body), "\n") {
			if comment.MatchString(line) {
				continue
			}
			if call.MatchString(line) {
				rel, _ := filepath.Rel(repoRoot(t), path)
				found = append(found, rel+":"+strconv.Itoa(i+1)+": "+strings.TrimSpace(line))
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if files < 10 {
		t.Fatalf("read %d files under web/src: this rule is reading the wrong tree", files)
	}
	if len(found) > 0 {
		t.Errorf("the browser's confirm() is used; ask with askConfirm() from web/src/confirm.ts:\n  %s",
			strings.Join(found, "\n  "))
	}
}
