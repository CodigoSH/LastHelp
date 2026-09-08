package middleware

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"net/http"
)

// CSRFCookie is readable by the page on purpose: the script reads it and echoes
// it back in a header, which is what proves the request came from a page on
// this origin rather than from a form somebody else hosted. It carries no
// authority of its own — the session cookie does — so it is not HttpOnly.
const CSRFCookie = "lasthelp_csrf"

// CSRFHeader is where the value comes back.
const CSRFHeader = "X-CSRF-Token"

// CSRF is the double-submit check.
//
// It sits behind [CrossSite], which already refuses anything a browser labels
// cross-site. This is the belt to that pair of braces: it covers the clients
// that send no Sec-Fetch-Site, where the cross-site check deliberately does not
// refuse.
func CSRF(secure bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			cookie, err := r.Cookie(CSRFCookie)
			if err != nil || cookie.Value == "" {
				token, genErr := newToken()
				if genErr != nil {
					Refuse(w, http.StatusInternalServerError, "error.internal")
					return
				}
				cookie = &http.Cookie{
					Name:     CSRFCookie,
					Value:    token,
					Path:     "/",
					Secure:   secure,
					SameSite: http.SameSiteLaxMode,
					// Deliberately readable: the page has to echo it back.
					HttpOnly: false,
				}
				http.SetCookie(w, cookie)
			}

			if safeMethod(r.Method) {
				next.ServeHTTP(w, r)
				return
			}

			sent := r.Header.Get(CSRFHeader)
			if sent == "" || subtle.ConstantTimeCompare([]byte(sent), []byte(cookie.Value)) != 1 {
				Refuse(w, http.StatusForbidden, "error.csrf")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func newToken() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}
