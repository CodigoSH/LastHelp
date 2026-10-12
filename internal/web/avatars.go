package web

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"net/http"
	"strconv"
	"strings"
)

// avatarTypes is what a picture beside a reply may be. Decided by the bytes: whoever
// uploads writes their own header, and the one thing they cannot choose is what the
// file is. A vector picture is left out on purpose — it is a document, and served on
// its own address it could carry markup.
var avatarTypes = map[string]bool{
	"image/png": true, "image/jpeg": true, "image/webp": true, "image/gif": true,
}

// handleAvatar serves the picture of a team member whose name is on a public thread.
//
// It answers for nothing else: an account number that has written no reply, one with
// no picture of its own and a picture that is not a plain raster all get the same
// "not found", so the address tells nobody who has an account.
func (s *Server) handleAvatar(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		http.NotFound(w, r)
		return
	}
	stored, ok := s.items().TeamAvatar(id)
	if !ok {
		http.NotFound(w, r)
		return
	}
	header, payload, found := strings.Cut(stored, ",")
	if !found || !strings.HasPrefix(header, "data:image/") || !strings.HasSuffix(header, ";base64") {
		http.NotFound(w, r)
		return
	}
	data, err := base64.StdEncoding.DecodeString(payload)
	if err != nil || len(data) == 0 {
		http.NotFound(w, r)
		return
	}
	contentType := http.DetectContentType(data)
	if !avatarTypes[contentType] {
		http.NotFound(w, r)
		return
	}

	sum := sha256.Sum256(data)
	etag := `"` + hex.EncodeToString(sum[:8]) + `"`
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("ETag", etag)
	// Public, and asked again every time: a picture somebody changes shows at once,
	// and the answer to "has it changed" is a few bytes.
	w.Header().Set("Cache-Control", "public, no-cache")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; sandbox")
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	_, _ = w.Write(data)
}
