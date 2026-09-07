package outbound

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func get(t *testing.T, c *Client, url string) (*http.Response, error) {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	return c.Do(req)
}

// TestPrivateAddressesAreAllowed is the test that stops a future audit from
// "fixing" this.
//
// Pointing at a Gitea on the LAN is the feature. A well-meaning change that
// starts refusing private addresses would break the product for the people it
// is built for, and it would pass every other test in the repository.
func TestPrivateAddressesAreAllowed(t *testing.T) {
	// httptest listens on loopback, which is exactly the address the usual
	// advice says to refuse.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("a forge"))
	}))
	defer srv.Close()

	resp, err := get(t, New(false, time.Second), srv.URL)
	if err != nil {
		t.Fatalf("loopback was refused: %v\n\n"+
			"Private, loopback and link-local addresses are allowed here on purpose: "+
			"the URL comes from the operator, and pointing at their own forge is the "+
			"whole point.", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, _ := io.ReadAll(resp.Body)
	if string(body) != "a forge" {
		t.Errorf("body is %q", body)
	}
}

func TestCloudMetadataIsRefused(t *testing.T) {
	c := New(false, time.Second)
	for _, addr := range []string{
		"http://169.254.169.254/latest/meta-data/",
		"http://[fd00:ec2::254]/",
		"http://100.100.100.200/",
	} {
		if _, err := get(t, c, addr); err == nil {
			t.Errorf("%s was allowed; no forge lives there and what does live there hands out credentials", addr)
		}
	}
}

func TestTheOfflineSwitchStopsEverything(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer srv.Close()

	_, err := get(t, New(true, time.Second), srv.URL)
	if !errors.Is(err, ErrOffline) {
		t.Fatalf("offline did not stop the request, got %v\n\n"+
			"SECURITY.md says this switch stops every outbound connection. An operator "+
			"turns it on to check that claim; if anything still gets out, the document "+
			"is wrong.", err)
	}
}

// TestARedirectToAnotherHostIsRefused.
//
// Private addresses are allowed, so a forge on the LAN can answer "go and ask
// this other address instead" — and that other address is chosen by whoever
// controls the forge, not by the operator.
func TestARedirectToAnotherHostIsRefused(t *testing.T) {
	elsewhere := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("somewhere else"))
	}))
	defer elsewhere.Close()

	forge := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, elsewhere.URL+"/", http.StatusFound)
	}))
	defer forge.Close()

	_, err := get(t, New(false, time.Second), forge.URL)
	if err == nil {
		t.Fatal("followed a redirect to a different host")
	}
	if !strings.Contains(err.Error(), "another host") {
		t.Errorf("refused for the wrong reason: %v", err)
	}
}

// TestARedirectOnTheSameHostIsFollowed keeps the rule above from being a blunt
// "no redirects", which would break forges that redirect /api to /api/.
func TestARedirectOnTheSameHostIsFollowed(t *testing.T) {
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/moved" {
			http.Redirect(w, r, srv.URL+"/here", http.StatusFound)
			return
		}
		_, _ = w.Write([]byte("arrived"))
	}))
	defer srv.Close()

	resp, err := get(t, New(false, time.Second), srv.URL+"/moved")
	if err != nil {
		t.Fatalf("a same-host redirect was refused: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "arrived" {
		t.Errorf("body is %q, want %q", body, "arrived")
	}
}

// TestTheBodyIsBounded.
//
// Every caller gets a bounded body without having to remember to bound it,
// which is the point of having a gate rather than a convention.
func TestTheBodyIsBounded(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		chunk := strings.Repeat("x", 1<<20)
		for i := 0; i < 12; i++ { // 12 MiB, over the 8 MiB cap
			if _, err := w.Write([]byte(chunk)); err != nil {
				return
			}
		}
	}))
	defer srv.Close()

	resp, err := get(t, New(false, 10*time.Second), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()

	n, err := io.Copy(io.Discard, resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if n != MaxResponseBytes {
		t.Errorf("read %d bytes, want the cap of %d", n, MaxResponseBytes)
	}
}

func TestOnlyHTTPGoesOut(t *testing.T) {
	c := New(false, time.Second)
	for _, addr := range []string{"file:///etc/passwd", "gopher://example.com/", "ftp://example.com/"} {
		if _, err := get(t, c, addr); err == nil {
			t.Errorf("%s was allowed", addr)
		}
	}
}
