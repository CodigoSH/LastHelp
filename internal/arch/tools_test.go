package arch

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestVerificationToolsStayOutOfWhatWeShip.
//
// internal/forge/fake is a forge that answers "yes" to everything and signs its
// own deliveries. That is exactly what a test needs and exactly what must never
// be reachable from a release: a binary that could be pointed at it is a binary
// with a forge that approves anything.
//
// It finds nothing today. That is the point of writing it now — the import that
// would break this is one somebody adds while debugging, at four in the
// afternoon, meaning to take it out again.
func TestVerificationToolsStayOutOfWhatWeShip(t *testing.T) {
	const tool = "internal/forge/fake"

	for _, f := range loadSources(t) {
		if strings.HasSuffix(f.path, "_test.go") || under(f.pkg, []string{tool}) {
			continue
		}
		for _, imp := range f.imports() {
			if under(imp, []string{tool}) {
				t.Errorf("\n%s imports %s\n\n"+
					"That package is a forge which answers yes to everything and signs its own\n"+
					"deliveries. It belongs to tests. Reachable from a release, it is a way to\n"+
					"point this server at a forge that approves anything.", f.path, imp)
			}
		}
	}
}

// TestEveryAdapterUsesTheGate.
//
// The outbound rule already forbids constructing an http.Client outside the
// gate. This is the other direction: an adapter that talks to somebody else's
// server must actually hold the gate, rather than being handed a plain client
// by a caller who meant well.
func TestEveryAdapterUsesTheGate(t *testing.T) {
	seen := false
	for _, f := range loadSources(t) {
		if !under(f.pkg, []string{"internal/forge"}) || strings.HasSuffix(f.path, "_test.go") {
			continue
		}
		if under(f.pkg, []string{"internal/forge/fake"}) {
			continue
		}
		seen = true

		src := f.render(t)
		if !strings.Contains(src, "outbound.Client") && strings.Contains(src, "http.NewRequest") {
			t.Errorf("\n%s builds requests without holding the outbound gate\n\n"+
				"Every connection to somebody else's server goes through\n"+
				"internal/platform/outbound: it is what the offline switch stops, what\n"+
				"bounds the response, and what refuses a redirect to another host.", f.path)
		}
	}
	if !seen {
		t.Error("no adapter files were read; this rule is checking nothing")
	}
}

// TestTheSigningToolStaysOutOfWhatWeShip.
//
// cmd/sign holds the code that turns a private key into a signature. It is a
// separate binary and must stay one: reachable from the server, it is a signing
// tool on every user's machine, and the only thing standing between somebody
// with the binary and a release that looks like ours is the key — which is one
// mistake away from being somewhere it can be read.
func TestTheSigningToolStaysOutOfWhatWeShip(t *testing.T) {
	for _, f := range loadSources(t) {
		if strings.HasSuffix(f.path, "_test.go") || under(f.pkg, []string{"cmd/sign"}) {
			continue
		}
		for _, imp := range f.imports() {
			if under(imp, []string{"cmd/sign"}) {
				t.Errorf("\n%s imports %s\n\n"+
					"The signing tool is a separate binary. Inside the server it ships to every "+
					"user, and what stands between them and a release that looks like ours "+
					"becomes the key alone.", f.path, imp)
			}
		}
	}
}

// TestTheReleaseKeysAreAList.
//
// A binary verifies only with the keys it was built with. Replacing the key
// rather than appending leaves everybody on an older version unable to verify
// anything — exactly the people who most need to update — and the mistake is
// invisible until somebody tries.
func TestTheReleaseKeysAreAList(t *testing.T) {
	source, err := os.ReadFile(filepath.Join(repoRoot(t), "internal/version/keys.go"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(source), "ReleaseKeys = []string{") {
		t.Error("ReleaseKeys is no longer a list; rotation has to be overlapping, so there " +
			"has to be room for two keys at once")
	}
}

// TestTheInstallerAndTheBinaryTrustTheSameKeys.
//
// There are two lists of release keys: the one compiled into the binary and the
// one in install.sh. They have to hold the same keys, and the way they stop
// doing so is somebody rotating a key in one place — after which either every
// new install fails to verify, or the updater does, and only one of those gets
// noticed.
func TestTheInstallerAndTheBinaryTrustTheSameKeys(t *testing.T) {
	root := repoRoot(t)

	goKeys, err := os.ReadFile(filepath.Join(root, "internal/version/keys.go"))
	if err != nil {
		t.Fatal(err)
	}
	installer, err := os.ReadFile(filepath.Join(root, "install.sh"))
	if err != nil {
		t.Fatal(err)
	}

	// Double quotes in Go, single quotes in the shell: the installer keeps its
	// keys as a newline-separated string rather than an array, because an array
	// is bash and that script has to parse under dash and BusyBox.
	key := regexp.MustCompile(`"([A-Za-z0-9+/]{42,44}=*)"`)
	shellKey := regexp.MustCompile(`([A-Za-z0-9+/]{43}=)`)
	inBinary := key.FindAllStringSubmatch(string(goKeys), -1)
	inInstaller := shellKey.FindAllStringSubmatch(string(installer), -1)

	if len(inBinary) != len(inInstaller) {
		t.Errorf("\nthe binary trusts %d release key(s) and install.sh trusts %d\n\n"+
			"They have to hold the same keys. When they diverge, either every new install\n"+
			"stops verifying or the updater does — and only one of those gets noticed.",
			len(inBinary), len(inInstaller))
		return
	}

	for i := range inBinary {
		if inBinary[i][1] != inInstaller[i][1] {
			t.Errorf("release key %d differs between the binary and install.sh", i+1)
		}
	}
}
