// Package notify tells the team something arrived.
//
// This is not a nicety. Without it the product fails by abandonment: nobody
// opens a moderation queue every day on the chance that it has something in it,
// so the queue fills, the portal fills with people talking to nobody, and that
// reads worse from outside than having no portal at all.
//
// Four channels and no more, chosen because they are what a self-hoster already
// runs. The shape is small on purpose — a config with a kind, a switch, one
// function per channel — because that is all four of them need.
package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"git.codigosh.com/CodigoSH/LastHelp/internal/platform/mail"
	"git.codigosh.com/CodigoSH/LastHelp/internal/platform/outbound"
)

// Kind is which channel.
type Kind string

const (
	None    Kind = ""
	Ntfy    Kind = "ntfy"
	Gotify  Kind = "gotify"
	Webhook Kind = "webhook"
	SMTP    Kind = "smtp"
)

// Config is one channel, as stored on a repository.
//
// Per repository rather than per server, because that costs nothing here and is
// what lets a group send one board to one room and another board somewhere
// else. One global channel is a channel everybody eventually silences.
type Config struct {
	Kind Kind `json:"kind"`

	// URL is the ntfy topic, the Gotify server, or the webhook endpoint.
	URL string `json:"url,omitempty"`

	// Token authenticates to ntfy or Gotify. Never logged.
	Token string `json:"token,omitempty"`

	// To is the address, for the mail channel.
	To string `json:"to,omitempty"`
}

// Active says whether anything would be sent.
func (c Config) Active() bool {
	switch c.Kind {
	case Ntfy, Gotify, Webhook:
		return c.URL != ""
	case SMTP:
		return c.To != ""
	}
	return false
}

// Parse reads a stored configuration. An unreadable one is treated as no
// channel rather than as an error: a bad JSON blob in a column must not stop a
// report from being accepted.
func Parse(raw string) Config {
	if strings.TrimSpace(raw) == "" {
		return Config{}
	}
	var c Config
	if err := json.Unmarshal([]byte(raw), &c); err != nil {
		return Config{}
	}
	return c
}

// Message is what gets sent. Short on purpose: this is a nudge to go and look,
// not a copy of the report. A notification that contains the whole submission
// is a notification that delivers spam to wherever the team reads it.
type Message struct {
	Title string
	Body  string
	URL   string
}

// Notifier holds the two ways out.
type Notifier struct {
	Out  *outbound.Client
	Mail mail.Sender
}

// Send delivers one message, or says why not.
func (n Notifier) Send(ctx context.Context, cfg Config, msg Message) error {
	if !cfg.Active() {
		return nil
	}
	switch cfg.Kind {
	case Ntfy:
		return n.sendNtfy(ctx, cfg, msg)
	case Gotify:
		return n.sendGotify(ctx, cfg, msg)
	case Webhook:
		return n.sendWebhook(ctx, cfg, msg)
	case SMTP:
		return n.Mail.Send(cfg.To, msg.Title, msg.Body+"\n\n"+msg.URL)
	}
	return fmt.Errorf("unknown notification channel %q", cfg.Kind)
}

// sendNtfy posts the body as plain text, with the title and a click-through in
// headers, which is ntfy's own shape.
func (n Notifier) sendNtfy(ctx context.Context, cfg Config, msg Message) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.URL, strings.NewReader(msg.Body))
	if err != nil {
		return err
	}
	req.Header.Set("Title", header(msg.Title))
	if msg.URL != "" {
		req.Header.Set("Click", msg.URL)
	}
	if cfg.Token != "" {
		req.Header.Set("Authorization", "Bearer "+cfg.Token)
	}
	return n.do(req)
}

func (n Notifier) sendGotify(ctx context.Context, cfg Config, msg Message) error {
	payload, err := json.Marshal(map[string]any{
		"title": msg.Title,
		"message": func() string {
			if msg.URL == "" {
				return msg.Body
			}
			return msg.Body + "\n" + msg.URL
		}(),
	})
	if err != nil {
		return err
	}

	endpoint := strings.TrimSuffix(cfg.URL, "/") + "/message"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if cfg.Token != "" {
		req.Header.Set("X-Gotify-Key", cfg.Token)
	}
	return n.do(req)
}

// sendWebhook posts a small, stable JSON object.
//
// Generic on purpose: it is what lets somebody point this at Matrix, at a chat
// bridge, at a script — whatever the team already reads. Adding a channel per
// service would be a channel per service to maintain.
func (n Notifier) sendWebhook(ctx context.Context, cfg Config, msg Message) error {
	payload, err := json.Marshal(map[string]any{
		"title": msg.Title,
		"body":  msg.Body,
		"url":   msg.URL,
	})
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.URL, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if cfg.Token != "" {
		req.Header.Set("Authorization", "Bearer "+cfg.Token)
	}
	return n.do(req)
}

func (n Notifier) do(req *http.Request) error {
	resp, err := n.Out.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 400 {
		// The body is not included: a misconfigured endpoint answering with a
		// page of HTML would otherwise end up in a log line, and a log that
		// swallows somebody else's error page is a log nobody reads.
		return fmt.Errorf("the notification channel answered %d", resp.StatusCode)
	}
	return nil
}

// header strips what would break out of a header line. The title comes from an
// item written by a stranger.
func header(value string) string {
	return strings.NewReplacer("\r", " ", "\n", " ").Replace(value)
}
