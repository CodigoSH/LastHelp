// Package middleware holds the wrappers every route is built out of.
//
// The important one is the absence of a list. Nothing here knows which paths
// are public: a route is public by *not being wrapped*, which is a decision
// visible where the route is declared. A list of public prefixes inside a
// middleware is how this house once served every non-existent file under
// /dist/ without a session — those fall through to the application shell, and
// the prefix matched.
package middleware

import (
	"log/slog"
	"net/http"
)

// Recover turns a panic into a 500 with a logged stack.
//
// Without it the browser gets a severed connection — a network error with no
// status — and the log gets a stack with nothing about the request that caused
// it. http.ErrAbortHandler is re-panicked because the standard library raises
// it on purpose to abort a response, and swallowing it would turn a deliberate
// abort into a logged crash.
func Recover(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			rec := recover()
			if rec == nil {
				return
			}
			if rec == http.ErrAbortHandler {
				panic(rec)
			}
			slog.Error("panic serving a request",
				"error", rec, "method", r.Method, "path", r.URL.Path)
			http.Error(w, "error.internal", http.StatusInternalServerError)
		}()
		next.ServeHTTP(w, r)
	})
}

// BodyLimit caps how much a request may send.
//
// It is the backstop, not the real limit: each handler sets its own, sized to
// what it expects. This one exists so that a handler which forgets is merely
// wrong rather than a way to fill memory.
func BodyLimit(max int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r.Body = http.MaxBytesReader(w, r.Body, max)
			next.ServeHTTP(w, r)
		})
	}
}

// CrossSite refuses a state-changing request that came from another site.
//
// Sec-Fetch-Site is written by the browser from the actual origins involved and
// a page cannot forge it, so this rejects a cross-site write before any token
// is even looked at. When the header is absent — an old browser, curl, a test —
// it does not refuse: the CSRF token check still applies, and breaking every
// non-browser client to enforce a header they never send would be trading a
// real feature for no security.
func CrossSite(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if safeMethod(r.Method) {
			next.ServeHTTP(w, r)
			return
		}
		switch r.Header.Get("Sec-Fetch-Site") {
		case "", "same-origin", "same-site", "none":
			// "none" is a user typing the address or opening a bookmark.
			next.ServeHTTP(w, r)
		default:
			http.Error(w, "error.cross_site", http.StatusForbidden)
		}
	})
}

func safeMethod(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return true
	}
	return false
}
