package web

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"strings"

	"git.codigosh.com/CodigoSH/LastHelp/internal/core/passkey"
	"git.codigosh.com/CodigoSH/LastHelp/internal/core/webauthn"
	"git.codigosh.com/CodigoSH/LastHelp/internal/web/middleware"
)

/* ---------------------------------------------------------------------------
 * Passkeys: signing in without a password
 *
 * A passkey is made while signed in, from Profile, and from then on signs in
 * with nothing typed: the device asks for its fingerprint, face or PIN, and
 * signs. That is both factors at once, so a passkey sign-in asks for no code
 * after it. The password keeps working as it did.
 *
 * A passkey belongs to a site by name, so the site is read from each request.
 * Browsers only offer passkeys on https or on localhost, and never for a bare
 * IP address; the server refuses there too, with a reason the screen can say.
 * ------------------------------------------------------------------------ */

const (
	// passkeyTimeoutMS is how long the browser's own dialog waits, as WebAuthn asks for it.
	passkeyTimeoutMS = 300000
	// passkeyBodyLimit bounds what a ceremony sends: a key, a signature, two small JSONs.
	passkeyBodyLimit = 64 << 10
)

// errPasskeyUnavailable: this address cannot have passkeys.
var errPasskeyUnavailable = errors.New("passkeys need https and a name, not an IP address")

// relyingParty is the site a passkey belongs to, as this request reached it:
// its name (the host without the port) and its origin. The host a proxy says it
// forwarded is read only when a proxy is declared; otherwise it is what the
// caller typed.
func (s *Server) relyingParty(r *http.Request) (rpID, origin string, err error) {
	host := ""
	if s.cfg.TrustProxy {
		host = strings.TrimSpace(strings.Split(r.Header.Get("X-Forwarded-Host"), ",")[0])
	}
	if host == "" {
		host = r.Host
	}
	name := host
	if h, _, splitErr := net.SplitHostPort(host); splitErr == nil {
		name = h
	}
	name = strings.TrimSuffix(strings.ToLower(name), ".")
	if name == "" || net.ParseIP(strings.Trim(name, "[]")) != nil {
		return "", "", errPasskeyUnavailable
	}
	secure := s.isSecure(r)
	if !secure && name != "localhost" && !strings.HasSuffix(name, ".localhost") {
		return "", "", errPasskeyUnavailable
	}
	scheme := "http"
	if secure {
		scheme = "https"
	}
	return name, scheme + "://" + strings.ToLower(host), nil
}

func decodeB64Field(s string) ([]byte, bool) {
	b, err := webauthn.B64.DecodeString(strings.TrimRight(s, "="))
	return b, err == nil && len(b) > 0
}

/* ---------------------------------------------------------------------------
 * The account's own passkeys
 * ------------------------------------------------------------------------ */

// handleListPasskeys answers with the account's passkeys, and whether this
// address can make one: the screen says why not, instead of a button that fails.
func (s *Server) handleListPasskeys(w http.ResponseWriter, r *http.Request) {
	actor, ok := middleware.ActorFrom(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "error.unauthorized")
		return
	}
	list, err := s.passkeys().List(actor.UserID)
	if err != nil {
		slog.Error("listing passkeys", "error", err)
		writeError(w, http.StatusInternalServerError, "error.internal")
		return
	}
	_, _, unavailable := s.relyingParty(r)
	writeJSON(w, http.StatusOK, map[string]any{"available": unavailable == nil, "passkeys": list})
}

// handlePasskeyRegisterBegin hands the browser what navigator.credentials.create needs.
func (s *Server) handlePasskeyRegisterBegin(w http.ResponseWriter, r *http.Request) {
	actor, ok := middleware.ActorFrom(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "error.unauthorized")
		return
	}
	rpID, _, err := s.relyingParty(r)
	if err != nil {
		writeError(w, http.StatusConflict, "error.passkey_unavailable")
		return
	}
	user, err := s.accounts().ByID(actor.UserID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "error.internal")
		return
	}
	store := s.passkeys()
	handle, err := store.Handle(actor.UserID)
	if err != nil {
		slog.Error("drawing a passkey handle", "error", err)
		writeError(w, http.StatusInternalServerError, "error.internal")
		return
	}
	challenge, err := store.NewChallenge(passkey.PurposeRegister, actor.UserID, s.now())
	if err != nil {
		slog.Error("starting a passkey", "error", err)
		writeError(w, http.StatusInternalServerError, "error.internal")
		return
	}
	// The passkeys it already has, so the same device does not make a second.
	ids, err := store.CredentialIDs(actor.UserID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "error.internal")
		return
	}
	exclude := make([]map[string]string, 0, len(ids))
	for _, id := range ids {
		exclude = append(exclude, map[string]string{"type": "public-key", "id": id})
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"challenge": webauthn.B64.EncodeToString(challenge),
		"rp":        map[string]string{"id": rpID, "name": s.totpIssuer()},
		"user": map[string]string{
			"id": webauthn.B64.EncodeToString(handle), "name": user.Username, "displayName": user.Username,
		},
		"pubKeyCredParams": []map[string]any{
			{"type": "public-key", "alg": webauthn.AlgES256},
			{"type": "public-key", "alg": webauthn.AlgEdDSA},
		},
		"timeout":     passkeyTimeoutMS,
		"attestation": "none",
		"authenticatorSelection": map[string]any{
			"residentKey": "required", "requireResidentKey": true, "userVerification": "required",
		},
		"excludeCredentials": exclude,
	})
}

// handlePasskeyRegisterFinish checks what the device made and keeps its public key.
func (s *Server) handlePasskeyRegisterFinish(w http.ResponseWriter, r *http.Request) {
	actor, ok := middleware.ActorFrom(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "error.unauthorized")
		return
	}
	rpID, origin, err := s.relyingParty(r)
	if err != nil {
		writeError(w, http.StatusConflict, "error.passkey_unavailable")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, passkeyBodyLimit)
	var in struct {
		Name              string `json:"name"`
		ClientDataJSON    string `json:"clientDataJSON"`
		AttestationObject string `json:"attestationObject"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "error.bad_request")
		return
	}
	clientData, ok1 := decodeB64Field(in.ClientDataJSON)
	attestation, ok2 := decodeB64Field(in.AttestationObject)
	if !ok1 || !ok2 {
		writeError(w, http.StatusBadRequest, "error.bad_request")
		return
	}

	challenge, err := webauthn.CheckClientData(clientData, webauthn.TypeCreate, origin)
	if err != nil {
		slog.Warn("passkey creation refused", "user", actor.UserID, "why", err)
		writeError(w, http.StatusBadRequest, "error.passkey_refused")
		return
	}
	store := s.passkeys()
	owner, err := store.TakeChallenge(challenge, passkey.PurposeRegister, s.now())
	if errors.Is(err, passkey.ErrChallengeGone) || (err == nil && owner != actor.UserID) {
		writeError(w, http.StatusBadRequest, "error.passkey_expired")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "error.internal")
		return
	}
	ad, err := webauthn.ParseAttestationObject(attestation)
	if err == nil {
		err = ad.Check(rpID)
	}
	var key webauthn.PublicKey
	if err == nil {
		key, err = webauthn.ParsePublicKey(ad.PublicKey)
	}
	if err != nil {
		slog.Warn("passkey creation refused", "user", actor.UserID, "why", err)
		writeError(w, http.StatusBadRequest, "error.passkey_refused")
		return
	}
	id, err := store.Add(actor.UserID, passkey.New{
		CredentialID: webauthn.B64.EncodeToString(ad.CredentialID),
		PublicKey:    ad.PublicKey,
		Alg:          key.Alg,
		SignCount:    ad.SignCount,
		Name:         strings.TrimSpace(in.Name),
	}, s.now())
	if errors.Is(err, passkey.ErrAlreadyAdded) {
		writeError(w, http.StatusConflict, "error.passkey_already_added")
		return
	}
	if err != nil {
		slog.Error("storing a passkey", "error", err)
		writeError(w, http.StatusInternalServerError, "error.internal")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"id": id})
}

// handleDeletePasskey removes one of the account's passkeys.
func (s *Server) handleDeletePasskey(w http.ResponseWriter, r *http.Request) {
	actor, ok := middleware.ActorFrom(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "error.unauthorized")
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "error.bad_request")
		return
	}
	switch err := s.passkeys().Delete(actor.UserID, id); {
	case errors.Is(err, passkey.ErrNotFound):
		writeError(w, http.StatusNotFound, "error.not_found")
		return
	case err != nil:
		writeError(w, http.StatusInternalServerError, "error.internal")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

/* ---------------------------------------------------------------------------
 * Signing in with a passkey
 * ------------------------------------------------------------------------ */

// handlePasskeySignInBegin hands the browser a challenge. It does not ask who
// is signing in: the passkey the person picks says so.
func (s *Server) handlePasskeySignInBegin(w http.ResponseWriter, r *http.Request) {
	rpID, _, err := s.relyingParty(r)
	if err != nil {
		writeError(w, http.StatusConflict, "error.passkey_unavailable")
		return
	}
	challenge, err := s.passkeys().NewChallenge(passkey.PurposeSignIn, 0, s.now())
	if err != nil {
		slog.Error("starting a passkey sign-in", "error", err)
		writeError(w, http.StatusInternalServerError, "error.internal")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"challenge":        webauthn.B64.EncodeToString(challenge),
		"rpId":             rpID,
		"timeout":          passkeyTimeoutMS,
		"userVerification": "required",
	})
}

// handlePasskeySignInFinish checks the passkey's signature and signs its account in.
func (s *Server) handlePasskeySignInFinish(w http.ResponseWriter, r *http.Request) {
	address, _ := s.addressOf(r)
	if left, locked := s.passkeyIn.Locked(address); locked {
		w.Header().Set("Retry-After", strconv.Itoa(int(left.Seconds())+1))
		writeError(w, http.StatusTooManyRequests, "error.too_many")
		return
	}
	rpID, origin, err := s.relyingParty(r)
	if err != nil {
		writeError(w, http.StatusConflict, "error.passkey_unavailable")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, passkeyBodyLimit)
	var in struct {
		CredentialID      string `json:"credentialId"`
		ClientDataJSON    string `json:"clientDataJSON"`
		AuthenticatorData string `json:"authenticatorData"`
		Signature         string `json:"signature"`
		UserHandle        string `json:"userHandle"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "error.bad_request")
		return
	}
	credID, ok1 := decodeB64Field(in.CredentialID)
	clientData, ok2 := decodeB64Field(in.ClientDataJSON)
	authData, ok3 := decodeB64Field(in.AuthenticatorData)
	sig, ok4 := decodeB64Field(in.Signature)
	if !ok1 || !ok2 || !ok3 || !ok4 {
		writeError(w, http.StatusBadRequest, "error.bad_request")
		return
	}

	// Every way this can fail looks the same from outside, and counts.
	refuse := func(why error) {
		slog.Warn("passkey sign-in refused", "why", why)
		s.passkeyIn.Fail(address)
		writeError(w, http.StatusUnauthorized, "error.passkey_refused")
	}

	challenge, err := webauthn.CheckClientData(clientData, webauthn.TypeGet, origin)
	if err != nil {
		refuse(err)
		return
	}
	store := s.passkeys()
	if _, err := store.TakeChallenge(challenge, passkey.PurposeSignIn, s.now()); errors.Is(err, passkey.ErrChallengeGone) {
		writeError(w, http.StatusUnauthorized, "error.passkey_expired")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "error.internal")
		return
	}

	stored, err := store.ByCredential(webauthn.B64.EncodeToString(credID))
	if errors.Is(err, passkey.ErrNotFound) {
		refuse(err)
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "error.internal")
		return
	}
	// A device that names its account must name this one.
	if in.UserHandle != "" {
		if sent, ok := decodeB64Field(in.UserHandle); !ok || string(sent) != string(stored.Handle) {
			refuse(errors.New("the passkey named another account"))
			return
		}
	}
	ad, err := webauthn.ParseAuthData(authData)
	if err == nil {
		err = ad.Check(rpID)
	}
	var key webauthn.PublicKey
	if err == nil {
		key, err = webauthn.ParsePublicKey(stored.PublicKey)
	}
	if err == nil {
		err = key.Verify(authData, clientData, sig)
	}
	if err == nil && !webauthn.CounterAllows(stored.SignCount, ad.SignCount) {
		err = errors.New("the signature counter went back: the passkey may have been copied")
	}
	if err != nil {
		refuse(err)
		return
	}
	if err := store.Used(stored.ID, stored.SignCount, ad.SignCount, s.now()); err != nil {
		refuse(err)
		return
	}

	user, err := s.accounts().ByID(stored.UserID)
	if err != nil || user.Disabled {
		refuse(errors.New("the account is disabled or gone"))
		return
	}
	s.passkeyIn.Succeed(address)
	s.startSession(w, r, user.ID)
	writeJSON(w, http.StatusOK, map[string]any{"id": user.ID, "username": user.Username})
}
