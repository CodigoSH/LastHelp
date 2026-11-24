package web

import (
	"log/slog"
	"net"
	"net/http"
)

// noticeForwarded notices a proxy in front of a server that was told there is
// none.
//
// Behind a reverse proxy or a tunnel every request comes from the proxy, so with
// TRUST_PROXY off every visitor has the proxy's address: one line in each
// account's list of sessions, and one sign-in limit shared by everybody. Nothing
// says so, and the installer's question is easy to answer no to from behind a
// tunnel. A proxy announces itself with the headers it adds, so their arrival is
// the clue, said once in the log and kept for About.
//
// Only from a private or loopback peer, which is where a proxy that serves this
// one lives: anybody on the internet can type the header, and the notice would
// then be theirs to raise. It changes nothing by itself; whether there is a
// proxy stays the operator's to say.
func (s *Server) noticeForwarded(next http.Handler) http.Handler {
	if s.cfg.TrustProxy {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !s.forwardedUnread.Load() && forwardedByNeighbour(r) && s.forwardedUnread.CompareAndSwap(false, true) {
			slog.Warn("requests arrive through a proxy, but LASTHELP_TRUST_PROXY is off: every visitor gets the proxy's address. If a reverse proxy or a tunnel is in front, set LASTHELP_TRUST_PROXY=true and restart.")
		}
		next.ServeHTTP(w, r)
	})
}

// forwardedByNeighbour is a request that carries a proxy's headers and comes
// from a private or loopback address.
func forwardedByNeighbour(r *http.Request) bool {
	if r.Header.Get("X-Forwarded-For") == "" && r.Header.Get("X-Real-Ip") == "" && r.Header.Get("Forwarded") == "" {
		return false
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	ip := net.ParseIP(host)
	return ip != nil && (ip.IsLoopback() || ip.IsPrivate())
}
