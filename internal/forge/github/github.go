// Package github speaks to GitHub, and to GitHub Enterprise Server.
package github

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"git.codigosh.com/CodigoSH/LastHelp/internal/core/forge"
	"git.codigosh.com/CodigoSH/LastHelp/internal/platform/outbound"
)

// Client talks to one owner's repositories.
//
// A fine-grained token is bound to a single owner and cannot span two
// organisations, which is why a connection is per organisation rather than per
// server. BaseURL exists for GitHub Enterprise Server, which is the same API at
// a different address.
type Client struct {
	BaseURL string // "https://api.github.com", or an Enterprise host
	Token   string
	Out     *outbound.Client
}

const apiVersion = "2022-11-28"

func (c Client) CreateIssue(ctx context.Context, repo forge.Repo, in forge.NewIssue) (forge.Issue, error) {
	// GitHub calls it "body" and takes labels as an array. GitLab calls the
	// same field "description" and takes a comma-separated string. This is the
	// whole reason adapters exist.
	payload := map[string]any{"title": in.Title, "body": in.Body}
	if len(in.Labels) > 0 {
		payload["labels"] = in.Labels
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

func (c Client) path(repo forge.Repo, suffix string) string {
	base := strings.TrimSuffix(c.BaseURL, "/")
	if base == "" {
		base = "https://api.github.com"
	}
	return fmt.Sprintf("%s/repos/%s/issues%s", base, strings.Trim(string(repo), "/"), suffix)
}

func (c Client) call(ctx context.Context, method, url string, body any, out any) error {
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(encoded)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", apiVersion)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.Out.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if err := statusError(resp); err != nil {
		return err
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func statusError(resp *http.Response) error {
	switch {
	case resp.StatusCode == http.StatusNotFound:
		return forge.ErrNotFound
	case resp.StatusCode == http.StatusUnauthorized, resp.StatusCode == http.StatusForbidden:
		// 403 is both "you may not" and, on GitHub, secondary rate limiting.
		// The retry header is what tells them apart, and the difference
		// matters: one is retried, the other needs a person.
		if resp.Header.Get("Retry-After") != "" || resp.Header.Get("X-RateLimit-Remaining") == "0" {
			return forge.ErrRateLimited
		}
		return forge.ErrDenied
	case resp.StatusCode == http.StatusTooManyRequests:
		return forge.ErrRateLimited
	case resp.StatusCode >= 400:
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return fmt.Errorf("github answered %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return nil
}

type issue struct {
	Number  int64  `json:"number"`
	HTMLURL string `json:"html_url"`
	Title   string `json:"title"`
	State   string `json:"state"`
	Labels  []struct {
		Name string `json:"name"`
	} `json:"labels"`
}

func (i issue) toForge() forge.Issue {
	labels := make([]string, 0, len(i.Labels))
	for _, l := range i.Labels {
		labels = append(labels, l.Name)
	}
	return forge.Issue{
		Number: i.Number,
		URL:    i.HTMLURL,
		Title:  i.Title,
		State:  state(i.State),
		Labels: labels,
	}
}

func state(raw string) forge.State {
	if raw == "closed" {
		return forge.Closed
	}
	return forge.Open
}

// Receiver verifies and translates GitHub's deliveries.
type Receiver struct{}

// Verify checks X-Hub-Signature-256, which is an HMAC-SHA256 of the raw body
// prefixed with "sha256=". Compared in constant time, as GitHub's own
// documentation insists: "never use a plain == operator".
func (Receiver) Verify(headers http.Header, body []byte, secret string) error {
	given := headers.Get("X-Hub-Signature-256")
	if given == "" {
		return fmt.Errorf("no X-Hub-Signature-256")
	}
	hexDigest, ok := strings.CutPrefix(given, "sha256=")
	if !ok {
		return fmt.Errorf("signature is not sha256=")
	}
	sent, err := hex.DecodeString(hexDigest)
	if err != nil {
		return fmt.Errorf("signature is not hex")
	}

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	if !hmac.Equal(sent, mac.Sum(nil)) {
		return fmt.Errorf("signature does not match")
	}
	return nil
}

// Parse translates a verified delivery.
//
// GitHub has an action for labelling, which GitLab does not — there, the same
// thing arrives as "update" and has to be worked out from a diff. Mapping
// actions one-to-one across forges is how a whole class of event goes missing.
func (Receiver) Parse(headers http.Header, body []byte) (forge.Event, error) {
	if event := headers.Get("X-GitHub-Event"); event != "issues" {
		return forge.Event{Kind: forge.EventIgnored}, nil
	}

	var payload struct {
		Action string `json:"action"`
		Issue  issue  `json:"issue"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return forge.Event{}, fmt.Errorf("github delivery: %w", err)
	}

	kind := forge.EventIgnored
	switch payload.Action {
	case "opened":
		kind = forge.EventOpened
	case "closed":
		kind = forge.EventClosed
	case "reopened":
		kind = forge.EventReopened
	case "labeled", "unlabeled":
		kind = forge.EventLabelled
	}

	translated := payload.Issue.toForge()
	return forge.Event{
		Kind:       kind,
		Number:     translated.Number,
		State:      translated.State,
		Labels:     translated.Labels,
		DeliveryID: headers.Get("X-GitHub-Delivery"),
	}, nil
}
