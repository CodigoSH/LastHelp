package web

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"git.codigosh.com/CodigoSH/LastHelp/internal/core/authz"
	"git.codigosh.com/CodigoSH/LastHelp/internal/core/settings"
	"git.codigosh.com/CodigoSH/LastHelp/internal/platform/notify"
)

// The channel for the whole installation.
//
// It is the administrator's and nobody else's: it carries a credential for a
// service that reaches every board that chooses it. A repository does not
// inherit it; it picks "the server's" in its own settings, which is what keeps
// a board that wants its own room from being talked over.

func (s *Server) handleGetServerChannel(w http.ResponseWriter, r *http.Request) {
	if !s.may(w, r, authz.ManageServer, authz.Scope{}) {
		return
	}
	cfg := s.serverChannel()
	body := channelDetails(string(mustJSON(cfg)))
	body["kind"] = string(cfg.Kind)
	writeJSON(w, http.StatusOK, body)
}

func (s *Server) handleSetServerChannel(w http.ResponseWriter, r *http.Request) {
	if !s.may(w, r, authz.ManageServer, authz.Scope{}) {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, panelBodyLimit)
	var in notifyChannel
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "error.bad_request")
		return
	}
	// The server's channel cannot be "the server's": that is a board's choice.
	cfg, failure := in.build(s.serverChannel(), false)
	if failure != "" {
		writeError(w, http.StatusBadRequest, failure)
		return
	}
	if err := s.settings().Set(settings.NotifyChannel, string(mustJSON(cfg))); err != nil {
		slog.Error("storing the server's notification channel", "error", err)
		writeError(w, http.StatusInternalServerError, "error.internal")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"kind": in.Kind, "active": cfg.Active()})
}

func (s *Server) handleTestServerChannel(w http.ResponseWriter, r *http.Request) {
	if !s.may(w, r, authz.ManageServer, authz.Scope{}) {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, panelBodyLimit)
	var in notifyChannel
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "error.bad_request")
		return
	}
	cfg, failure := in.build(s.serverChannel(), false)
	if failure != "" || cfg.Kind == notify.None {
		if failure == "" {
			failure = "error.channel_incomplete"
		}
		writeError(w, http.StatusBadRequest, failure)
		return
	}
	s.finishChannelTest(w, r.Context(), cfg, "this server")
}

// mustJSON is a Config as the JSON it is stored as. Marshalling a struct of
// strings does not fail, and a function that returned an error nobody can act
// on would only be a place to forget to check it.
func mustJSON(cfg notify.Config) []byte {
	encoded, err := json.Marshal(cfg)
	if err != nil {
		return []byte("{}")
	}
	return encoded
}
