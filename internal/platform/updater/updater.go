// Package updater replaces this binary with a newer one, or refuses to.
//
// Everything here is written around one idea: **the update path is the most
// dangerous code in a self-hosted product.** It runs as whoever runs the
// server, it fetches something from the internet, and it puts that something
// where the operating system will execute it. So every step either proves
// something or stops.
//
// The order is deliberate and is the same as install.sh's:
//
//  1. Refuse outright if this build has no release keys. No key does not mean
//     accept anything.
//  2. Fetch the checksum file, then its signature. A missing signature aborts —
//     it never falls back to checksum-only, because a host that could tamper
//     with the binary could also delete the signature, and a fallback turns
//     that into a downgrade attack rather than a failure.
//  3. Verify the signature. Only a verified checksum file is allowed to say
//     what the binary should hash to.
//  4. Fetch the binary, bounded.
//  5. Check it against the verified checksums.
//  6. Run it. It has to start on this machine and report what it is.
//  7. Refuse it if it understands a lower schema than the database is at.
//  8. Back up the database.
//  9. Only now, replace and restart.
package updater

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"git.codigosh.com/CodigoSH/LastHelp/internal/platform/outbound"
)

var (
	// ErrNoKeys means this build cannot verify anything, so it will not update.
	ErrNoKeys = errors.New("this build has no release keys, so it cannot verify an update")

	// ErrUnsigned means the release has no signature. Never a fallback.
	ErrUnsigned = errors.New("that release is not signed")

	// ErrBadSignature means it is signed by something we do not trust.
	ErrBadSignature = errors.New("the checksums are not signed by a key this build trusts")

	// ErrBadChecksum means the binary is not the one that was signed.
	ErrBadChecksum = errors.New("the downloaded binary does not match the signed checksums")

	// ErrWontRun means the candidate could not start here.
	ErrWontRun = errors.New("the downloaded binary did not run on this machine")

	// ErrOlderThanDatabase means installing it would leave a binary that does
	// not understand its own database.
	ErrOlderThanDatabase = errors.New("that release is older than this database")

	// ErrNoRelease means there is nothing to install.
	ErrNoRelease = errors.New("no release found")
)

// MaxBinaryBytes bounds the download. A release binary is tens of megabytes,
// and an unbounded read from anywhere is a way to fill a disk — which is the
// same class of problem as a decompression bomb, avoided here by shipping a
// plain binary rather than an archive: there is nothing to decompress.
const MaxBinaryBytes = 200 << 20

// downloadTimeout is how long the binary may take to arrive: tens of megabytes
// on a slow line, where the gate's own deadline is sized for an API call.
const downloadTimeout = 10 * time.Minute

// statusError carries the code, because 404 means something different from
// every other answer: nothing is published, rather than nothing is reachable.
type statusError struct {
	url  string
	code int
}

func (e statusError) Error() string { return fmt.Sprintf("%s answered %d", e.url, e.code) }

// Release is what could be installed.
type Release struct {
	Tag        string `json:"tag"`
	Notes      string `json:"notes"`
	Prerelease bool   `json:"prerelease"`
}

// Candidate is what a downloaded binary says about itself.
type Candidate struct {
	Version string `json:"version"`
	Commit  string `json:"commit"`
	Schema  int    `json:"schema"`
}

// Updater checks for and installs releases.
type Updater struct {
	Out *outbound.Client

	// APIBase is the forge API for our own repository, and DownloadBase is
	// where its release assets live. Both are ours: this is the one outbound
	// address that is not the operator's choice, and SECURITY.md names it.
	APIBase      string
	DownloadBase string

	// Keys are the public halves this build will accept a release from.
	Keys []string

	// Current is the running version.
	Current string

	// SchemaVersion is what the database is at right now.
	SchemaVersion func() (int, error)

	// Now, so a test does not have to wait.
	Now func() time.Time
}

// Step names what is happening, for the progress a person watches.
type Step string

const (
	StepChecksums Step = "checksums"
	StepSignature Step = "signature"
	StepDownload  Step = "download"
	StepVerify    Step = "verify"
	StepProve     Step = "prove"
	StepBackup    Step = "backup"
	StepInstall   Step = "install"
)

// Latest returns the newest release, or ErrNoRelease.
func (u Updater) Latest(ctx context.Context, includeBeta bool) (Release, error) {
	endpoint := strings.TrimSuffix(u.APIBase, "/") + "/releases/latest"
	if includeBeta {
		endpoint = strings.TrimSuffix(u.APIBase, "/") + "/releases?limit=1"
	}

	body, err := u.fetch(ctx, endpoint, 1<<20)
	if err != nil {
		// A forge with nothing published on this channel answers 404, and that
		// is not a failure to reach it. Reported as a transport error it became
		// "could not reach the release server" in the panel, which is the one
		// sentence that sends an operator to look at their network — while
		// every install was in exactly this state: three betas and no stable,
		// so the stable channel had nothing to find.
		var status statusError
		if errors.As(err, &status) && status.code == http.StatusNotFound {
			return Release{}, ErrNoRelease
		}
		return Release{}, err
	}

	type release struct {
		Tag        string `json:"tag_name"`
		Body       string `json:"body"`
		Draft      bool   `json:"draft"`
		Prerelease bool   `json:"prerelease"`
	}

	var one release
	if err := json.Unmarshal(body, &one); err == nil && one.Tag != "" {
		return Release{Tag: one.Tag, Notes: one.Body, Prerelease: one.Prerelease}, nil
	}

	var many []release
	if err := json.Unmarshal(body, &many); err != nil {
		return Release{}, fmt.Errorf("reading the release list: %w", err)
	}
	for _, r := range many {
		if r.Draft {
			// A draft is a release mid-publication, with some of its files
			// attached and some not. Installing one is installing half.
			continue
		}
		return Release{Tag: r.Tag, Notes: r.Body, Prerelease: r.Prerelease}, nil
	}
	return Release{}, ErrNoRelease
}

// Available says whether a release is newer than what is running.
func (u Updater) Available(release Release) bool {
	return newer(release.Tag, u.Current)
}

// Apply installs a release, or refuses.
//
// progress may be nil. It is called at each step so a person watching has
// something better than a spinner — and so that a failure says which step.
func (u Updater) Apply(ctx context.Context, release Release, dbPath string, progress func(Step, string)) error {
	report := func(step Step, detail string) {
		if progress != nil {
			progress(step, detail)
		}
	}

	if len(u.Keys) == 0 {
		return ErrNoKeys
	}

	base := strings.TrimSuffix(u.DownloadBase, "/") + "/" + release.Tag
	assetName := fmt.Sprintf("lasthelp-%s-%s", runtime.GOOS, runtime.GOARCH)

	report(StepChecksums, "SHA256SUMS")
	sums, err := u.fetch(ctx, base+"/SHA256SUMS", 1<<20)
	if err != nil {
		return fmt.Errorf("downloading the checksums: %w", err)
	}

	report(StepSignature, "SHA256SUMS.sig")
	rawSig, err := u.fetch(ctx, base+"/SHA256SUMS.sig", 4<<10)
	if err != nil {
		// Never a fallback to checksum-only. A host that could tamper with the
		// binary could also delete the signature, and falling back would turn
		// that into a way to strip the check rather than a failure.
		return ErrUnsigned
	}
	if err := u.verifySignature(sums, rawSig); err != nil {
		return err
	}

	report(StepDownload, assetName)
	binary, err := u.fetch(ctx, base+"/"+assetName, MaxBinaryBytes)
	if err != nil {
		return fmt.Errorf("downloading %s: %w", assetName, err)
	}

	report(StepVerify, assetName)
	if err := checkChecksum(sums, assetName, binary); err != nil {
		return err
	}

	// Written next to the running binary, not in /tmp: /tmp is often mounted
	// noexec, and a candidate that cannot be executed cannot be made to prove
	// anything. It is also the directory the final rename has to happen in,
	// because a rename across filesystems is not atomic.
	running, err := executable()
	if err != nil {
		return fmt.Errorf("finding the running binary: %w", err)
	}
	running, err = filepath.EvalSymlinks(running)
	if err != nil {
		return fmt.Errorf("resolving the running binary: %w", err)
	}

	candidatePath := running + ".candidate"
	if err := os.WriteFile(candidatePath, binary, 0o755); err != nil {
		return fmt.Errorf("writing the candidate: %w", err)
	}
	defer func() { _ = os.Remove(candidatePath) }()

	report(StepProve, "asking it what it is")
	candidate, err := prove(ctx, candidatePath)
	if err != nil {
		return err
	}

	current, err := u.SchemaVersion()
	if err != nil {
		return fmt.Errorf("reading the database schema version: %w", err)
	}
	if candidate.Schema < current {
		// The failure without this is not a crash: it is an older binary
		// writing rows that ignore columns it does not know about, and nobody
		// noticing until the newer one comes back.
		return fmt.Errorf("%w: it understands schema %d and this database is at %d",
			ErrOlderThanDatabase, candidate.Schema, current)
	}

	if dbPath != "" {
		report(StepBackup, filepath.Base(dbPath))
		if err := backup(dbPath, release.Tag); err != nil {
			return fmt.Errorf("backing up the database: %w", err)
		}
	}

	report(StepInstall, candidate.Version)
	// Rename, not copy: it is atomic within a filesystem, so there is no moment
	// where the file on disk is half a binary. Replacing a running executable
	// this way is fine on Linux — the running process keeps its own inode.
	if err := os.Rename(candidatePath, running); err != nil {
		return fmt.Errorf("replacing the binary: %w", err)
	}
	return nil
}

// verifySignature accepts the checksums only if one of this build's keys signed
// them.
func (u Updater) verifySignature(sums, rawSig []byte) error {
	signature, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(rawSig)))
	if err != nil {
		return ErrBadSignature
	}

	for _, encoded := range u.Keys {
		key, err := base64.StdEncoding.DecodeString(strings.TrimSpace(encoded))
		if err != nil || len(key) != ed25519.PublicKeySize {
			continue
		}
		if ed25519.Verify(ed25519.PublicKey(key), sums, signature) {
			return nil
		}
	}
	return ErrBadSignature
}

// checkChecksum compares the download against the line for it in the verified
// checksum file.
func checkChecksum(sums []byte, name string, content []byte) error {
	sum := sha256.Sum256(content)
	want := hex.EncodeToString(sum[:])

	for _, line := range strings.Split(string(sums), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 {
			continue
		}
		if strings.TrimPrefix(fields[1], "*") != name {
			continue
		}
		if fields[0] == want {
			return nil
		}
		return ErrBadChecksum
	}
	// Listed nowhere. Not "no checksum, carry on": an asset the signed file does
	// not mention is one nobody signed.
	return fmt.Errorf("%w: %s is not listed in the signed checksums", ErrBadChecksum, name)
}

// prove runs the candidate and reads what it says about itself.
//
// This is what catches a binary for the wrong architecture, one that was
// truncated, one built against libraries this machine does not have — every
// way a download can be intact and still useless. Doing it after the swap means
// finding out by being down.
func prove(ctx context.Context, path string) (Candidate, error) {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()

	out, err := exec.CommandContext(ctx, path, "version").Output()
	if err != nil {
		return Candidate{}, fmt.Errorf("%w: %v", ErrWontRun, err)
	}

	var candidate Candidate
	if err := json.Unmarshal(bytes.TrimSpace(out), &candidate); err != nil {
		return Candidate{}, fmt.Errorf("%w: it did not say what it is", ErrWontRun)
	}
	if candidate.Version == "" || candidate.Schema <= 0 {
		return Candidate{}, fmt.Errorf("%w: it answered %q", ErrWontRun, string(out))
	}
	return candidate, nil
}

// backup copies the database beside itself, named after the release going in.
//
// Before anything is replaced. An update that goes wrong with no backup is an
// update nobody can undo, and the moment somebody wants one is after it is too
// late to take.
func backup(dbPath, tag string) error {
	source, err := os.Open(dbPath)
	if err != nil {
		return err
	}
	defer func() { _ = source.Close() }()

	target := dbPath + ".before-" + strings.NewReplacer("/", "-", " ", "-").Replace(tag)
	destination, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	defer func() { _ = destination.Close() }()

	if _, err := io.Copy(destination, source); err != nil {
		return err
	}
	return destination.Sync()
}

// fetch downloads one file of a release, up to limit bytes.
//
// The client is bounded here, per file, rather than trusted to be: the gate's
// own bound is sized for forge replies, and a release binary is larger than
// that. Relying on whoever built the client meant every update downloaded the
// first 8 MiB of the binary and failed its checksum.
func (u Updater) fetch(ctx context.Context, url string, limit int64) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")

	// A binary gets the time a slow line needs; the context the caller passed
	// is still the outer bound.
	timeout := time.Duration(0)
	if limit > outbound.MaxResponseBytes {
		timeout = downloadTimeout
	}
	resp, err := u.Out.Bounded(limit, timeout).Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, statusError{url: url, code: resp.StatusCode}
	}
	return io.ReadAll(io.LimitReader(resp.Body, limit))
}

// newer compares two version tags.
//
// Numeric segments compared as numbers — comparing tags as strings makes
// v1.10.0 older than v1.9.0, which is the kind of bug that only appears at the
// tenth release — and pre-releases ordered the way Semantic Versioning orders
// them.
//
// That second half was missing: with the same numbers, a pre-release never
// beat another pre-release, so an installation on Beta.03 was told Beta.04 was
// not newer and every beta stayed on the one it was installed from.
func newer(candidate, current string) bool {
	c, cPre := splitVersion(candidate)
	r, rPre := splitVersion(current)

	for i := 0; i < 3; i++ {
		if c[i] != r[i] {
			return c[i] > r[i]
		}
	}
	switch {
	case cPre == "" || rPre == "":
		// A release beats a pre-release of it, and nothing beats a release.
		return rPre != "" && cPre == ""
	default:
		return comparePre(cPre, rPre) > 0
	}
}

// comparePre orders two pre-release labels by the Semantic Versioning rules:
// dot-separated identifiers left to right, numbers as numbers ("Beta.10" after
// "Beta.9", and "04" is 4), words by their letters, a number before a word, and
// a longer label after a shorter one it begins with. Letters are compared
// without case, so "beta" and "Beta" are the same channel and "rc" still comes
// after both.
func comparePre(a, b string) int {
	as, bs := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < len(as) && i < len(bs); i++ {
		x, xErr := strconv.Atoi(as[i])
		y, yErr := strconv.Atoi(bs[i])
		switch {
		case xErr == nil && yErr == nil:
			if x != y {
				return cmpInt(x, y)
			}
		case xErr == nil:
			return -1
		case yErr == nil:
			return 1
		default:
			if c := strings.Compare(strings.ToLower(as[i]), strings.ToLower(bs[i])); c != 0 {
				return c
			}
		}
	}
	return cmpInt(len(as), len(bs))
}

func cmpInt(a, b int) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

// Prerelease says whether a version is a pre-release: v1.0.0-Beta.04 is,
// v1.0.0 is not.
func Prerelease(tag string) bool {
	_, pre := splitVersion(tag)
	return pre != ""
}

func splitVersion(tag string) ([3]int, string) {
	tag = strings.TrimPrefix(strings.TrimSpace(tag), "v")

	pre := ""
	for _, sep := range []string{"-", "+"} {
		if index := strings.Index(tag, sep); index >= 0 {
			pre = tag[index+1:]
			tag = tag[:index]
			break
		}
	}

	var parts [3]int
	for i, segment := range strings.SplitN(tag, ".", 3) {
		if i > 2 {
			break
		}
		n, _ := strconv.Atoi(segment)
		parts[i] = n
	}
	return parts, pre
}

// executable is a variable so a test can point it at a stand-in. The real one
// would have the test replace the test binary, which is a memorable way to
// discover that a test did something irreversible.
var executable = os.Executable
