package web

import (
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"

	"git.codigosh.com/CodigoSH/LastHelp/internal/core/item"
	"git.codigosh.com/CodigoSH/LastHelp/internal/core/picture"
)

// FollowTokenHeader carries a reporter's follow link on a request the page
// makes for them: an upload or a removal. A picture the page draws cannot send
// a header, so reading one takes the same secret as ?k=, the way the report
// page itself does.
const FollowTokenHeader = "X-Follow-Token"

// pictureRights is who is asking about a report's pictures. Written once,
// here, because three routes ask and a rule decided three times is decided
// three ways.
type pictureRights struct {
	team     bool // may triage the board the report is on
	reporter bool // holds the report's follow link
}

func (s *Server) pictureRightsFor(r *http.Request, it item.Item) pictureRights {
	rights := pictureRights{team: s.mayTriageItem(r, it)}
	token := r.Header.Get(FollowTokenHeader)
	if token == "" {
		token = r.URL.Query().Get("k")
	}
	if token != "" {
		if _, err := s.items().CheckFollowToken(it.PublicID, token); err == nil {
			rights.reporter = true
		}
	}
	return rights
}

// mayAdd is who adds pictures where. The reporter adds to their report and to
// their own follow-ups; the team adds to its own replies. Nobody else adds
// anything: a visitor's comment carries words only, which was the decision.
func (p pictureRights) mayAdd(on *item.Comment) bool {
	if on == nil {
		return p.reporter
	}
	switch on.Author {
	case item.AuthorReporter:
		return p.reporter
	case item.AuthorTeam:
		return p.team
	}
	return false
}

// mayRemove is who takes one out: the team, any of them; the reporter, the
// ones they added.
func (p pictureRights) mayRemove(on *item.Comment) bool {
	if p.team {
		return true
	}
	return p.reporter && (on == nil || on.Author == item.AuthorReporter)
}

// maySee is whether a picture is shown to this reader. It follows what it
// belongs to: a picture on a report waiting for review waits with it, and one
// on a message waits with the message. The team sees everything, which is the
// point of looking before approving; the reporter sees their own report.
func (p pictureRights) maySee(it item.Item, on *item.Comment) bool {
	if p.team {
		return true
	}
	if it.MergedInto != 0 {
		return false
	}
	if on != nil && on.Moderation != item.Published {
		return p.reporter && on.Author == item.AuthorReporter
	}
	return it.Moderation == item.Published || p.reporter
}

// commentFor reads the message a picture belongs to, or nil for the report.
func (s *Server) commentFor(it item.Item, commentID int64) (*item.Comment, error) {
	if commentID == 0 {
		return nil, nil
	}
	c, err := s.items().CommentByID(commentID)
	if err != nil {
		return nil, err
	}
	if c.ItemID != it.ID {
		return nil, item.ErrNotFound
	}
	return &c, nil
}

// handleAddPicture takes one picture for a report, or for one message on it
// with ?comment=. The body is the picture itself, as the browser prepared it.
func (s *Server) handleAddPicture(w http.ResponseWriter, r *http.Request) {
	it, ok := s.lookupItem(w, r)
	if !ok {
		return
	}
	commentID, _ := strconv.ParseInt(r.URL.Query().Get("comment"), 10, 64)
	on, err := s.commentFor(it, commentID)
	if err != nil {
		writeError(w, http.StatusNotFound, "error.not_found")
		return
	}
	if !s.pictureRightsFor(r, it).mayAdd(on) {
		writeError(w, http.StatusForbidden, "error.forbidden")
		return
	}

	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, item.MaxImageUpload))
	if err != nil {
		writeError(w, http.StatusRequestEntityTooLarge, "error.image_too_large")
		return
	}
	img, err := s.items().AddImage(it.ID, commentID, data, s.now())
	if err != nil {
		writePictureError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, describePicture(it, img))
}

// writePictureError says why a picture was refused, each reason in words the
// person can act on.
func writePictureError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, item.ErrImageTooLarge):
		writeError(w, http.StatusRequestEntityTooLarge, "error.image_too_large")
	case errors.Is(err, picture.ErrNotAPicture), errors.Is(err, picture.ErrTooManyPixels):
		writeError(w, http.StatusBadRequest, "error.not_an_image")
	case errors.Is(err, item.ErrTooManyImages):
		writeError(w, http.StatusConflict, "error.too_many_images")
	case errors.Is(err, item.ErrImagesOff):
		writeError(w, http.StatusConflict, "error.images_off")
	case errors.Is(err, item.ErrImageQuota):
		writeError(w, http.StatusInsufficientStorage, "error.images_full")
	case errors.Is(err, item.ErrConversationClosed):
		writeError(w, http.StatusConflict, "error.conversation_closed")
	case errors.Is(err, item.ErrNotFound):
		writeError(w, http.StatusNotFound, "error.not_found")
	default:
		slog.Error("storing a picture", "error", err)
		writeError(w, http.StatusInternalServerError, "error.internal")
	}
}

// handlePicture serves one picture, full size or ?size=thumb.
//
// Asked of the server every time it is shown (no-cache, revalidated by its
// name, which never changes for the same bytes): a picture the team takes out
// after it was published has to stop being shown, and a year of immutable
// cache would keep it on every screen that had seen it.
func (s *Server) handlePicture(w http.ResponseWriter, r *http.Request) {
	it, ok := s.lookupItem(w, r)
	if !ok {
		return
	}
	data, contentType, img, err := s.items().ImageData(r.PathValue("image"), r.URL.Query().Get("size") == "thumb")
	if err != nil || img.ItemID != it.ID {
		http.NotFound(w, r)
		return
	}
	on, err := s.commentFor(it, img.CommentID)
	if err != nil || !s.pictureRightsFor(r, it).maySee(it, on) {
		// Not "it exists and you may not": somebody who may not see it learns
		// nothing about what is waiting.
		http.NotFound(w, r)
		return
	}

	etag := `"` + img.ID + `-` + r.URL.Query().Get("size") + `"`
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("ETag", etag)
	w.Header().Set("Cache-Control", "private, no-cache")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	// Opened on its own, it is a picture and nothing else: no script, no
	// frame, nothing it could load.
	w.Header().Set("Content-Security-Policy", "default-src 'none'; sandbox")
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	_, _ = w.Write(data)
}

// handleRemovePicture takes one out.
func (s *Server) handleRemovePicture(w http.ResponseWriter, r *http.Request) {
	it, ok := s.lookupItem(w, r)
	if !ok {
		return
	}
	_, _, img, err := s.items().ImageData(r.PathValue("image"), true)
	if err != nil || img.ItemID != it.ID {
		writeError(w, http.StatusNotFound, "error.not_found")
		return
	}
	on, err := s.commentFor(it, img.CommentID)
	if err != nil {
		writeError(w, http.StatusNotFound, "error.not_found")
		return
	}
	if !s.pictureRightsFor(r, it).mayRemove(on) {
		writeError(w, http.StatusForbidden, "error.forbidden")
		return
	}
	if err := s.items().RemoveImage(img.ID); err != nil {
		writePictureError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// describePicture is one picture as a page draws it: where its two sizes are,
// and its shape so the space is kept before it arrives.
func describePicture(it item.Item, img item.Image) map[string]any {
	base := "/api/items/" + it.PublicID + "/images/" + img.ID
	return map[string]any{
		"id":         img.ID,
		"comment_id": img.CommentID,
		"width":      img.Width,
		"height":     img.Height,
		"url":        base,
		"thumb":      base + "?size=thumb",
	}
}

// visiblePictures is every picture on a report this reader may see.
func (s *Server) visiblePictures(r *http.Request, it item.Item) []map[string]any {
	list, err := s.items().Images(it.ID)
	if err != nil {
		slog.Error("listing a report's pictures", "error", err)
		return []map[string]any{}
	}
	rights := s.pictureRightsFor(r, it)
	out := make([]map[string]any, 0, len(list))
	comments := map[int64]*item.Comment{}
	for _, img := range list {
		on, seen := comments[img.CommentID]
		if !seen {
			on, err = s.commentFor(it, img.CommentID)
			if err != nil {
				continue
			}
			comments[img.CommentID] = on
		}
		if rights.maySee(it, on) {
			out = append(out, describePicture(it, img))
		}
	}
	return out
}
