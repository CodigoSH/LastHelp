package middleware

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"git.codigosh.com/CodigoSH/LastHelp/internal/core/session"
	"git.codigosh.com/CodigoSH/LastHelp/internal/platform/config"
)

/* ---------------------------------------------------------------------------
 * Signing in through a proxy (forward-auth)
 *
 * A proxy in front that has already signed the person in says who it is in a
 * header; it is believed only from the proxy's own addresses, and only for an
 * account that already exists by that name and is not disabled. It never makes
 * an account and never changes what an account may do: the administrator
 * decides both, as always. The proxy is the sign-in, so nothing more is asked —
 * not a password, not a code.
 *
 * What it opens is an ordinary stored session, so the rest of the server needs
 * to know nothing about proxies: Active sessions lists it, and closing it ends it.
 * ------------------------------------------------------------------------ */

// ProxySessionLife is how long a session opened by the proxy lasts unused. The
// proxy is asked again by every request anyway; this only bounds a cookie that
// outlives the proxy's own session.
const ProxySessionLife = 24 * time.Hour

// maxProxyName bounds the header's value, in characters.
const maxProxyName = 128

// FromAuthProxy says whether the request came from the trusted proxy — judged
// by the address that opened the connection, never by a header.
func FromAuthProxy(p config.AuthProxy, r *http.Request) bool {
	if !p.Enabled() {
		return false
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	addr, err := netip.ParseAddr(host)
	return err == nil && p.Trusts(addr)
}

// proxiedName is the username the proxy says is signed in, or "".
func proxiedName(p config.AuthProxy, r *http.Request) string {
	if !FromAuthProxy(p, r) {
		return ""
	}
	name := strings.TrimSpace(r.Header.Get(p.Header))
	if name == "" || utf8.RuneCountInString(name) > maxProxyName || strings.IndexFunc(name, unicode.IsControl) >= 0 {
		return ""
	}
	return name
}

// proxied attaches the account the proxy names, when it names one, and says
// whether it handled the request. The session the browser carries is used only
// if it is that person's; otherwise a read opens one for them. A write never
// does — its CSRF token belongs to the session it came with — and goes on with
// nobody signed in rather than as whoever that session was.
func (a Authenticator) proxied(w http.ResponseWriter, r *http.Request, next http.Handler) bool {
	name := proxiedName(a.Proxy, r)
	if name == "" {
		return false
	}
	user, err := a.Accounts.ByUsername(name)
	if err != nil || user.Disabled {
		// A name with no account here: the proxy signed somebody in that this
		// server does not know, and nobody is signed in.
		next.ServeHTTP(w, r)
		return true
	}
	if cookie, err := r.Cookie(SessionCookie); err == nil && cookie.Value != "" {
		found, err := a.Sessions.Resolve(cookie.Value, a.Now(), session.DefaultTTL, a.Device(r))
		if err == nil && found.UserID == user.ID {
			return false // their own session: the ordinary path carries on
		}
		if err != nil && !errors.Is(err, session.ErrInvalid) {
			Refuse(w, http.StatusInternalServerError, "error.internal")
			return true
		}
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		next.ServeHTTP(w, r)
		return true
	}
	token, err := a.Sessions.Create(user.ID, a.Now(), ProxySessionLife, a.Device(r))
	if err != nil {
		slog.Error("opening a session for the proxy's account", "error", err)
		Refuse(w, http.StatusInternalServerError, "error.internal")
		return true
	}
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookie,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   a.Secure != nil && a.Secure(r),
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(ProxySessionLife.Seconds()),
	})
	found, err := a.Sessions.Resolve(token, a.Now(), ProxySessionLife, a.Device(r))
	if err != nil {
		Refuse(w, http.StatusInternalServerError, "error.internal")
		return true
	}
	actor, err := a.Accounts.Actor(user.ID)
	if err != nil {
		next.ServeHTTP(w, r)
		return true
	}
	ctx := context.WithValue(r.Context(), actorCtxKey{}, actor)
	ctx = context.WithValue(ctx, sessionCtxKey{}, found.ID)
	next.ServeHTTP(w, r.WithContext(ctx))
	return true
}
