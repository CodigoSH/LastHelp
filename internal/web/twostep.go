package web

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"git.codigosh.com/CodigoSH/LastHelp/internal/core/account"
	"git.codigosh.com/CodigoSH/LastHelp/internal/core/authz"
	"git.codigosh.com/CodigoSH/LastHelp/internal/core/settings"
	"git.codigosh.com/CodigoSH/LastHelp/internal/core/totp"
	"git.codigosh.com/CodigoSH/LastHelp/internal/core/twostep"
	"git.codigosh.com/CodigoSH/LastHelp/internal/web/middleware"
)

/* ---------------------------------------------------------------------------
 * The second step of signing in
 *
 * An account that turns it on signs in with its password and then a code from
 * an authenticator app — or one of its recovery codes, each good once. Between
 * the two the browser holds a challenge: it lasts a few minutes, takes a few
 * wrong codes, and is all the password buys on its own.
 *
 * The way back when the phone and the codes are both lost is the administrator,
 * who can take it off any account, and for the administrator's own account,
 * `lasthelp 2fa reset <username>` on the machine itself.
 * ------------------------------------------------------------------------ */

// twoStepBodyLimit bounds what these routes read: a code, a token, a password.
const twoStepBodyLimit = 4 << 10

// secondStepKey is the lock of one account's codes.
func secondStepKey(userID int64) string { return strconv.FormatInt(userID, 10) }

// handleSecondStep finishes a sign-in that passed the password: the challenge
// and a code. It answers like the sign-in does when it succeeds.
func (s *Server) handleSecondStep(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, twoStepBodyLimit)
	var in struct {
		Challenge string `json:"challenge"`
		Code      string `json:"code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil || in.Challenge == "" {
		writeError(w, http.StatusBadRequest, "error.bad_request")
		return
	}

	store := s.twoStep()
	userID, err := store.Challenge(in.Challenge, s.now())
	switch {
	case errors.Is(err, twostep.ErrChallengeGone):
		// Gone, spent, or out of time: the browser starts again from the password.
		writeError(w, http.StatusUnauthorized, "error.sign_in_expired")
		return
	case err != nil:
		slog.Error("reading a sign-in", "error", err)
		writeError(w, http.StatusInternalServerError, "error.internal")
		return
	}

	key := secondStepKey(userID)
	if left, locked := s.secondStep.Locked(key); locked {
		w.Header().Set("Retry-After", strconv.Itoa(int(left.Seconds())+1))
		writeError(w, http.StatusTooManyRequests, "error.too_many")
		return
	}

	switch err := store.Accept(userID, in.Code, s.now()); {
	case errors.Is(err, twostep.ErrRejected):
		s.secondStep.Fail(key)
		if err := store.FailChallenge(in.Challenge); err != nil {
			slog.Error("counting a wrong code", "error", err)
		}
		writeError(w, http.StatusUnauthorized, "error.code_wrong")
		return
	case err != nil:
		slog.Error("checking a code", "error", err)
		writeError(w, http.StatusInternalServerError, "error.internal")
		return
	}

	// A challenge is good for one sign-in: two requests with the same one and
	// two good codes do not make two sessions.
	if err := store.SpendChallenge(in.Challenge); err != nil {
		writeError(w, http.StatusUnauthorized, "error.sign_in_expired")
		return
	}
	s.secondStep.Succeed(key)

	user, err := s.accounts().ByID(userID)
	if err != nil || user.Disabled {
		// Disabled between the password and the code.
		writeError(w, http.StatusUnauthorized, "error.bad_credentials")
		return
	}
	s.startSession(w, r, user.ID)
	writeJSON(w, http.StatusOK, map[string]any{"id": user.ID, "username": user.Username})
}

/* ---------------------------------------------------------------------------
 * Turning it on and off, for the account signed in
 * ------------------------------------------------------------------------ */

// handleTwoStepStatus says whether the account has a second step, and how many
// recovery codes it has left.
func (s *Server) handleTwoStepStatus(w http.ResponseWriter, r *http.Request) {
	actor, ok := middleware.ActorFrom(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "error.unauthorized")
		return
	}
	st, err := s.twoStep().Status(actor.UserID)
	if err != nil {
		slog.Error("reading the second step", "error", err)
		writeError(w, http.StatusInternalServerError, "error.internal")
		return
	}
	writeJSON(w, http.StatusOK, st)
}

// totpIssuer is the name an authenticator app files the account under: the
// installation's own, so somebody with two of them can tell which is which.
func (s *Server) totpIssuer() string {
	if name, err := s.settings().Get(settings.InstanceName); err == nil && strings.TrimSpace(name) != "" {
		return strings.TrimSpace(name)
	}
	return "LastHelp"
}

// handleTwoStepSetup draws a new secret to scan. It is not in force until a code
// from it is confirmed.
func (s *Server) handleTwoStepSetup(w http.ResponseWriter, r *http.Request) {
	actor, ok := middleware.ActorFrom(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "error.unauthorized")
		return
	}
	user, err := s.accounts().ByID(actor.UserID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "error.internal")
		return
	}
	encoded, err := s.twoStep().Draft(actor.UserID, s.now())
	switch {
	case errors.Is(err, twostep.ErrAlreadyOn):
		writeError(w, http.StatusConflict, "error.two_step_on")
		return
	case err != nil:
		slog.Error("drawing a secret", "error", err)
		writeError(w, http.StatusInternalServerError, "error.internal")
		return
	}
	secret, err := totp.DecodeSecret(encoded)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "error.internal")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"secret": encoded,
		"uri":    totp.URI(s.totpIssuer(), user.Username, secret),
	})
}

// handleTwoStepEnable puts the scanned secret in force once a code from it proves
// the app has it, and answers with the recovery codes.
func (s *Server) handleTwoStepEnable(w http.ResponseWriter, r *http.Request) {
	actor, ok := middleware.ActorFrom(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "error.unauthorized")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, twoStepBodyLimit)
	var in struct {
		Code string `json:"code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "error.bad_request")
		return
	}

	tx, err := s.db.Begin()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "error.internal")
		return
	}
	defer func() { _ = tx.Rollback() }()
	codes, err := twostep.Store{DB: tx}.Enable(actor.UserID, in.Code, s.now())
	switch {
	case errors.Is(err, twostep.ErrRejected):
		writeError(w, http.StatusBadRequest, "error.code_wrong")
		return
	case errors.Is(err, twostep.ErrAlreadyOn):
		writeError(w, http.StatusConflict, "error.two_step_on")
		return
	case errors.Is(err, twostep.ErrNotSetUp):
		writeError(w, http.StatusConflict, "error.two_step_not_set_up")
		return
	case err != nil:
		slog.Error("turning the second step on", "error", err)
		writeError(w, http.StatusInternalServerError, "error.internal")
		return
	}
	if err := tx.Commit(); err != nil {
		writeError(w, http.StatusInternalServerError, "error.internal")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"recovery_codes": codes})
}

// confirmOwnPassword proves the password of the account signed in, held to the
// same lock as signing in: a stolen session must not be a place to guess the
// password from. It writes the refusal itself and says whether to go on.
func (s *Server) confirmOwnPassword(w http.ResponseWriter, r *http.Request, userID int64) bool {
	r.Body = http.MaxBytesReader(w, r.Body, twoStepBodyLimit)
	var in struct {
		Current string `json:"current"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil || in.Current == "" {
		writeError(w, http.StatusBadRequest, "error.bad_request")
		return false
	}
	user, err := s.accounts().ByID(userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "error.internal")
		return false
	}
	name := strings.ToLower(user.Username)
	if left, locked := s.signIn.Locked(name); locked {
		w.Header().Set("Retry-After", strconv.Itoa(int(left.Seconds())+1))
		writeError(w, http.StatusTooManyRequests, "error.too_many")
		return false
	}
	switch err := s.accounts().CheckPassword(userID, in.Current); {
	case errors.Is(err, account.ErrNotFound):
		s.signIn.Fail(name)
		writeError(w, http.StatusForbidden, "error.password_wrong")
		return false
	case err != nil:
		writeError(w, http.StatusInternalServerError, "error.internal")
		return false
	}
	s.signIn.Succeed(name)
	return true
}

// handleTwoStepDisable turns the account's second step off, with its password.
func (s *Server) handleTwoStepDisable(w http.ResponseWriter, r *http.Request) {
	actor, ok := middleware.ActorFrom(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "error.unauthorized")
		return
	}
	if !s.confirmOwnPassword(w, r, actor.UserID) {
		return
	}
	if err := s.removeTwoStep(actor.UserID); err != nil {
		writeError(w, http.StatusInternalServerError, "error.internal")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleTwoStepRecoveryCodes draws a new set in place of the old, with the password.
func (s *Server) handleTwoStepRecoveryCodes(w http.ResponseWriter, r *http.Request) {
	actor, ok := middleware.ActorFrom(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "error.unauthorized")
		return
	}
	if !s.confirmOwnPassword(w, r, actor.UserID) {
		return
	}
	if on, err := s.twoStep().Enabled(actor.UserID); err != nil {
		writeError(w, http.StatusInternalServerError, "error.internal")
		return
	} else if !on {
		writeError(w, http.StatusConflict, "error.two_step_not_set_up")
		return
	}
	tx, err := s.db.Begin()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "error.internal")
		return
	}
	defer func() { _ = tx.Rollback() }()
	codes, err := twostep.Store{DB: tx}.ReplaceRecoveryCodes(actor.UserID)
	if err != nil || tx.Commit() != nil {
		writeError(w, http.StatusInternalServerError, "error.internal")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"recovery_codes": codes})
}

// removeTwoStep takes the second step off an account in one transaction.
func (s *Server) removeTwoStep(userID int64) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := (twostep.Store{DB: tx}).Remove(userID); err != nil {
		slog.Error("removing the second step", "error", err)
		return err
	}
	return tx.Commit()
}

// handleRemovePersonTwoStep is the administrator taking the second step off an
// account: the way back for somebody who lost the phone and the codes.
func (s *Server) handleRemovePersonTwoStep(w http.ResponseWriter, r *http.Request) {
	if !s.may(w, r, authz.ManageServer, authz.Scope{}) {
		return
	}
	userID, ok := pathID(w, r)
	if !ok {
		return
	}
	if _, err := s.accounts().ByID(userID); errors.Is(err, account.ErrNotFound) {
		writeError(w, http.StatusNotFound, "error.not_found")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "error.internal")
		return
	}
	if err := s.removeTwoStep(userID); err != nil {
		writeError(w, http.StatusInternalServerError, "error.internal")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
