// Package gitea speaks to Gitea and to Forgejo.
//
// One adapter for two projects, and not because compatibility is assumed: they
// are separate projects with separate release cycles. The surface used here —
// create an issue, read one, receive a delivery — has the same shape in both,
// so it is written once and tested against both. If they diverge, this file
// grows a branch rather than the interface growing a fifth member.
//
// The webhook payload was not read from documentation, because there is none:
// Forgejo's webhook page documents the push event and nothing else. It was read
// from the structs both projects serialise.
package gitea

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

// Client talks to one instance.
//
// BaseURL is the instance, not an API host: these are self-hosted, so it is
// routinely a private address with a self-signed certificate. Pointing at one
// is the feature, which is why the outbound gate allows private addresses.
type Client struct {
	BaseURL string
	Token   string
	Out     *outbound.Client
}

func (c Client) CreateIssue(ctx context.Context, repo forge.Repo, in forge.NewIssue) (forge.Issue, error) {
	payload := map[string]any{"title": in.Title, "body": in.Body}
	if len(in.Labels) > 0 {
		// Labels are ids here rather than names, so a label that has not been
		// created on the repository cannot be attached. Sending names would be
		// rejected as a type error, which is worse than not sending them.
		payload["labels"] = []int64{}
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
	return fmt.Sprintf("%s/api/v1/repos/%s/issues%s",
		strings.TrimSuffix(c.BaseURL, "/"), strings.Trim(string(repo), "/"), suffix)
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
	// "token <key>" is the form both accept, alongside Bearer.
	req.Header.Set("Authorization", "token "+c.Token)
	req.Header.Set("Accept", "application/json")
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
		return fmt.Errorf("the forge answered %d: %s", resp.StatusCode, strings.TrimSpace(string(detail)))
	}

	if out == nil {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
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

// Receiver verifies and translates Gitea's and Forgejo's deliveries.
type Receiver struct{}

// Verify checks the HMAC-SHA256 of the body.
//
// Forgejo sends X-Forgejo-Signature and Gitea sends X-Gitea-Signature, and
// Forgejo sends both — along with GitHub's and Gogs' event headers, which is
// precisely why the receiver is chosen from the connection record rather than
// sniffed from the request. A receiver that guessed the forge from headers
// would let the caller pick which verification it faced.
func (Receiver) Verify(headers http.Header, body []byte, secret string) error {
	given := headers.Get("X-Forgejo-Signature")
	if given == "" {
		given = headers.Get("X-Gitea-Signature")
	}
	if given == "" {
		return fmt.Errorf("no X-Forgejo-Signature or X-Gitea-Signature")
	}

	sent, err := hex.DecodeString(strings.TrimSpace(given))
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
// A third vocabulary for the same three things: labelling is "label_updated"
// and "label_cleared" here, "labeled"/"unlabeled" on GitHub, and not an action
// at all on GitLab.
func (Receiver) Parse(headers http.Header, body []byte) (forge.Event, error) {
	if !isIssueEvent(headers) {
		return forge.Event{Kind: forge.EventIgnored}, nil
	}

	var payload struct {
		Action string `json:"action"`
		Number int64  `json:"number"`
		Issue  issue  `json:"issue"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return forge.Event{}, fmt.Errorf("delivery: %w", err)
	}

	kind := forge.EventIgnored
	switch payload.Action {
	case "opened":
		kind = forge.EventOpened
	case "closed":
		kind = forge.EventClosed
	case "reopened":
		kind = forge.EventReopened
	case "label_updated", "label_cleared":
		kind = forge.EventLabelled
	}

	translated := payload.Issue.toForge()
	number := translated.Number
	if number == 0 {
		// The payload carries it at the top level too, and one of the two is
		// enough. Trusting only the nested one would drop events from a version
		// that populates the other.
		number = payload.Number
	}

	return forge.Event{
		Kind:       kind,
		Number:     number,
		State:      translated.State,
		Labels:     translated.Labels,
		DeliveryID: deliveryID(headers),
	}, nil
}

func isIssueEvent(headers http.Header) bool {
	for _, name := range []string{"X-Forgejo-Event", "X-Gitea-Event", "X-Gogs-Event", "X-GitHub-Event"} {
		if headers.Get(name) == "issues" {
			return true
		}
	}
	return false
}

func deliveryID(headers http.Header) string {
	for _, name := range []string{"X-Forgejo-Delivery", "X-Gitea-Delivery", "X-Gogs-Delivery", "X-GitHub-Delivery"} {
		if id := headers.Get(name); id != "" {
			return id
		}
	}
	return ""
}
