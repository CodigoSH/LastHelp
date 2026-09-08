package middleware

import "net/http"

// SecurityHeaders puts the same headers on every response.
//
// The policy is the strict one this application can actually keep, which is not
// the same as the strictest one that can be written down:
//
//   - script-src is 'self' and nothing else. The shell loads one module by URL
//     and there is not an inline script anywhere in it, so there is nothing to
//     allow and no nonce to carry — a nonce exists in the sibling project
//     because that one has inline scripts, and copying it here would be
//     machinery guarding nothing.
//   - style-src is 'self' too, which took work rather than a word. The
//     interface used to set style attributes — the size of a mark, the width of
//     a bar, where a tooltip lands — and there is no nonce for an attribute.
//     They are classes now, and the three values that are genuinely open-ended
//     — the operator's accent, a toast's countdown, the tooltip's position —
//     are written as rules into a stylesheet the document adopts, which the
//     policy does not govern. The browser walk fails if any of that regresses.
//   - img-src allows data:, because an avatar and an instance logo are stored
//     as data URLs in the database and rendered from there.
//   - default-src is 'none', so a directive nobody thought about refuses rather
//     than inherits: anything this page needs is named above, and there is no
//     third party in this product to make an exception for.
//
// frame-ancestors rather than X-Frame-Options: the header is the older spelling
// of the same rule and every browser this ships to reads the directive.
const contentSecurityPolicy = "default-src 'none'; " +
	"script-src 'self'; " +
	"style-src 'self'; " +
	"img-src 'self' data:; " +
	"font-src 'self'; " +
	"connect-src 'self'; " +
	"manifest-src 'self'; " +
	"form-action 'self'; " +
	"frame-ancestors 'none'; " +
	"base-uri 'none'; " +
	"object-src 'none'"

func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Security-Policy", contentSecurityPolicy)
		// A response whose type is guessed is a response somebody can make the
		// browser treat as something else.
		h.Set("X-Content-Type-Options", "nosniff")
		// Addresses in this product carry a follow token in the query string.
		// Sending that anywhere as a referrer would hand somebody else's report
		// to whatever a reporter clicks next.
		h.Set("Referrer-Policy", "no-referrer")
		next.ServeHTTP(w, r)
	})
}
