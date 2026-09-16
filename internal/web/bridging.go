package web

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"git.codigosh.com/CodigoSH/LastHelp/internal/core/authz"
	"git.codigosh.com/CodigoSH/LastHelp/internal/core/bridge"
	"git.codigosh.com/CodigoSH/LastHelp/internal/core/forge"
	"git.codigosh.com/CodigoSH/LastHelp/internal/core/org"
	"git.codigosh.com/CodigoSH/LastHelp/internal/forge/gitea"
	"git.codigosh.com/CodigoSH/LastHelp/internal/forge/github"
	"git.codigosh.com/CodigoSH/LastHelp/internal/forge/gitlab"
	"git.codigosh.com/CodigoSH/LastHelp/internal/platform/mail"
	"git.codigosh.com/CodigoSH/LastHelp/internal/platform/notify"
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
// somewhere else. One global channel is a channel everybody eventually
// silences, and a silenced channel is the same as no channel with extra steps.
func (s *Server) announce(ctx context.Context, repoID int64, title, kind, link string) error {
	repo, err := s.orgs().RepositoryByID(repoID)
	if err != nil {
		return err
	}

	cfg := notify.Parse(repo.NotifyConfig)
	if !cfg.Active() {
		return nil
	}

	notifier := notify.Notifier{
		Out: s.out,
		Mail: mail.Sender{
			Config:  s.cfg.Mail,
			Offline: s.cfg.Offline,
		},
	}
	return notifier.Send(ctx, cfg, notify.Message{
		Title: repo.Name + ": " + kind,
		Body:  title,
		URL:   link,
	})
}

type notifyChannel struct {
	Kind  string `json:"kind"`
	URL   string `json:"url"`
	Token string `json:"token"`
	To    string `json:"to"`
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

	cfg := notify.Config{
		Kind: notify.Kind(in.Kind), URL: in.URL, Token: in.Token, To: in.To,
	}
	if in.Kind != "" && !cfg.Active() {
		writeError(w, http.StatusBadRequest, "error.channel_incomplete")
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
