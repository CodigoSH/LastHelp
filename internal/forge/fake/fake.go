// Package fake is a forge that only exists in tests.
//
// It answers the three requests the adapters make and, more importantly, signs
// its webhook deliveries the way each real forge signs them — with three
// different schemes, because that is the fact the whole receiver design rests
// on. A test double that signed all three the same way would happily accept a
// single generic verifier, which is exactly the bug it exists to catch.
//
// It is a verification tool: an architecture rule fails the build if anything
// outside a test imports it, because a test double compiled into a release is a
// forge that answers "yes" to everything.
package fake

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
)

// Forge is a stand-in instance.
type Forge struct {
	Server *httptest.Server

	mu      sync.Mutex
	issues  map[int64]map[string]any
	nextNum int64

	// Fail makes the next call answer with this status, for the paths that only
	// happen when somebody else's server is having a bad afternoon.
	Fail int
}

// New starts one. The caller closes it.
func New() *Forge {
	f := &Forge{issues: map[int64]map[string]any{}, nextNum: 1}
	f.Server = httptest.NewServer(http.HandlerFunc(f.handle))
	return f
}

func (f *Forge) Close() { f.Server.Close() }

// URL is the base to give an adapter.
func (f *Forge) URL() string { return f.Server.URL }

// Issues is how many exist, for a test that wants to assert nothing was created.
func (f *Forge) Issues() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.issues)
}

func (f *Forge) handle(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.Fail != 0 {
		status := f.Fail
		f.Fail = 0
		if status == http.StatusTooManyRequests {
			w.Header().Set("Retry-After", "30")
		}
		w.WriteHeader(status)
		return
	}

	// Every forge's create path ends in /issues and every read path ends in
	// /issues/<number>, whatever the prefix, which is enough to serve all three
	// from one handler without pretending to be a router.
	path := strings.TrimSuffix(r.URL.Path, "/")

	switch {
	case r.Method == http.MethodPost && strings.HasSuffix(path, "/issues"):
		var in map[string]any
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		number := f.nextNum
		f.nextNum++

		// Both spellings, so one fake serves adapters that read different
		// fields — which is also how a test would notice an adapter reading the
		// wrong one, because the values differ.
		title, _ := in["title"].(string)
		issue := map[string]any{
			"number":   number,
			"iid":      number,
			"id":       number + 1000, // deliberately not the iid
			"title":    title,
			"state":    "open",
			"html_url": fmt.Sprintf("%s/owner/repo/issues/%d", f.Server.URL, number),
			"web_url":  fmt.Sprintf("%s/owner/repo/-/issues/%d", f.Server.URL, number),
			"labels":   []any{},
		}
		f.issues[number] = issue
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(issue)

	case r.Method == http.MethodGet:
		parts := strings.Split(path, "/")
		var number int64
		_, err := fmt.Sscanf(parts[len(parts)-1], "%d", &number)
		issue, ok := f.issues[number]
		if err != nil || !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(issue)

	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

// Close an issue, as the forge would when somebody clicks the button.
func (f *Forge) CloseIssue(number int64) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if issue, ok := f.issues[number]; ok {
		issue["state"] = "closed"
	}
}

// --- deliveries, signed the way each real forge signs them ------------------

// GitHubDelivery builds headers and body as GitHub sends them: HMAC-SHA256 of
// the body, hex, prefixed "sha256=".
func GitHubDelivery(secret, action string, number int64, state string) (http.Header, []byte) {
	body, _ := json.Marshal(map[string]any{
		"action": action,
		"issue": map[string]any{
			"number": number, "state": state, "title": "an issue",
			"html_url": "https://github.com/owner/repo/issues/1",
			"labels":   []any{map[string]any{"name": "bug"}},
		},
	})

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)

	headers := http.Header{}
	headers.Set("X-GitHub-Event", "issues")
	headers.Set("X-GitHub-Delivery", "delivery-github-1")
	headers.Set("X-Hub-Signature-256", "sha256="+hex.EncodeToString(mac.Sum(nil)))
	return headers, body
}

// GitLabDelivery builds one the modern way: HMAC-SHA256 over
// "{id}.{timestamp}.{body}", base64, prefixed "v1,". The key arrives
// base64-encoded behind "whsec_".
func GitLabDelivery(secret, action string, iid int64, state string, labels, previous []string) (http.Header, []byte) {
	toLabels := func(names []string) []any {
		out := make([]any, 0, len(names))
		for _, n := range names {
			out = append(out, map[string]any{"title": n})
		}
		return out
	}

	body, _ := json.Marshal(map[string]any{
		"object_attributes": map[string]any{
			"iid": iid, "action": action, "state": state,
			"url": "https://gitlab.com/owner/repo/-/issues/1",
		},
		"labels": toLabels(labels),
		"changes": map[string]any{
			"labels": map[string]any{"previous": toLabels(previous), "current": toLabels(labels)},
		},
	})

	id, timestamp := "delivery-gitlab-1", "1700000000"
	key, _ := base64.StdEncoding.DecodeString(strings.TrimPrefix(secret, "whsec_"))
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(id + "." + timestamp + "." + string(body)))

	headers := http.Header{}
	headers.Set("X-Gitlab-Event", "Issue Hook")
	headers.Set("webhook-id", id)
	headers.Set("webhook-timestamp", timestamp)
	headers.Set("webhook-signature", "v1,"+base64.StdEncoding.EncodeToString(mac.Sum(nil)))
	return headers, body
}

// GiteaDelivery builds one as Gitea and Forgejo send them.
//
// It sets GitHub's headers as well as its own, which Forgejo really does — and
// which is the reason nothing here may decide what a delivery is by looking at
// its headers.
func GiteaDelivery(secret, action string, number int64, state string) (http.Header, []byte) {
	body, _ := json.Marshal(map[string]any{
		"action": action,
		"number": number,
		"issue": map[string]any{
			"number": number, "state": state, "title": "an issue",
			"html_url": "https://forge.example.com/owner/repo/issues/1",
			"labels":   []any{map[string]any{"name": "bug"}},
		},
	})

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)

	headers := http.Header{}
	headers.Set("X-Forgejo-Event", "issues")
	headers.Set("X-Gitea-Event", "issues")
	headers.Set("X-GitHub-Event", "issues") // yes, really
	headers.Set("X-Forgejo-Delivery", "delivery-forgejo-1")
	headers.Set("X-Forgejo-Signature", hex.EncodeToString(mac.Sum(nil)))
	return headers, body
}

// GitLabSecret builds a signing token in the shape GitLab hands out.
func GitLabSecret(raw string) string {
	return "whsec_" + base64.StdEncoding.EncodeToString([]byte(raw))
}
