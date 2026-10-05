package web

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"git.codigosh.com/CodigoSH/LastHelp/internal/core/authz"
	"git.codigosh.com/CodigoSH/LastHelp/internal/core/bridge"
	"git.codigosh.com/CodigoSH/LastHelp/internal/core/forge"
	"git.codigosh.com/CodigoSH/LastHelp/internal/core/org"
	"git.codigosh.com/CodigoSH/LastHelp/internal/core/settings"
	"git.codigosh.com/CodigoSH/LastHelp/internal/forge/gitea"
	"git.codigosh.com/CodigoSH/LastHelp/internal/forge/github"
	"git.codigosh.com/CodigoSH/LastHelp/internal/forge/gitlab"
	"git.codigosh.com/CodigoSH/LastHelp/internal/platform/mail"
	"git.codigosh.com/CodigoSH/LastHelp/internal/platform/notify"
	"git.codigosh.com/CodigoSH/LastHelp/internal/platform/outbound"
)

// webhookBodyLimit is what a delivery may be. Generous — an issue payload with
// a long body and many labels is real — and bounded, because the signature is
// over the whole body and reading an unbounded one to check it is a way to be
// handed a gigabyte.
const webhookBodyLimit = 1 << 20

// clientFor builds the adapter for an organisation, reading the token at the
// moment of use rather than carrying it around.
func (s *Server) clientFor(o org.Organization) (forge.Client, error) {
	token, _, err := s.orgs().Credentials(o.ID)
	if err != nil {
		return nil, err
	}

	switch o.Forge {
	case org.ForgeGitHub:
		return github.Client{BaseURL: o.BaseURL, Token: token, Out: s.out}, nil
	case org.ForgeGitLab:
		return gitlab.Client{BaseURL: o.BaseURL, Token: token, Out: s.out}, nil
	// Gitea and Forgejo share an adapter. Not because compatibility is
	// assumed — they are separate projects now — but because the surface used
	// here has the same shape in both, and both are driven against a real
	// server separately. If they diverge, this grows a branch rather than the
	// interface growing a fifth member.
	case org.ForgeGitea, org.ForgeForgejo:
		return gitea.Client{BaseURL: o.BaseURL, Token: token, Out: s.out}, nil
	default:
		return nil, bridge.ErrNoForge
	}
}

// receiverFor returns the verifier and translator for a forge.
//
// Chosen from the connection record. Never from the request: Forgejo sends
// GitHub's event and delivery headers alongside its own, so a server that
// decided by sniffing would let the caller choose which verification it faced —
// and GitLab's legacy path, a plain shared token, is weaker than the HMACs.
func receiverFor(kind org.Forge) (forge.Receiver, bool) {
	switch kind {
	case org.ForgeGitHub:
		return github.Receiver{}, true
	case org.ForgeGitLab:
		return gitlab.Receiver{}, true
	case org.ForgeGitea, org.ForgeForgejo:
		return gitea.Receiver{}, true
	}
	return nil, false
}

func (s *Server) bridge() bridge.Bridge {
	return bridge.Bridge{
		DB: s.db, Orgs: s.orgs(), Items: s.items(), Queue: s.outbox(),
		Clients: s.clientFor, Announce: s.announce, Tell: s.tellReporter,
		PublicURL: s.cfg.PublicURL,
	}
}

// handleWebhook receives a delivery.
//
// Public by necessity — a forge cannot sign in — and therefore the one endpoint
// where the signature is the whole of the authentication. Nothing in the body
// is read as meaning anything until it has been checked.
func (s *Server) handleWebhook(w http.ResponseWriter, r *http.Request) {
	organisation, err := s.orgs().ByHookID(r.PathValue("hook"))
	if err != nil {
		// Not "no such connection": an address that does not exist and one that
		// does but failed to verify answer identically, so this endpoint cannot
		// be used to find out which organisations exist.
		http.NotFound(w, r)
		return
	}

	receiver, ok := receiverFor(organisation.Forge)
	if !ok {
		http.NotFound(w, r)
		return
	}

	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, webhookBodyLimit))
	if err != nil {
		http.Error(w, "error.too_large", http.StatusRequestEntityTooLarge)
		return
	}

	_, secret, err := s.orgs().Credentials(organisation.ID)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	if err := receiver.Verify(r.Header, body, secret); err != nil {
		// Deliberately not logged with the headers. GitLab's legacy scheme puts
		// the shared secret in one, and a log that records headers is a log
		// that records secrets.
		slog.Warn("a delivery failed verification", "org", organisation.Slug)
		http.Error(w, "error.bad_signature", http.StatusUnauthorized)
		return
	}

	event, err := receiver.Parse(r.Header, body)
	if err != nil {
		slog.Warn("a verified delivery could not be read", "org", organisation.Slug, "error", err)
		w.WriteHeader(http.StatusAccepted)
		return
	}

	repos, err := s.orgs().ListRepositories(organisation.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "error.internal")
		return
	}

	// The delivery says which issue, not which of our repositories. Applying it
	// to each is safe: the link table is what decides, and an issue nothing is
	// linked to changes nothing anywhere.
	for _, repo := range repos {
		applied, err := s.bridge().Apply(organisation.ID, repo.ID, event, s.now())
		if err != nil {
			slog.Error("applying a delivery", "org", organisation.Slug, "error", err)
			writeError(w, http.StatusInternalServerError, "error.internal")
			return
		}
		if applied.Item != "" {
			slog.Info("a delivery reached an item", "item", applied.Item, "status", applied.Status)
			break
		}
	}

	// Accepted, whatever it meant. A forge that gets an error retries, and
	// retrying a delivery this server understood and decided to ignore is
	// noise for both sides.
	w.WriteHeader(http.StatusAccepted)
}

// handleCreateIssue queues an item to become an issue.
func (s *Server) handleCreateIssue(w http.ResponseWriter, r *http.Request) {
	it, _, ok := s.triageTarget(w, r)
	if !ok {
		return
	}
	if err := s.bridge().RequestIssue(it.ID, s.now()); err != nil {
		slog.Error("queueing an issue", "error", err)
		writeError(w, http.StatusInternalServerError, "error.internal")
		return
	}
	// Accepted, not created: the forge has not been called yet, and saying
	// otherwise would be reporting something that has not happened.
	writeJSON(w, http.StatusAccepted, map[string]any{"queued": true})
}

type attachRequest struct {
	// Either a number or the URL of the issue, because what somebody has in
	// their clipboard is the URL.
	Issue string `json:"issue"`
}

// handleAttachIssue links an item to an issue that already exists.
func (s *Server) handleAttachIssue(w http.ResponseWriter, r *http.Request) {
	it, _, ok := s.triageTarget(w, r)
	if !ok {
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, panelBodyLimit)
	var in attachRequest
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "error.bad_request")
		return
	}

	number, err := issueNumber(in.Issue)
	if err != nil {
		writeError(w, http.StatusBadRequest, "error.bad_issue")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()

	found, err := s.bridge().Attach(ctx, it.ID, number, s.now())
	if err != nil {
		switch {
		case errors.Is(err, forge.ErrNotFound):
			writeError(w, http.StatusNotFound, "error.issue_not_found")
		case errors.Is(err, bridge.ErrNoForge):
			writeError(w, http.StatusBadRequest, "error.no_forge")
		case errors.Is(err, forge.ErrDenied):
			writeError(w, http.StatusBadRequest, "error.forge_denied")
		default:
			slog.Error("attaching an issue", "error", err)
			writeError(w, http.StatusInternalServerError, "error.internal")
		}
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"number": found.Number, "url": found.URL})
}

// issueNumber accepts a bare number or the end of a URL, because a URL is what
// people actually have to hand.
func issueNumber(raw string) (int64, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return 0, errors.New("empty")
	}
	if index := strings.LastIndex(trimmed, "/"); index >= 0 {
		trimmed = trimmed[index+1:]
	}
	trimmed = strings.TrimPrefix(trimmed, "#")
	return strconv.ParseInt(trimmed, 10, 64)
}

type forgeCredentials struct {
	Token         string `json:"token"`
	WebhookSecret string `json:"webhook_secret"`
}

// handleSetCredentials stores an organisation's forge credentials.
//
// Owner-level, not collaborator: the whole reason the two levels exist is that
// somebody trusted to triage a repository is not automatically somebody trusted
// with a credential that can write to every repository the organisation owns.
func (s *Server) handleSetCredentials(w http.ResponseWriter, r *http.Request) {
	o, ok := s.lookupOrganization(w, r)
	if !ok {
		return
	}
	if !s.may(w, r, authz.ReadForgeCredentials, authz.Organization(o.ID)) {
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, panelBodyLimit)
	var in forgeCredentials
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "error.bad_request")
		return
	}

	if err := s.orgs().SetCredentials(o.ID, in.Token, in.WebhookSecret); err != nil {
		slog.Error("storing credentials", "error", err)
		writeError(w, http.StatusInternalServerError, "error.internal")
		return
	}

	hook, err := s.orgs().HookID(o.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "error.internal")
		return
	}

	// The address to configure on the forge, and nothing else. The token is
	// never echoed back: an endpoint that returns what it stored is a way to
	// read a credential through the panel.
	writeJSON(w, http.StatusOK, map[string]any{"webhook_path": "/hooks/" + hook})
}

// announce delivers a notification for one repository, on whatever channel that
// repository has configured.
//
// Per repository, so a group can send one board to one room and another
// somewhere else. The server's own channel is a choice a board makes in its
// settings, not something that applies to every board by itself: one channel
// for everything is one everybody eventually silences, and a silenced channel is
// the same as no channel with extra steps.
func (s *Server) announce(ctx context.Context, repoID int64, title, kind, link string) error {
	repo, err := s.orgs().RepositoryByID(repoID)
	if err != nil {
		return err
	}

	cfg := s.channelFor(repo.NotifyConfig)
	if !cfg.Active() {
		return nil
	}
	return s.notifier().Send(ctx, cfg, notify.Message{
		Title: repo.Name + ": " + kind,
		Body:  title,
		URL:   link,
	})
}

func (s *Server) notifier() notify.Notifier {
	return notify.Notifier{
		Out: s.out,
		Mail: mail.Sender{
			Config:  s.cfg.Mail,
			Offline: s.cfg.Offline,
		},
	}
}

// channelFor is the channel a message really goes out on: the board's own, or the
// server's when the board chose it. An unreadable or missing server channel
// resolves to nothing rather than to an error, for the same reason a bad JSON
// blob on a repository does: a report must be accepted whatever state the
// notifications are in.
func (s *Server) channelFor(stored string) notify.Config {
	cfg := notify.Parse(stored)
	if cfg.Kind != notify.Server {
		return cfg
	}
	return s.serverChannel()
}

// serverChannel is the channel set for the whole installation, or none.
func (s *Server) serverChannel() notify.Config {
	raw, err := s.settings().Get(settings.NotifyChannel)
	if err != nil {
		slog.Error("reading the server's notification channel", "error", err)
		return notify.Config{}
	}
	cfg := notify.Parse(raw)
	if cfg.Kind == notify.Server {
		// A server channel that points at itself is nothing, not a loop.
		return notify.Config{}
	}
	return cfg
}

// notifyChannel is what the channel form sends.
//
// A token is never sent back to the page, so an empty one means "leave it as it
// is", and removing it is its own explicit flag: the alternative is a form that
// erases a credential every time somebody corrects the address next to it.
type notifyChannel struct {
	Kind       string `json:"kind"`
	URL        string `json:"url"`
	Token      string `json:"token"`
	To         string `json:"to"`
	ClearToken bool   `json:"clear_token"`
}

// build turns what the form sent into the channel to store, given what is stored.
func (in notifyChannel) build(previous notify.Config, mayChooseServer bool) (notify.Config, string) {
	kind := notify.Kind(in.Kind)
	if kind == notify.Server && !mayChooseServer {
		return notify.Config{}, "error.channel_incomplete"
	}
	cfg := notify.Config{
		Kind: kind, URL: strings.TrimSpace(in.URL), Token: in.Token, To: strings.TrimSpace(in.To),
	}
	if kind == notify.Server {
		cfg.URL, cfg.Token, cfg.To = "", "", ""
		return cfg, ""
	}
	if cfg.Token == "" && !in.ClearToken && previous.Kind == kind {
		cfg.Token = previous.Token
	}
	if kind != "" && !cfg.Active() {
		return notify.Config{}, "error.channel_incomplete"
	}
	switch kind {
	case notify.Ntfy, notify.Gotify, notify.Webhook:
		// An address with no scheme is the common mistake, and the test message
		// would otherwise report it as the service being unreachable.
		if u, err := url.Parse(cfg.URL); err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return notify.Config{}, "error.channel_bad_url"
		}
	}
	return cfg, ""
}

// handleSetNotifyChannel stores where a board's team wants to be told.
//
// Owner-level, like the forge credentials and for the same reason: the channel
// carries a token, and somebody trusted to triage a repository is not
// automatically somebody trusted with a credential.
func (s *Server) handleSetNotifyChannel(w http.ResponseWriter, r *http.Request) {
	repo, organisation, ok := s.lookupRepository(w, r)
	if !ok {
		return
	}
	if !s.may(w, r, authz.ManageOrganization, authz.Organization(organisation.ID)) {
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, panelBodyLimit)
	var in notifyChannel
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "error.bad_request")
		return
	}

	cfg, failure := in.build(notify.Parse(repo.NotifyConfig), true)
	if failure != "" {
		writeError(w, http.StatusBadRequest, failure)
		return
	}

	encoded, err := json.Marshal(cfg)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "error.internal")
		return
	}
	if err := s.orgs().SetNotifyConfig(repo.ID, string(encoded)); err != nil {
		slog.Error("storing the notification channel", "error", err)
		writeError(w, http.StatusInternalServerError, "error.internal")
		return
	}

	// The token is not echoed back, for the same reason the forge token is not:
	// an endpoint that returns what it stored is a way to read a credential.
	writeJSON(w, http.StatusOK, map[string]any{"kind": in.Kind, "active": cfg.Active()})
}

// handleTestNotifyChannel sends one message down the channel as the form has it,
// before anything is saved, so a mistake shows up now and not at the first
// report. A token left empty is the stored one, which the form cannot show.
func (s *Server) handleTestNotifyChannel(w http.ResponseWriter, r *http.Request) {
	repo, organisation, ok := s.lookupRepository(w, r)
	if !ok {
		return
	}
	if !s.may(w, r, authz.ManageOrganization, authz.Organization(organisation.ID)) {
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, panelBodyLimit)
	var in notifyChannel
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "error.bad_request")
		return
	}
	cfg, failure := in.build(notify.Parse(repo.NotifyConfig), true)
	if failure != "" || cfg.Kind == notify.None {
		if failure == "" {
			failure = "error.channel_incomplete"
		}
		writeError(w, http.StatusBadRequest, failure)
		return
	}
	if cfg.Kind == notify.Server {
		cfg = s.serverChannel()
		if !cfg.Active() {
			writeError(w, http.StatusUnprocessableEntity, "error.channel_server_unset")
			return
		}
	}
	s.finishChannelTest(w, r.Context(), cfg, repo.Name)
}

// finishChannelTest sends the test message and answers with the reason it did not
// arrive, in a form a person can act on.
func (s *Server) finishChannelTest(w http.ResponseWriter, ctx context.Context, cfg notify.Config, name string) {
	link := ""
	if base := strings.TrimSuffix(s.cfg.PublicURL, "/"); base != "" {
		link = base
	}
	// A short deadline of its own: the person is waiting on the button, and a
	// service that does not answer is the answer.
	ctx, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()

	err := s.notifier().Send(ctx, cfg, notify.Message{
		Title: "LastHelp: test",
		Body:  "This is a test from " + name + ". If you can read it, this channel works.",
		URL:   link,
	})
	if err == nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
		return
	}
	key := channelFailure(err, cfg.Kind)
	// The reason class only, never the error text: it carries the address, and an
	// address is half of a credential for a service that is addressed by a topic.
	slog.Warn("a notification channel test failed", "kind", string(cfg.Kind), "reason", key)
	writeError(w, http.StatusUnprocessableEntity, key)
}

// channelFailure names why a message did not go, as a key a person can read.
func channelFailure(err error, kind notify.Kind) string {
	var refused notify.StatusError
	switch {
	case errors.Is(err, mail.ErrNotConfigured):
		return "error.channel_no_relay"
	case errors.Is(err, mail.ErrOffline), errors.Is(err, outbound.ErrOffline):
		return "error.channel_offline"
	case errors.As(err, &refused):
		switch refused.Code {
		case http.StatusUnauthorized, http.StatusForbidden:
			return "error.channel_denied"
		case http.StatusNotFound:
			return "error.channel_not_found"
		}
		return "error.channel_rejected"
	}
	return "error.channel_unreachable"
}

// tellReporter delivers to the person who reported, opening the address only
// for as long as it takes to send.
//
// The reason is a translation key, and it is looked up in the catalogue the
// server ships rather than composed here — but the reporter's language is not
// known, so it goes out in English. That is a real limitation, written down
// rather than hidden: the honest fix is asking for a language alongside the
// address, and that costs a field on a form whose whole point is being short.
func (s *Server) tellReporter(_ context.Context, address, title, reason, link string) error {
	if s.cfg.Mail.From == "" {
		return nil
	}

	sender := mail.Sender{Config: s.cfg.Mail, Offline: s.cfg.Offline}
	body := "Your report \"" + title + "\" " + reason + ".\n"
	if link != "" {
		body += "\n" + link + "\n"
	}
	body += "\nNobody reads replies to this address."

	return sender.Send(address, "LastHelp: "+title, body)
}

// mailConfigured says whether the notify-me field should be offered at all.
//
// With no relay there is nothing to notify anybody with, so the form does not
// ask for an address — rather than collecting one it cannot use, which is
// collecting personal data for nothing.
func (s *Server) mailConfigured() bool { return s.cfg.Mail.Host != "" && s.cfg.Mail.From != "" }
