package web

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"git.codigosh.com/CodigoSH/LastHelp/internal/core/account"
	"git.codigosh.com/CodigoSH/LastHelp/internal/core/session"
	"git.codigosh.com/CodigoSH/LastHelp/internal/web/middleware"
)

// Bodies are bounded per handler, sized to what each one expects. The global
// cap is the backstop for a handler that forgets; these are the real limits.
const credentialsBodyLimit = 4 << 10 // 4 KiB

// minPasswordLength is a floor, not a policy. No composition rules: they push
// people towards "Password1!" and away from the long passphrase that is
// actually strong, and argon2id does not care what characters are in it.
const minPasswordLength = 10

type credentials struct {
	Email       string `json:"email"`
	Password    string `json:"password"`
	DisplayName string `json:"display_name"`
}

// handleStatus tells the page whether this instance has been claimed yet. It is
// public because the setup screen has to be reachable before anybody exists.
func (s *Server) handleStatus(w http.ResponseWriter, _ *http.Request) {
	count, err := s.accounts().Count()
	if err != nil {
		slog.Error("counting accounts", "error", err)
		writeError(w, http.StatusInternalServerError, "error.internal")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"needs_setup": count == 0})
}

// handleSetup claims an unclaimed instance, creating the administrator.
//
// The guard is the account count, checked inside the same request that inserts.
// It is not a race worth locking against: SQLite serialises the write, and the
// second caller's insert fails on the unique index even if both read zero.
func (s *Server) handleSetup(w http.ResponseWriter, r *http.Request) {
	count, err := s.accounts().Count()
	if err != nil {
		slog.Error("counting accounts", "error", err)
		writeError(w, http.StatusInternalServerError, "error.internal")
		return
	}
	if count > 0 {
		// Deliberately not "already set up, sign in here": an unauthenticated
		// caller learns nothing about an instance that is not theirs.
		writeError(w, http.StatusForbidden, "error.already_set_up")
		return
	}

	in, ok := readCredentials(w, r)
	if !ok {
		return
	}

	user, err := s.accounts().Create(in.Email, in.Password, in.DisplayName, true, s.now())
	if err != nil {
		if errors.Is(err, account.ErrEmailTaken) {
			writeError(w, http.StatusConflict, "error.email_taken")
			return
		}
		slog.Error("creating the first account", "error", err)
		writeError(w, http.StatusInternalServerError, "error.internal")
		return
	}

	slog.Info("instance claimed", "user", user.ID)
	s.startSession(w, r, user.ID)
	writeJSON(w, http.StatusCreated, map[string]any{"id": user.ID, "email": user.Email})
}

// handleSignIn exchanges an email and password for a session.
func (s *Server) handleSignIn(w http.ResponseWriter, r *http.Request) {
	in, ok := readCredentials(w, r)
	if !ok {
		return
	}

	user, err := s.accounts().Authenticate(in.Email, in.Password)
	if err != nil {
		if errors.Is(err, account.ErrNotFound) {
			// One answer for a wrong password and an address with no account.
			writeError(w, http.StatusUnauthorized, "error.bad_credentials")
			return
		}
		slog.Error("authenticating", "error", err)
		writeError(w, http.StatusInternalServerError, "error.internal")
		return
	}

	s.startSession(w, r, user.ID)
	writeJSON(w, http.StatusOK, map[string]any{"id": user.ID, "email": user.Email})
}

// handleSignOut ends the session, in the database and not only in the browser.
// A token the browser forgot is still a token somebody could have kept.
func (s *Server) handleSignOut(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(middleware.SessionCookie); err == nil {
		if err := s.sessions().Delete(cookie.Value); err != nil {
			slog.Error("deleting the session", "error", err)
		}
	}
	http.SetCookie(w, &http.Cookie{
		Name:     middleware.SessionCookie,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   s.isSecure(r),
		SameSite: http.SameSiteLaxMode,
	})
	w.WriteHeader(http.StatusNoContent)
}

// handleMe describes whoever is signed in, so the panel can draw itself.
//
// It reports memberships rather than a verdict on each button: the page uses
// them to decide what to show, and the server decides again on every request
// that matters. What the page draws is a convenience, never the check.
func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	actor, ok := middleware.ActorFrom(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "error.unauthorized")
		return
	}

	memberships := make([]map[string]any, 0, len(actor.Memberships))
	for _, m := range actor.Memberships {
		memberships = append(memberships, map[string]any{
			"scope":  string(m.Scope.Kind),
			"id":     m.Scope.ID,
			"org_id": m.Scope.OrgID,
			"role":   string(m.Role),
		})
	}
	// The wire format deliberately does not repeat the role vocabulary. If a
	// role is ever renamed, a key spelled out here would quietly become stale —
	// which is the same reason the architecture rule refuses to see role names
	// outside authz, and it fired on this exact line.
	writeJSON(w, http.StatusOK, map[string]any{
		"id":          actor.UserID,
		"admin":       actor.IsAdministrator,
		"memberships": memberships,
	})
}

func (s *Server) startSession(w http.ResponseWriter, r *http.Request, userID int64) {
	token, err := s.sessions().Create(userID, s.now(), session.DefaultTTL)
	if err != nil {
		slog.Error("creating the session", "error", err)
		writeError(w, http.StatusInternalServerError, "error.internal")
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     middleware.SessionCookie,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   s.isSecure(r),
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(session.DefaultTTL.Seconds()),
	})
}

func readCredentials(w http.ResponseWriter, r *http.Request) (credentials, bool) {
	r.Body = http.MaxBytesReader(w, r.Body, credentialsBodyLimit)

	var in credentials
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "error.bad_request")
		return credentials{}, false
	}
	if strings.TrimSpace(in.Email) == "" || !strings.Contains(in.Email, "@") {
		writeError(w, http.StatusBadRequest, "error.email_required")
		return credentials{}, false
	}
	if len(in.Password) < minPasswordLength {
		writeError(w, http.StatusBadRequest, "error.password_too_short")
		return credentials{}, false
	}
	return in, true
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

// writeError sends a translation key, not a sentence.
//
// The server does not know what language the reader has chosen, and a message
// composed here would be the one string on the screen that stayed English.
func writeError(w http.ResponseWriter, status int, key string) {
	writeJSON(w, status, map[string]string{"error": key})
}
