package web

import (
	"os"
	"path/filepath"
	"testing"
)

// TestEveryEngineIsRecognised.
//
// Inside a container the update button must not be offered: the image comes
// back on the next restart and undoes it. Each engine is told apart by the mark
// it leaves, and the one that was missed was Podman's — under cgroup v2 the
// cgroup of the first process names no engine at all, so an image run by
// Podman said it was not in a container.
func TestEveryEngineIsRecognised(t *testing.T) {
	for _, tc := range []struct {
		name  string
		files map[string]string
		want  bool
	}{
		{"docker", map[string]string{".dockerenv": ""}, true},
		{"podman", map[string]string{"run/.containerenv": ""}, true},
		{"cgroup v1 naming docker", map[string]string{"proc/1/cgroup": "12:pids:/docker/abc\n"}, true},
		{"a machine of its own, cgroup v2", map[string]string{"proc/1/cgroup": "0::/init.scope\n"}, false},
		{"nothing to go on", nil, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			for name, body := range tc.files {
				path := filepath.Join(root, name)
				if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			if got := inContainerUnder(root); got != tc.want {
				t.Errorf("inContainerUnder = %v, want %v", got, tc.want)
			}
		})
	}
}
