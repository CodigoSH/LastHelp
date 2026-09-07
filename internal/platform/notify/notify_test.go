package notify

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"git.codigosh.com/CodigoSH/LastHelp/internal/platform/mail"
	"git.codigosh.com/CodigoSH/LastHelp/internal/platform/outbound"
)

type captured struct {
	path    string
	headers http.Header
	body    string
}

func recorder(t *testing.T) (*httptest.Server, *captured) {
	t.Helper()
	seen := &captured{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		seen.path = r.URL.Path
		seen.headers = r.Header.Clone()
		seen.body = string(body)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)
	return server, seen
}

func notifier() Notifier {
	return Notifier{Out: outbound.New(false, 5*time.Second)}
}

func TestNtfyGetsTheTitleInAHeaderAndTheBodyAsText(t *testing.T) {
	server, seen := recorder(t)

	err := notifier().Send(context.Background(),
		Config{Kind: Ntfy, URL: server.URL + "/lasthelp", Token: "tk_secret"},
		Message{Title: "A new report", Body: "crash on import", URL: "https://example/i/abc"})
	if err != nil {
		t.Fatal(err)
	}

	if seen.body != "crash on import" {
		t.Errorf("body is %q", seen.body)
	}
	if got := seen.headers.Get("Title"); got != "A new report" {
		t.Errorf("title header is %q", got)
	}
	if got := seen.headers.Get("Click"); got == "" {
		t.Error("no click-through, so the notification cannot be acted on from the phone")
	}
	if got := seen.headers.Get("Authorization"); got != "Bearer tk_secret" {
		t.Errorf("authorization is %q", got)
	}
}

func TestGotifyPostsToTheMessageEndpoint(t *testing.T) {
	server, seen := recorder(t)

	err := notifier().Send(context.Background(),
		Config{Kind: Gotify, URL: server.URL + "/", Token: "app-token"},
		Message{Title: "A new report", Body: "crash", URL: "https://example/i/abc"})
	if err != nil {
		t.Fatal(err)
	}

	if seen.path != "/message" {
		t.Errorf("posted to %q, want /message", seen.path)
	}
	if got := seen.headers.Get("X-Gotify-Key"); got != "app-token" {
		t.Errorf("key header is %q", got)
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(seen.body), &payload); err != nil {
		t.Fatal(err)
	}
	if payload["title"] != "A new report" {
		t.Errorf("payload is %v", payload)
	}
	if text, _ := payload["message"].(string); !strings.Contains(text, "https://example/i/abc") {
		t.Error("the link did not travel with the message")
	}
}

// TestTheWebhookShapeIsSmallAndStable.
//
// It is what lets somebody point this at Matrix, at a bridge, at a script. A
// shape that changes is a shape that breaks whatever they wrote.
func TestTheWebhookShapeIsSmallAndStable(t *testing.T) {
	server, seen := recorder(t)

	err := notifier().Send(context.Background(),
		Config{Kind: Webhook, URL: server.URL + "/hook"},
		Message{Title: "A new report", Body: "crash", URL: "https://example/i/abc"})
	if err != nil {
		t.Fatal(err)
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(seen.body), &payload); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"title", "body", "url"} {
		if _, ok := payload[key]; !ok {
			t.Errorf("the payload has no %q", key)
		}
	}
	if len(payload) != 3 {
		t.Errorf("the payload has %d fields: %v — this shape is somebody's script", len(payload), payload)
	}
}

// TestNothingIsSentWhenNothingIsConfigured.
//
// The common case. A repository with no channel must not error on every
// submission, and must not be a reason to refuse one.
func TestNothingIsSentWhenNothingIsConfigured(t *testing.T) {
	for _, cfg := range []Config{
		{},
		{Kind: Ntfy},             // no URL
		{Kind: SMTP},             // no address
		{Kind: Webhook, URL: ""}, // explicitly empty
	} {
		if cfg.Active() {
			t.Errorf("%+v is considered active", cfg)
		}
		if err := notifier().Send(context.Background(), cfg, Message{Title: "x"}); err != nil {
			t.Errorf("%+v gave %v", cfg, err)
		}
	}
}

// TestAnUnreadableConfigIsNoChannel.
//
// A bad JSON blob in a column must not stop a report from being accepted: the
// person submitting has nothing to do with the operator's notification setup.
func TestAnUnreadableConfigIsNoChannel(t *testing.T) {
	for _, raw := range []string{"", "   ", "not json", `{"kind":`, `[]`} {
		if Parse(raw).Active() {
			t.Errorf("%q parsed into an active channel", raw)
		}
	}

	parsed := Parse(`{"kind":"ntfy","url":"https://ntfy.example/topic"}`)
	if !parsed.Active() || parsed.Kind != Ntfy {
		t.Errorf("a good config parsed to %+v", parsed)
	}
}

// TestAFailingChannelSaysSoWithoutQuotingIt.
//
// A misconfigured endpoint often answers with a page of HTML. Putting that in
// the error puts it in the log, and a log full of somebody else's error pages
// is a log nobody reads.
func TestAFailingChannelSaysSoWithoutQuotingIt(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte("<html><body>a whole error page</body></html>"))
	}))
	defer server.Close()

	err := notifier().Send(context.Background(),
		Config{Kind: Webhook, URL: server.URL}, Message{Title: "x"})
	if err == nil {
		t.Fatal("a 502 was treated as success")
	}
	if strings.Contains(err.Error(), "html") {
		t.Errorf("the error quotes the response body: %v", err)
	}
}

// TestATitleCannotBreakOutOfAHeader.
//
// The title is an item title, written by a stranger. A newline in an ntfy Title
// header would end the header and start another.
func TestATitleCannotBreakOutOfAHeader(t *testing.T) {
	server, seen := recorder(t)

	err := notifier().Send(context.Background(),
		Config{Kind: Ntfy, URL: server.URL},
		Message{Title: "innocent\r\nPriority: max\r\nTags: skull", Body: "b"})
	if err != nil {
		t.Fatal(err)
	}
	if got := seen.headers.Get("Priority"); got != "" {
		t.Errorf("a title injected a Priority header: %q", got)
	}
	if strings.ContainsAny(seen.headers.Get("Title"), "\r\n") {
		t.Error("the title header still contains a line break")
	}
}

// TestTheOfflineSwitchStopsNotificationsToo.
//
// SECURITY.md says the switch stops every outbound connection. Notifications
// are outbound connections.
func TestTheOfflineSwitchStopsNotificationsToo(t *testing.T) {
	server, seen := recorder(t)

	offline := Notifier{
		Out:  outbound.New(true, time.Second),
		Mail: mail.Sender{Offline: true, Config: mail.Config{Host: "localhost", From: "a@b"}},
	}

	if err := offline.Send(context.Background(),
		Config{Kind: Webhook, URL: server.URL}, Message{Title: "x"}); err == nil {
		t.Error("a webhook notification got out while offline")
	}
	if err := offline.Send(context.Background(),
		Config{Kind: SMTP, To: "somebody@example.com"}, Message{Title: "x"}); err == nil {
		t.Error("a mail notification got out while offline")
	}
	if seen.path != "" {
		t.Error("something reached the server while offline")
	}
}
