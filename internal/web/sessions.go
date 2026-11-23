package web

import (
	"errors"
	"log/slog"
	"net/http"

	"git.codigosh.com/CodigoSH/LastHelp/internal/core/session"
	"git.codigosh.com/CodigoSH/LastHelp/internal/web/middleware"
)

// deviceOf is the browser and the address a request came from, as the list of
// sessions shows them. The address is worked out the way the rate limiter
// works it out, so behind a proxy it is the visitor's and not the proxy's.
func (s *Server) deviceOf(r *http.Request) session.Device {
	address, _ := s.addressOf(r)
	return session.Device{UserAgent: r.UserAgent(), IP: address}
}

// sessionRow is one session as its owner's list shows it.
type sessionRow struct {
	session.Listed
	Current bool `json:"current"`
}

// handleListSessions lists where this account is signed in, and which of them
// is the browser asking.
func (s *Server) handleListSessions(w http.ResponseWriter, r *http.Request) {
	actor, _ := middleware.ActorFrom(r)
	listed, err := s.sessions().List(actor.UserID, s.now())
	if err != nil {
		slog.Error("listing sessions", "error", err)
		writeError(w, http.StatusInternalServerError, "error.internal")
		return
	}
	current := middleware.SessionFrom(r)
	out := make([]sessionRow, 0, len(listed))
	for _, l := range listed {
		out = append(out, sessionRow{Listed: l, Current: l.ID == current})
	}
	writeJSON(w, http.StatusOK, map[string]any{"sessions": out})
}

// handleCloseSession signs one of the account's other browsers out. The one
// asking is not closed here: that is signing out, which also clears the cookie.
func (s *Server) handleCloseSession(w http.ResponseWriter, r *http.Request) {
	actor, _ := middleware.ActorFrom(r)
	id := r.PathValue("id")
	if id == middleware.SessionFrom(r) {
		writeError(w, http.StatusBadRequest, "error.bad_request")
		return
	}
	err := s.sessions().Close(actor.UserID, id)
	switch {
	case errors.Is(err, session.ErrNotFound):
		writeError(w, http.StatusNotFound, "error.not_found")
		return
	case err != nil:
		slog.Error("closing a session", "error", err)
		writeError(w, http.StatusInternalServerError, "error.internal")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// handleCloseOtherSessions signs the account out everywhere but here.
func (s *Server) handleCloseOtherSessions(w http.ResponseWriter, r *http.Request) {
	actor, _ := middleware.ActorFrom(r)
	closed, err := s.sessions().CloseOthers(actor.UserID, middleware.SessionFrom(r))
	switch {
	case errors.Is(err, session.ErrNotFound):
		writeError(w, http.StatusBadRequest, "error.bad_request")
		return
	case err != nil:
		slog.Error("closing the other sessions", "error", err)
		writeError(w, http.StatusInternalServerError, "error.internal")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"closed": closed})
}
