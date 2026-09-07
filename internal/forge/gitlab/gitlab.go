// Package gitlab speaks to GitLab, hosted or self-managed.
package gitlab

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"git.codigosh.com/CodigoSH/LastHelp/internal/core/forge"
	"git.codigosh.com/CodigoSH/LastHelp/internal/platform/outbound"
)

// Client talks to one group's projects.
type Client struct {
	BaseURL string // "https://gitlab.com", or a self-managed host
	Token   string
	Out     *outbound.Client
}

func (c Client) CreateIssue(ctx context.Context, repo forge.Repo, in forge.NewIssue) (forge.Issue, error) {
	// "description", not "body"; labels comma-separated, not an array. The two
	// differences that cost the most if an adapter is written from memory.
	payload := map[string]any{"title": in.Title, "description": in.Body}
	if len(in.Labels) > 0 {
		payload["labels"] = strings.Join(in.Labels, ",")
	}

	var out issue
	if err := c.call(ctx, http.MethodPost, c.path(repo, ""), payload, &out); err != nil {
		return forge.Issue{}, err
	}
	return out.toForge(), nil
}

func (c Client) FindIssue(ctx context.Context, repo forge.Repo, number int64) (forge.Issue, error) {
	var out issue
	if err := c.call(ctx, http.MethodGet, c.path(repo, fmt.Sprint("/", number)), nil, &out); err != nil {
		return forge.Issue{}, err
	}
	return out.toForge(), nil
}

// path URL-encodes the project path, slashes included.
//
// GitLab accepts either a numeric id or the full path with every "/" escaped —
// "CodigoSH/Lastboard" becomes "CodigoSH%2FLastboard". Leaving them unescaped
// produces a 404 that looks exactly like a missing repository.
func (c Client) path(repo forge.Repo, suffix string) string {
	base := strings.TrimSuffix(c.BaseURL, "/")
	if base == "" {
		base = "https://gitlab.com"
	}
	return fmt.Sprintf("%s/api/v4/projects/%s/issues%s", base, url.PathEscape(string(repo)), suffix)
}

func (c Client) call(ctx context.Context, method, endpoint string, body any, out any) error {
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(encoded)
	}

	req, err := http.NewRequestWithContext(ctx, method, endpoint, reader)
	if err != nil {
		return err
	}
	req.Header.Set("PRIVATE-TOKEN", c.Token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.Out.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	switch {
	case resp.StatusCode == http.StatusNotFound:
		return forge.ErrNotFound
	case resp.StatusCode == http.StatusUnauthorized, resp.StatusCode == http.StatusForbidden:
		return forge.ErrDenied
	case resp.StatusCode == http.StatusTooManyRequests:
		return forge.ErrRateLimited
	case resp.StatusCode >= 400:
		detail, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return fmt.Errorf("gitlab answered %d: %s", resp.StatusCode, strings.TrimSpace(string(detail)))
	}

	if out == nil {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

type issue struct {
	// IID, not ID. GitLab sends both: iid is the number shown in the interface
	// and the one a URL is built from; id is global to the instance. Using id
	// is the classic mistake and produces links that go to somebody else's
	// issue or nowhere.
	IID    int64    `json:"iid"`
	WebURL string   `json:"web_url"`
	Title  string   `json:"title"`
	State  string   `json:"state"`
	Labels []string `json:"labels"`
}

func (i issue) toForge() forge.Issue {
	return forge.Issue{
		Number: i.IID,
		URL:    i.WebURL,
		Title:  i.Title,
		State:  state(i.State),
		Labels: i.Labels,
	}
}

// state normalises GitLab's vocabulary. It says "opened" where the others say
// "open" — a one-word difference that belongs here rather than in the domain.
func state(raw string) forge.State {
	if raw == "closed" {
		return forge.Closed
	}
	return forge.Open
}

// Receiver verifies and translates GitLab's deliveries.
type Receiver struct{}

// Verify accepts either of GitLab's two schemes.
//
// The modern one signs a composed message — id, timestamp and body — with a key
// that arrives base64-encoded behind a "whsec_" prefix. The legacy one sends the
// shared secret in the clear in X-Gitlab-Token.
//
// A generic verifier that hashed the body would reject the first and accept
// nothing, which is why there is no generic verifier anywhere in this codebase.
// And because the legacy scheme puts a secret in a header, request headers are
// never logged.
func (Receiver) Verify(headers http.Header, body []byte, secret string) error {
	if signature := headers.Get("webhook-signature"); signature != "" {
		return verifySigned(headers, body, secret, signature)
	}

	given := headers.Get("X-Gitlab-Token")
	if given == "" {
		return fmt.Errorf("no webhook-signature and no X-Gitlab-Token")
	}
	if subtle.ConstantTimeCompare([]byte(given), []byte(secret)) != 1 {
		return fmt.Errorf("token does not match")
	}
	return nil
}

func verifySigned(headers http.Header, body []byte, secret, signature string) error {
	version, encoded, ok := strings.Cut(signature, ",")
	if !ok || version != "v1" {
		return fmt.Errorf("unknown signature version %q", version)
	}
	sent, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return fmt.Errorf("signature is not base64")
	}

	key, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(secret, "whsec_"))
	if err != nil {
		return fmt.Errorf("the signing token is not base64 after its prefix")
	}

	id := headers.Get("webhook-id")
	timestamp := headers.Get("webhook-timestamp")
	if id == "" || timestamp == "" {
		return fmt.Errorf("a signed delivery with no webhook-id or webhook-timestamp")
	}

	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(id + "." + timestamp + "." + string(body)))
	if !hmac.Equal(sent, mac.Sum(nil)) {
		return fmt.Errorf("signature does not match")
	}
	return nil
}

// Parse translates a verified delivery.
//
// GitLab has no labelling action. Relabelling arrives as "update" with the old
// and new label lists in `changes`, so anybody mapping actions one-to-one
// across the four forges loses every label change GitLab ever sends — silently,
// because nothing errors.
func (Receiver) Parse(headers http.Header, body []byte) (forge.Event, error) {
	if headers.Get("X-Gitlab-Event") != "Issue Hook" {
		return forge.Event{Kind: forge.EventIgnored}, nil
	}

	var payload struct {
		ObjectAttributes struct {
			IID    int64  `json:"iid"`
			Action string `json:"action"`
			State  string `json:"state"`
			URL    string `json:"url"`
		} `json:"object_attributes"`
		Labels []struct {
			Title string `json:"title"`
		} `json:"labels"`
		Changes struct {
			Labels struct {
				Previous []struct {
					Title string `json:"title"`
				} `json:"previous"`
				Current []struct {
					Title string `json:"title"`
				} `json:"current"`
			} `json:"labels"`
		} `json:"changes"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return forge.Event{}, fmt.Errorf("gitlab delivery: %w", err)
	}

	labels := make([]string, 0, len(payload.Labels))
	for _, l := range payload.Labels {
		labels = append(labels, l.Title)
	}

	kind := forge.EventIgnored
	switch payload.ObjectAttributes.Action {
	case "open":
		kind = forge.EventOpened
	case "close":
		kind = forge.EventClosed
	case "reopen":
		kind = forge.EventReopened
	case "update":
		// The only way to know a label changed: compare the two lists.
		if labelsDiffer(payload.Changes.Labels.Previous, payload.Changes.Labels.Current) {
			kind = forge.EventLabelled
		}
	}

	return forge.Event{
		Kind:       kind,
		Number:     payload.ObjectAttributes.IID,
		State:      state(payload.ObjectAttributes.State),
		Labels:     labels,
		DeliveryID: headers.Get("webhook-id"),
	}, nil
}

func labelsDiffer(before, after []struct {
	Title string `json:"title"`
}) bool {
	if len(before) != len(after) {
		return true
	}
	seen := map[string]int{}
	for _, l := range before {
		seen[l.Title]++
	}
	for _, l := range after {
		seen[l.Title]--
	}
	for _, count := range seen {
		if count != 0 {
			return true
		}
	}
	return false
}
