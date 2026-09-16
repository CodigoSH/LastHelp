package arch

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestNoInstanceSecretIsTracked.
//
// `lasthelp.key` holds the key that decrypts the forge tokens, and it was
// committed and pushed: .gitignore listed the database and every shape of it
// but not the key, and running the binary from the repository root creates both
// here, because keyFile() defaults to sitting next to the database. It lived in
// the public history for eight days, inside a commit about the report form,
// where nobody reviewing that change would have looked for it.
//
// The .gitignore line stops it being created again. This stops it being
// committed if the line is ever removed, if somebody adds a file with -f, or if
// the next secret arrives under a name nobody thought to ignore.
//
// It asks git rather than the filesystem: what matters is not whether the file
// is there — during development it usually is — but whether it is tracked.
func TestNoInstanceSecretIsTracked(t *testing.T) {
	root := repoRoot(t)

	out, err := exec.Command("git", "-C", root, "ls-files").Output()
	if err != nil {
		t.Skipf("git is not available here: %v", err)
	}

	tracked := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(tracked) < 50 {
		t.Fatalf("git listed only %d tracked files: this test is reading the wrong tree,"+
			" and a listing that finds nothing passes everything", len(tracked))
	}

	// Extensions that only ever belong to a running instance, never to source.
	secret := map[string]string{
		".key":     "the key that decrypts the forge tokens",
		".pem":     "a private key",
		".db":      "an instance database",
		".sqlite":  "an instance database",
		".sqlite3": "an instance database",
	}

	for _, path := range tracked {
		what, isSecret := secret[strings.ToLower(filepath.Ext(path))]
		if !isSecret {
			continue
		}
		t.Errorf("\n%s is tracked by git, and it is %s.\n\n"+
			"Nothing belonging to a running instance goes in the repository. If this\n"+
			"has been pushed, the secret is public: it has to be treated as\n"+
			"compromised and rotated, not only deleted.", path, what)
	}
}
