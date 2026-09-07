package web

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"net/http"
	"strings"
	"time"
)

// VoterCookie identifies a browser for the purpose of voting. It carries no
// authority: the worst somebody can do with a stolen one is un-vote.
const VoterCookie = "lasthelp_voter"

// voterCookieLife is deliberately long. A voter identity that expires is a
// person who can vote for the same idea again next year, and the count is the
// one number the roadmap is ordered by.
const voterCookieLife = 5 * 365 * 24 * time.Hour

// voterID reads the signed voter cookie, or mints one.
//
// The identity is a random value in a signed cookie, never the address. Behind
// a household router or a mobile carrier the address is shared, so keying votes
// on it would let one neighbour spend everybody else's vote — and it would let
// a single office look like one voter.
//
// Clearing the cookie allows a second vote. That is a documented limit rather
// than a defect: the alternative is asking people to create an account, which
// is the thing this product exists not to do, and the honest answer is that a
// vote count here is a signal rather than a ballot.
func (s *Server) voterID(w http.ResponseWriter, r *http.Request) string {
	if cookie, err := r.Cookie(VoterCookie); err == nil {
		if id, ok := s.openVoterCookie(cookie.Value); ok {
			return id
		}
	}

	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return ""
	}
	id := base64.RawURLEncoding.EncodeToString(raw)

	http.SetCookie(w, &http.Cookie{
		Name:     VoterCookie,
		Value:    s.sealVoterCookie(id),
		Path:     "/",
		HttpOnly: true,
		Secure:   s.isSecure(r),
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(voterCookieLife.Seconds()),
	})
	return id
}

// sealVoterCookie appends a signature, so a voter id cannot be chosen. Without
// it, one person could vote as a thousand by editing a cookie, which is a
// cheaper attack than clearing it a thousand times.
func (s *Server) sealVoterCookie(id string) string {
	return id + "." + base64.RawURLEncoding.EncodeToString(s.signVoter(id))
}

func (s *Server) openVoterCookie(value string) (string, bool) {
	id, signature, found := strings.Cut(value, ".")
	if !found || id == "" {
		return "", false
	}
	given, err := base64.RawURLEncoding.DecodeString(signature)
	if err != nil {
		return "", false
	}
	if !hmac.Equal(given, s.signVoter(id)) {
		return "", false
	}
	return id, true
}

func (s *Server) signVoter(id string) []byte {
	mac := hmac.New(sha256.New, s.voterKey)
	mac.Write([]byte(id))
	return mac.Sum(nil)
}
