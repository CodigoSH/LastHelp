package landing

import (
	"bytes"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"git.codigosh.com/CodigoSH/LastHelp/internal/platform/database"
)

// ImagePrefix is where a picture on the front page is served from.
//
// One constant, read by the handler that serves them and by the markup policy
// that decides which src attributes survive. Two copies of this string would
// mean a picture that uploads and then disappears the first time the page is
// read back, which is the kind of fault nobody looks for.
const ImagePrefix = "/api/landing/images/"

// MaxImageBytes is what one picture may weigh.
//
// The browser scales and re-encodes before sending — see web/src/services/
// landingImage.ts — so this is not the limit somebody meets with a photograph
// off a phone. It is the limit that stops a handler being handed a hundred
// megabytes by something that is not the browser.
const MaxImageBytes = 2 << 20

// ErrNotAPicture is a body that is not the format this accepts.
var ErrNotAPicture = errors.New("that is not a WebP image")

// webpMagic is the first bytes of every WebP file: "RIFF", four bytes of
// length, then "WEBP".
//
// Checked rather than trusted, and checked on the bytes rather than on the
// content type the request declared: a caller chooses its own header, and the
// one thing that cannot be chosen is what the file actually is.
func isWebP(data []byte) bool {
	return len(data) >= 12 &&
		bytes.Equal(data[0:4], []byte("RIFF")) &&
		bytes.Equal(data[8:12], []byte("WEBP"))
}

// Image is a picture stored for the front page.
type Image struct {
	ID          string
	Bytes       []byte
	ContentType string
}

// StoreImage writes one picture and returns the path it is served from.
//
// The time is given rather than read: a store that reads the wall clock is a
// store whose tests have to sleep.
//
// WebP only, and that is the point rather than a restriction: it is what the
// browser encodes to before sending, it is a third the weight of the PNG or
// JPEG that went in, and a front page that loads quickly is the half of SEO
// that is actually under our control. Accepting the original as well would
// mean the promise holds only for whoever happened to use a browser that could
// encode it.
func StoreImage(db database.Execer, data []byte, now time.Time) (string, error) {
	if len(data) == 0 || len(data) > MaxImageBytes {
		return "", fmt.Errorf("%w: %d bytes", ErrNotAPicture, len(data))
	}
	if !isWebP(data) {
		return "", ErrNotAPicture
	}

	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("naming the picture: %w", err)
	}
	id := hex.EncodeToString(raw)

	_, err := db.Exec(
		`INSERT INTO landing_image (id, bytes, content_type, created_at) VALUES (?, ?, ?, ?)`,
		id, data, "image/webp", now.UTC().Format(time.RFC3339),
	)
	if err != nil {
		return "", fmt.Errorf("storing the picture: %w", err)
	}
	return ImagePrefix + id + ".webp", nil
}

// ReadImage returns one picture by the name in its path.
func ReadImage(db database.Execer, id string) (Image, error) {
	id = strings.TrimSuffix(id, ".webp")
	if !looksLikeImageID(id) {
		return Image{}, sql.ErrNoRows
	}
	var img Image
	img.ID = id
	err := db.QueryRow(
		`SELECT bytes, content_type FROM landing_image WHERE id = ?`, id,
	).Scan(&img.Bytes, &img.ContentType)
	if err != nil {
		return Image{}, err
	}
	return img, nil
}

// looksLikeImageID keeps anything that is not one of our names away from the
// query, so a path segment cannot be used to ask a different question.
func looksLikeImageID(id string) bool {
	if len(id) != 32 {
		return false
	}
	for _, c := range id {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}

// ForgetUnusedImages removes the pictures no page refers to any more.
//
// Uploading happens while somebody is writing, so a picture dropped in and then
// deleted again would sit in the database forever. Sweeping against the pages
// that are stored rather than on delete means it does not matter how a picture
// stopped being used — undone, overwritten, or the whole module removed.
//
// It takes every page that can still be shown: the published one and the draft
// being edited. A picture only the draft uses is a picture the operator is about
// to publish, and one only the published page uses is still on the screen.
func ForgetUnusedImages(db database.Execer, pages ...Page) error {
	used := map[string]bool{}
	for _, page := range pages {
		for _, id := range page.imageIDs() {
			used[id] = true
		}
	}

	rows, err := db.Query(`SELECT id FROM landing_image`)
	if err != nil {
		return fmt.Errorf("listing the pictures: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var orphans []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return err
		}
		if !used[id] {
			orphans = append(orphans, id)
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}

	for _, id := range orphans {
		if _, err := db.Exec(`DELETE FROM landing_image WHERE id = ?`, id); err != nil {
			return fmt.Errorf("forgetting a picture: %w", err)
		}
	}
	return nil
}

// imageIDsIn pulls the names out of the markup of one module.
func imageIDsIn(body string) []string {
	var found []string
	rest := body
	for {
		at := strings.Index(rest, ImagePrefix)
		if at < 0 {
			return found
		}
		rest = rest[at+len(ImagePrefix):]
		end := strings.IndexAny(rest, `".' `)
		if end < 0 {
			end = len(rest)
		}
		id := strings.TrimSuffix(rest[:end], ".webp")
		if looksLikeImageID(id) {
			found = append(found, id)
		}
		rest = rest[end:]
	}
}

// imageIDs is every stored picture a page refers to: in the words of its text
// modules, as its image modules, and as the picture it is shared with.
func (p Page) imageIDs() []string {
	var ids []string
	for _, m := range p.Modules() {
		ids = append(ids, imageIDsIn(m.Body)...)
		ids = append(ids, imageIDsIn(m.Image)...)
	}
	ids = append(ids, imageIDsIn(p.Settings.ShareImage)...)
	return ids
}
