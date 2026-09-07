package updater

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"git.codigosh.com/CodigoSH/LastHelp/internal/platform/outbound"
)

// forge is a stand-in for our own release server: it serves a release, its
// checksums, the signature, and a binary — all in memory, no network.
type forge struct {
	server *httptest.Server

	tag       string
	binary    []byte
	sums      []byte
	signature []byte
	omitSig   bool
}

func newForge(t *testing.T, key ed25519.PrivateKey, tag string, binary []byte) *forge {
	t.Helper()

	name := fmt.Sprintf("lasthelp-%s-%s", runtime.GOOS, runtime.GOARCH)
	sum := sha256.Sum256(binary)
	sums := []byte(hex.EncodeToString(sum[:]) + "  " + name + "\n")

	f := &forge{
		tag: tag, binary: binary, sums: sums,
		signature: ed25519.Sign(key, sums),
	}

	f.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// The asset paths first, and deliberately: they live under
		// /releases/download/, so a Contains("/releases") arm placed above them
		// answers a download with a JSON release list. Which is what the first
		// version of this did, and every test then failed on a bad signature
		// while nothing was wrong with the signing.
		switch {
		case strings.HasSuffix(r.URL.Path, "SHA256SUMS.sig"):
			if f.omitSig {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			_, _ = w.Write([]byte(base64.StdEncoding.EncodeToString(f.signature) + "\n"))
		case strings.HasSuffix(r.URL.Path, "SHA256SUMS"):
			_, _ = w.Write(f.sums)
		case strings.HasSuffix(r.URL.Path, name):
			_, _ = w.Write(f.binary)
		case strings.HasSuffix(r.URL.Path, "/releases/latest"):
			fmt.Fprintf(w, `{"tag_name":%q,"body":"notes","draft":false,"prerelease":false}`, f.tag)
		case strings.Contains(r.URL.Path, "/releases"):
			fmt.Fprintf(w, `[{"tag_name":%q,"body":"notes","draft":false,"prerelease":true}]`, f.tag)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(f.server.Close)
	return f
}

// candidateBinary writes a small shell script that answers `version` the way the
// real binary does. It is enough to exercise "prove it runs", which is the
// point — the test is about the updater, not about Go compilation.
func candidateBinary(schema int) []byte {
	return []byte("#!/bin/sh\n" +
		`if [ "$1" = "version" ]; then echo '{"version":"v9.9.9","commit":"abc","schema":` +
		fmt.Sprint(schema) + `}'; exit 0; fi` + "\nexit 1\n")
}

func updaterFor(t *testing.T, f *forge, keys []string, schema int) (Updater, string) {
	t.Helper()

	// A stand-in for the running binary, in its own directory: Apply writes the
	// candidate beside it and renames over it.
	dir := t.TempDir()
	running := filepath.Join(dir, "lasthelp")
	if err := os.WriteFile(running, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	return Updater{
		Out:           outbound.New(false, 10*time.Second),
		APIBase:       f.server.URL + "/api/v1/repos/CodigoSH/LastHelp",
		DownloadBase:  f.server.URL + "/releases/download",
		Keys:          keys,
		Current:       "v1.0.0",
		SchemaVersion: func() (int, error) { return schema, nil },
		Now:           time.Now,
	}, running
}

// applyIn runs Apply with os.Executable pointing at our stand-in, which is what
// makes this testable at all: the real one replaces the test binary.
func applyIn(t *testing.T, u Updater, running string, release Release, dbPath string) error {
	t.Helper()
	original := executable
	executable = func() (string, error) { return running, nil }
	t.Cleanup(func() { executable = original })
	return u.Apply(context.Background(), release, dbPath, nil)
}

func keyPair(t *testing.T) (ed25519.PrivateKey, string) {
	t.Helper()
	public, private, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	return private, base64.StdEncoding.EncodeToString(public)
}

func TestAGoodUpdateIsInstalled(t *testing.T) {
	private, public := keyPair(t)
	f := newForge(t, private, "v1.1.0", candidateBinary(5))
	u, running := updaterFor(t, f, []string{public}, 2)

	release, err := u.Latest(context.Background(), false)
	if err != nil {
		t.Fatal(err)
	}
	if release.Tag != "v1.1.0" || !u.Available(release) {
		t.Fatalf("latest came back %+v, available=%v", release, u.Available(release))
	}

	if err := applyIn(t, u, running, release, ""); err != nil {
		t.Fatalf("a good update was refused: %v", err)
	}

	installed, err := os.ReadFile(running)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(installed), `"schema":5`) {
		t.Error("the binary was not replaced")
	}
	if _, err := os.Stat(running + ".candidate"); !os.IsNotExist(err) {
		t.Error("the candidate file was left behind")
	}
}

// TestAnUnsignedReleaseIsRefused.
//
// Never a fallback to checksum-only: a host that could tamper with the binary
// could also delete the signature, and falling back turns that into a way to
// strip the check rather than a failure.
func TestAnUnsignedReleaseIsRefused(t *testing.T) {
	private, public := keyPair(t)
	f := newForge(t, private, "v1.1.0", candidateBinary(5))
	f.omitSig = true

	u, running := updaterFor(t, f, []string{public}, 2)
	err := applyIn(t, u, running, Release{Tag: "v1.1.0"}, "")
	if !errors.Is(err, ErrUnsigned) {
		t.Fatalf("got %v, want ErrUnsigned", err)
	}
	assertNotReplaced(t, running)
}

func TestSomebodyElsesSignatureIsRefused(t *testing.T) {
	private, _ := keyPair(t)
	_, otherPublic := keyPair(t)
	f := newForge(t, private, "v1.1.0", candidateBinary(5))

	u, running := updaterFor(t, f, []string{otherPublic}, 2)
	if err := applyIn(t, u, running, Release{Tag: "v1.1.0"}, ""); !errors.Is(err, ErrBadSignature) {
		t.Fatalf("got %v, want ErrBadSignature", err)
	}
	assertNotReplaced(t, running)
}

// TestATamperedBinaryIsRefused.
//
// The checksums are signed; the binary is not. This is the step that connects
// the two, and without it the signature protects a list nothing is checked
// against.
func TestATamperedBinaryIsRefused(t *testing.T) {
	private, public := keyPair(t)
	f := newForge(t, private, "v1.1.0", candidateBinary(5))
	// The server hands out something other than what was signed.
	f.binary = append(f.binary, []byte("\n# and a little extra\n")...)

	u, running := updaterFor(t, f, []string{public}, 2)
	if err := applyIn(t, u, running, Release{Tag: "v1.1.0"}, ""); !errors.Is(err, ErrBadChecksum) {
		t.Fatalf("got %v, want ErrBadChecksum", err)
	}
	assertNotReplaced(t, running)
}

// TestABuildWithNoKeysWillNotUpdate.
//
// No key does not mean accept anything.
func TestABuildWithNoKeysWillNotUpdate(t *testing.T) {
	private, _ := keyPair(t)
	f := newForge(t, private, "v1.1.0", candidateBinary(5))

	u, running := updaterFor(t, f, nil, 2)
	if err := applyIn(t, u, running, Release{Tag: "v1.1.0"}, ""); !errors.Is(err, ErrNoKeys) {
		t.Fatalf("got %v, want ErrNoKeys", err)
	}
	assertNotReplaced(t, running)
}

// TestABinaryThatDoesNotRunIsRefusedBeforeTheSwap.
//
// The wrong architecture, a truncated download, a library this machine does not
// have — every way a download can be intact and useless. Finding out after the
// swap means finding out by being down.
func TestABinaryThatDoesNotRunIsRefusedBeforeTheSwap(t *testing.T) {
	private, public := keyPair(t)
	f := newForge(t, private, "v1.1.0", []byte("this is not a program"))

	u, running := updaterFor(t, f, []string{public}, 2)
	if err := applyIn(t, u, running, Release{Tag: "v1.1.0"}, ""); !errors.Is(err, ErrWontRun) {
		t.Fatalf("got %v, want ErrWontRun", err)
	}
	assertNotReplaced(t, running)
}

// TestABinaryOlderThanTheDatabaseIsRefused.
//
// With a beta channel somebody will eventually install an older binary over a
// newer database. The failure without this guard is not a crash: it is an old
// binary writing rows that ignore columns it does not know about.
func TestABinaryOlderThanTheDatabaseIsRefused(t *testing.T) {
	private, public := keyPair(t)
	f := newForge(t, private, "v1.1.0", candidateBinary(1))

	u, running := updaterFor(t, f, []string{public}, 4) // database is at 4
	err := applyIn(t, u, running, Release{Tag: "v1.1.0"}, "")
	if !errors.Is(err, ErrOlderThanDatabase) {
		t.Fatalf("got %v, want ErrOlderThanDatabase", err)
	}
	if !strings.Contains(err.Error(), "schema 1") || !strings.Contains(err.Error(), "at 4") {
		t.Errorf("the message does not say which is which: %v", err)
	}
	assertNotReplaced(t, running)
}

// TestTheDatabaseIsBackedUpBeforeAnythingIsReplaced.
//
// The moment somebody wants a backup is after it is too late to take one.
func TestTheDatabaseIsBackedUpBeforeAnythingIsReplaced(t *testing.T) {
	private, public := keyPair(t)
	f := newForge(t, private, "v1.1.0", candidateBinary(5))
	u, running := updaterFor(t, f, []string{public}, 2)

	db := filepath.Join(filepath.Dir(running), "lasthelp.db")
	if err := os.WriteFile(db, []byte("the database as it was"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := applyIn(t, u, running, Release{Tag: "v1.1.0"}, db); err != nil {
		t.Fatal(err)
	}

	saved, err := os.ReadFile(db + ".before-v1.1.0")
	if err != nil {
		t.Fatalf("no backup was taken: %v", err)
	}
	if string(saved) != "the database as it was" {
		t.Errorf("the backup holds %q", saved)
	}
}

// TestVersionsCompareAsNumbers.
//
// Comparing tags as strings makes v1.10.0 older than v1.9.0 — a bug that only
// turns up at the tenth release, by which point nobody is looking at this.
func TestVersionsCompareAsNumbers(t *testing.T) {
	cases := []struct {
		candidate, current string
		want               bool
	}{
		{"v1.1.0", "v1.0.0", true},
		{"v1.10.0", "v1.9.0", true}, // the one string comparison gets wrong
		{"v1.9.0", "v1.10.0", false},
		{"v2.0.0", "v1.99.99", true},
		{"v1.0.0", "v1.0.0", false},
		{"v1.0.0", "v1.0.1", false},
		{"v1.0.0", "v1.0.0-Beta.01", true},  // a release beats its own beta
		{"v1.0.0-Beta.02", "v1.0.0", false}, // and nothing beats a release
		{"v1.2.0-Beta.01", "v1.1.0", true},
	}

	for _, c := range cases {
		if got := newer(c.candidate, c.current); got != c.want {
			t.Errorf("newer(%q, %q) = %v, want %v", c.candidate, c.current, got, c.want)
		}
	}
}

// TestADraftReleaseIsSkipped.
//
// A draft is a release mid-publication: some files attached and some not.
// Installing one is installing half.
func TestADraftReleaseIsSkipped(t *testing.T) {
	private, _ := keyPair(t)
	f := newForge(t, private, "v1.1.0", candidateBinary(5))
	f.server.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`[{"tag_name":"v2.0.0","draft":true},{"tag_name":"v1.1.0","draft":false}]`))
	})

	u, _ := updaterFor(t, f, nil, 2)
	release, err := u.Latest(context.Background(), true)
	if err != nil {
		t.Fatal(err)
	}
	if release.Tag != "v1.1.0" {
		t.Errorf("picked %q; a draft is half a release", release.Tag)
	}
}

// TestTheOfflineSwitchStopsUpdatesToo.
func TestTheOfflineSwitchStopsUpdatesToo(t *testing.T) {
	private, public := keyPair(t)
	f := newForge(t, private, "v1.1.0", candidateBinary(5))
	u, running := updaterFor(t, f, []string{public}, 2)
	u.Out = outbound.New(true, time.Second)

	if _, err := u.Latest(context.Background(), false); !errors.Is(err, outbound.ErrOffline) {
		t.Errorf("checking for updates got out while offline: %v", err)
	}
	if err := applyIn(t, u, running, Release{Tag: "v1.1.0"}, ""); err == nil {
		t.Error("an update was installed while offline")
	}
	assertNotReplaced(t, running)
}

func assertNotReplaced(t *testing.T, running string) {
	t.Helper()
	content, err := os.ReadFile(running)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(content), "version") {
		t.Error("the binary was replaced despite the refusal")
	}
	if _, err := os.Stat(running + ".candidate"); !os.IsNotExist(err) {
		t.Error("a candidate binary was left next to the running one")
	}
}
