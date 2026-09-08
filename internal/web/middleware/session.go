package middleware

import (
	"context"
	"errors"
	"net/http"
	"time"

	"git.codigosh.com/CodigoSH/LastHelp/internal/core/account"
	"git.codigosh.com/CodigoSH/LastHelp/internal/core/authz"
	"git.codigosh.com/CodigoSH/LastHelp/internal/core/session"
)

// SessionCookie carries the token. HttpOnly so no script can read it, SameSite
// Lax so it does not ride along on a cross-site write, Secure whenever the
// server knows it is behind TLS.
const SessionCookie = "lasthelp_session"

type actorCtxKey struct{}

// Authenticator resolves the cookie on a request into the actor authz answers
// questions about.
type Authenticator struct {
	Sessions session.Store
	Accounts account.Store
	Now      func() time.Time
}

// Attach reads the session if there is one and puts the actor on the context.
// It never refuses: a request that does not need a session carries on without
// one, and [RequireActor] is what closes the door where the door belongs.
func (a Authenticator) Attach(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(SessionCookie)
		if err != nil || cookie.Value == "" {
			next.ServeHTTP(w, r)
			return
		}

		userID, err := a.Sessions.Resolve(cookie.Value, a.Now(), session.DefaultTTL)
		if err != nil {
			// An invalid session is not an error to shout about — expiry is
			// normal — but a broken query is, and Resolve keeps them apart.
			if !errors.Is(err, session.ErrInvalid) {
				Refuse(w, http.StatusInternalServerError, "error.internal")
				return
			}
			next.ServeHTTP(w, r)
			return
		}

		actor, err := a.Accounts.Actor(userID)
		if err != nil {
			// The account went away or was disabled while the session lived.
			next.ServeHTTP(w, r)
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), actorCtxKey{}, actor)))
	})
}

// RequireActor refuses anybody who is not signed in.
//
// This is what "private by default" means in practice: a route is protected by
// being wrapped in this, and public by not being wrapped. There is no list of
// paths anywhere for somebody to get wrong.
func RequireActor(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := ActorFrom(r); !ok {
			Refuse(w, http.StatusUnauthorized, "error.unauthorized")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// ActorFrom returns the actor attached to a request, if there is one. The zero
// actor can do nothing, so a handler that forgets to check gets refused by
// authz rather than waved through.
func ActorFrom(r *http.Request) (authz.Actor, bool) {
	actor, ok := r.Context().Value(actorCtxKey{}).(authz.Actor)
	return actor, ok
}
