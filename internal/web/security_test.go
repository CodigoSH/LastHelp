// The security inventory.
//
// This file exists because a control nobody wrote is invisible. The content
// policy was in the plan for this layer from the first day, was never
// programmed, and nothing noticed: no test asked for it, no document promised
// it, and a browser does not complain about a header that is absent.
//
// So the list of controls lives at the bottom of this file as a test rather
// than as prose — each control beside the test that keeps it true, checked
// against the repository so that deleting one fails here with the control's
// name in the message. What is only tested here is what nothing else covered:
// what may be indexed, and when HSTS is sent.
//
// The rule for anything new: if it protects somebody, it is on that list before
// it is in the README.
package web

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"git.codigosh.com/CodigoSH/LastHelp/internal/web/middleware"
)

func TestPrivatePagesStayOutOfSearchAndPublicOnesDoNot(t *testing.T) {
	team := newInstance(t)
	team.claim()

	// The one that matters: a follow link is the key to somebody's own report,
	// and an indexed key is a published key.
	for _, path := range []string{"/panel", "/panel/people", "/sign-in", "/i/anything?k=secret"} {
		resp := team.do(http.MethodGet, path, nil)
		robots := resp.Header.Get("X-Robots-Tag")
		_ = resp.Body.Close()
		if !strings.Contains(robots, "noindex") {
			t.Errorf("%s may be indexed: X-Robots-Tag=%q", path, robots)
		}
	}

	// And the public side is meant to be found: that is what the product is for.
	for _, path := range []string{"/", "/codigosh/lastboard"} {
		resp := team.do(http.MethodGet, path, nil)
		robots := resp.Header.Get("X-Robots-Tag")
		_ = resp.Body.Close()
		if robots != "" {
			t.Errorf("%s is kept out of search: X-Robots-Tag=%q", path, robots)
		}
	}
}

func TestHSTSIsSentOverTLSAndOnlyThere(t *testing.T) {
	team := newInstance(t)

	resp := team.do(http.MethodGet, "/", nil)
	over := resp.Header.Get("Strict-Transport-Security")
	_ = resp.Body.Close()
	if over != "" {
		t.Errorf("plain HTTP was told to remember HTTPS: %q", over)
	}

	// And the other half, asked of the middleware with a request it considers
	// secure. The server hands it the same function the cookie's Secure flag
	// uses — two ideas of "over HTTPS" in one server is one of them being wrong.
	rec := httptest.NewRecorder()
	middleware.SecurityHeaders(func(*http.Request) bool { return true })(
		http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}),
	).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	got := rec.Header().Get("Strict-Transport-Security")
	if !strings.Contains(got, "max-age=") {
		t.Errorf("a request over TLS was not told to remember it: %q", got)
	}
	// preload and includeSubDomains are deliberately absent: both are one-way
	// doors over domains this server knows nothing about.
	if strings.Contains(got, "preload") || strings.Contains(got, "includeSubDomains") {
		t.Errorf("HSTS reaches beyond this host: %q", got)
	}
}

// TestTheSecurityInventoryIsComplete.
//
// The list, and the reason this file exists.
//
// Every control this product leans on is named here beside the test that keeps
// it true, and this walks the repository to check that each of those tests
// still exists. It is deliberately not a document: prose about security decays
// silently, and the failure that started all of this — a content policy that
// was planned, never written, and noticed by nobody for months — is exactly
// what a list with no teeth would have let happen again.
//
// Adding a control means adding a line here. Deleting a test named here fails
// this one, with the control it belonged to in the message.
func TestTheSecurityInventoryIsComplete(t *testing.T) {
	inventory := map[string]string{
		"a content policy on every response, with no unsafe-inline in it": "TestEveryResponseCarriesAPolicy",
		"the session cookie cannot be read by script":                     "TestTheSessionCookieIsNotReadableByScripts",
		"a write without the CSRF token is refused":                       "TestAWriteWithoutTheCSRFTokenIsRefused",
		"a write from another site is refused before the token":           "TestAWriteFromAnotherSiteIsRefusedBeforeAnythingElse",
		"nothing private is indexable, and a follow token never is":       "TestPrivatePagesStayOutOfSearchAndPublicOnesDoNot",
		"HSTS over TLS and only there":                                    "TestHSTSIsSentOverTLSAndOnlyThere",
		"passwords are hashed with argon2id, not truncated":               "TestALongPassphraseIsNotTruncated",
		"the forge token goes in and never comes back out":                "TestTheTokenNeverComesBackOutOfTheAPI",
		"nothing waiting for a person is public":                          "TestNothingWaitingIsPublic",
		"a caller cannot choose the address they are limited by":          "TestTheCallerCannotChooseTheirOwnAddress",
		"only authz decides what somebody may do":                         "TestOnlyAuthzDecidesPermissions",
		"a private route is private by being wrapped, not by a list":      "TestLayering",
		"every outbound connection goes through the gate":                 "TestEveryOutboundConnectionGoesThroughAGate",
		"a release is verified before it is installed":                    "TestTheInstallerAndTheBinaryTrustTheSameKeys",
		"the sandbox the service runs in is signed with the release":      "TestTheInstallerAndTheReleaseAgreeOnWhatIsPublished",
		"the installer runs on the shells it says it does":                "TestTheInstallerIsPOSIXShellAndNotBash",
	}

	root := repoRootFromHere(t)
	written := map[string]bool{}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, "_test.go") {
			return err
		}
		source, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, line := range strings.Split(string(source), "\n") {
			if name, ok := strings.CutPrefix(line, "func Test"); ok {
				if at := strings.IndexByte(name, '('); at > 0 {
					written["Test"+name[:at]] = true
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	for control, test := range inventory {
		if !written[test] {
			t.Errorf("nothing keeps %q true any more: %s is gone", control, test)
		}
	}
}

// repoRootFromHere climbs to the directory holding go.mod.
func repoRootFromHere(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("no go.mod above the working directory")
		}
		dir = parent
	}
}
