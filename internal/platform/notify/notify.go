// Package notify tells the team something arrived.
//
// This is not a nicety. Without it the product fails by abandonment: nobody
// opens a moderation queue every day on the chance that it has something in it,
// so the queue fills, the portal fills with people talking to nobody, and that
// reads worse from outside than having no portal at all.
//
// Five channels and no more, chosen because they are what a self-hoster already
// runs. The shape is small on purpose — a config with a kind, a switch, one
// function per channel — because that is all four of them need.
package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
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

	// Push is the browsers of the team: a notification on a phone or a desktop
	// with no page of LastHelp open. It needs nothing typed in — who receives it
	// is whoever on the team turned it on in their own browser — so it is a
	// complete choice by itself, like Server.
	Push Kind = "push"

	// Server is not a channel but a choice: "use the one the administrator set
	// for the whole installation". It is stored on a repository like any other
	// kind and resolved to the real one at the moment of sending, so changing
	// the server's channel moves every board that chose it, and a board that
	// never chose it is untouched.
	Server Kind = "server"
)

// Config is one channel, as stored on a repository, or as the server's own.
//
// Per repository first, because that costs nothing here and is what lets a
// group send one board to one room and another board somewhere else. A single
// channel for everything is one everybody eventually silences, so the server's
// is something a board chooses (Server), never something that applies by
// itself.
type Config struct {
	Kind Kind `json:"kind"`

	// URL is the ntfy topic, the Gotify server, or the webhook endpoint.
	URL string `json:"url,omitempty"`

	// Token authenticates to ntfy or Gotify. Never logged.
	Token string `json:"token,omitempty"`

	// To is the address, for the mail channel.
	To string `json:"to,omitempty"`
}

// Active says whether this is a complete choice: a channel with what it needs,
// or the server's, which is complete by itself and is resolved when sending.
func (c Config) Active() bool {
	switch c.Kind {
	case Ntfy, Gotify, Webhook:
		return c.URL != ""
	case SMTP:
		return c.To != ""
	case Server, Push:
		return true
	}
	return false
}

// StatusError is a channel that was reached and said no. The code is what lets a
// person be told something useful — a token refused is not an address that does
// not exist — without the body of somebody else's error page coming along.
type StatusError struct{ Code int }

func (e StatusError) Error() string {
	return fmt.Sprintf("the notification channel answered %d", e.Code)
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

	// Kind is what arrived, as a code and not as a word: "idea", "bug" or
	// "comment". A channel shared by a whole team has no one language to say it
	// in, so it says it with a mark every language reads — a bulb, a beetle, a
	// speech balloon — and a script reads the code itself. A channel that goes to
	// one person writes the word in that person's language instead.
	Kind string

	// Repo is the board this is about, and User is the one account a test is
	// for. Only a channel that goes to people rather than to an address reads
	// them: Push, which sends to the team of Repo, or to User alone.
	Repo int64
	User int64
}

// Glyph is the mark of a kind, and Tag the name ntfy gives the same mark. Both
// are empty for a message that is not about a report, a test for one.
func (m Message) Glyph() string {
	switch m.Kind {
	case "idea":
		return "💡"
	case "bug":
		return "🐞"
	case "comment":
		return "💬"
	}
	return ""
}

func (m Message) Tag() string {
	switch m.Kind {
	case "idea":
		return "bulb"
	case "bug":
		return "beetle"
	case "comment":
		return "speech_balloon"
	}
	return ""
}

// withGlyph puts the mark before a title, for a channel that has no place of its own for one.
func (m Message) withGlyph() string {
	if g := m.Glyph(); g != "" {
		return g + " " + m.Title
	}
	return m.Title
}

// Notifier holds the ways out.
type Notifier struct {
	Out  *outbound.Client
	Mail mail.Sender

	// Push delivers to browsers. It is a function because who may be told is
	// the server's to say, and this package does not know what a team is.
	Push func(ctx context.Context, msg Message) error
}

// Send delivers one message, or says why not.
func (n Notifier) Send(ctx context.Context, cfg Config, msg Message) error {
	if !cfg.Active() {
		return nil
	}
	if cfg.Kind == Server {
		// Resolved by the caller, who knows where the server's is kept. Reaching
		// here means one was not, and sending nothing silently would be a
		// channel that looks configured and never speaks.
		return errors.New("the server's channel was not resolved before sending")
	}
	switch cfg.Kind {
	case Ntfy:
		return n.sendNtfy(ctx, cfg, msg)
	case Gotify:
		return n.sendGotify(ctx, cfg, msg)
	case Webhook:
		return n.sendWebhook(ctx, cfg, msg)
	case SMTP:
		return n.Mail.Send(cfg.To, msg.withGlyph(), msg.Body+"\n\n"+msg.URL)
	case Push:
		if n.Push == nil {
			return errors.New("push notifications are not available")
		}
		return n.Push(ctx, msg)
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
	// The kind as ntfy's own emoji tag, which it draws beside the title: no word to translate.
	if tag := msg.Tag(); tag != "" {
		req.Header.Set("Tags", tag)
	}
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
		"title": msg.withGlyph(),
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
	fields := map[string]any{
		"title": msg.Title,
		"body":  msg.Body,
		"url":   msg.URL,
	}
	// What arrived, as a code a script can switch on without reading a language.
	if msg.Kind != "" {
		fields["kind"] = msg.Kind
	}
	payload, err := json.Marshal(fields)
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
		return StatusError{Code: resp.StatusCode}
	}
	return nil
}

// header strips what would break out of a header line. The title comes from an
// item written by a stranger.
func header(value string) string {
	return strings.NewReplacer("\r", " ", "\n", " ").Replace(value)
}
