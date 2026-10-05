package item

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"git.codigosh.com/CodigoSH/LastHelp/internal/core/picture"
)

// What one report may carry, and what the installation may hold.
//
// A picture is the easiest thing a stranger can send that costs the operator
// something — disk, and the attention of whoever has to look at it before it
// is shown — so every count has a ceiling and the last one is the machine's.
const (
	// MaxImagesPerReport is what the report itself may carry.
	MaxImagesPerReport = 5
	// MaxImagesPerComment is what one reply or follow-up may carry.
	MaxImagesPerComment = 3
	// MaxImagesPerThread bounds a report and every message on it together, so
	// a long conversation cannot become a gallery.
	MaxImagesPerThread = 20
	// MaxImageUpload is what one picture may weigh as it arrives. The browser
	// scales and re-encodes before sending, so a real picture is a fraction of
	// this; the bound is for whatever is not the browser.
	MaxImageUpload = 2 << 20
	// ImageQuota is what every picture in the installation may weigh together,
	// as stored. Reaching it refuses new pictures with a reason the operator
	// can act on, instead of filling the disk the database lives on.
	ImageQuota = 512 << 20
	// ImageGrace is how long a closed report keeps its pictures: long enough to
	// reopen one closed by mistake, short enough that nothing stays for ever.
	ImageGrace = 30 * 24 * time.Hour

	// imageSide is the full picture's longest edge; thumbSide the small one's.
	imageSide = 1600
	thumbSide = 320
)

var (
	// ErrImagesOff is a board whose team turned pictures off.
	ErrImagesOff = errors.New("this board does not take pictures")
	// ErrTooManyImages is a message or a thread already at its ceiling.
	ErrTooManyImages = errors.New("no more pictures fit here")
	// ErrImageQuota is the installation's store of pictures being full.
	ErrImageQuota = errors.New("the space for pictures is full")
	// ErrImageTooLarge is a picture over MaxImageUpload as it arrived.
	ErrImageTooLarge = errors.New("that picture is too large")
)

// Image is one picture, without its bytes: what a page needs to draw it.
type Image struct {
	ID        string
	ItemID    int64
	CommentID int64 // 0 for a picture on the report itself
	Position  int
	Width     int
	Height    int
	Bytes     int
	CreatedAt time.Time
}

// AddImage draws a picture again and stores it on a report, or on one message
// on it when commentID is not 0.
//
// It goes through the same door as a vote and a comment: a report that is
// answered, turned down or folded into another takes nothing more. On top of
// that, the board has to take pictures, the message and the thread have to
// have room, and so does the installation. `author` is who the picture is from,
// which is who the message it hangs on is from, or the reporter for the report.
func (s Store) AddImage(itemID, commentID int64, author Author, data []byte, now time.Time) (Image, error) {
	if len(data) > MaxImageUpload {
		return Image{}, ErrImageTooLarge
	}
	if err := s.mayWriteOn(itemID, author, now); err != nil {
		return Image{}, err
	}

	var accepts bool
	if err := s.DB.QueryRow(`SELECT r.accepts_images FROM item i JOIN repository r ON r.id = i.repo_id
		WHERE i.id = ?`, itemID).Scan(&accepts); err != nil {
		return Image{}, fmt.Errorf("reading the board: %w", err)
	}
	if !accepts {
		return Image{}, ErrImagesOff
	}

	if commentID != 0 {
		var on int64
		switch err := s.DB.QueryRow(`SELECT item_id FROM comment WHERE id = ?`, commentID).Scan(&on); {
		case errors.Is(err, sql.ErrNoRows):
			return Image{}, ErrNotFound
		case err != nil:
			return Image{}, fmt.Errorf("reading the message: %w", err)
		case on != itemID:
			// A picture for a message on another report is not one this
			// report can hold.
			return Image{}, ErrNotFound
		}
	}

	// Drawn before anything is written: it is the slow part, and a picture
	// that does not decode is refused without touching the table.
	drawn, err := picture.Redraw(data, imageSide, thumbSide)
	if err != nil {
		return Image{}, err
	}
	stored := len(drawn.Full.Bytes) + len(drawn.Thumb.Bytes)

	limit := MaxImagesPerReport
	if commentID != 0 {
		limit = MaxImagesPerComment
	}
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return Image{}, fmt.Errorf("naming the picture: %w", err)
	}
	img := Image{
		ID: hex.EncodeToString(raw), ItemID: itemID, CommentID: commentID,
		Width: drawn.Full.Width, Height: drawn.Full.Height, Bytes: stored, CreatedAt: now,
	}
	var comment any
	if commentID != 0 {
		comment = commentID
	}

	// The ceilings are checked by the statement that writes, not before it:
	// one statement is atomic, so two pictures sent at the same moment cannot
	// both find the last free place.
	const onMessage = `(SELECT count(*) FROM item_image WHERE item_id = ? AND COALESCE(comment_id, 0) = ?)`
	res, err := s.DB.Exec(`INSERT INTO item_image
		(id, item_id, comment_id, position, full, full_type, width, height, thumb, thumb_type, bytes, created_at)
		SELECT ?, ?, ?, `+onMessage+`, ?, ?, ?, ?, ?, ?, ?, ?
		WHERE `+onMessage+` < ?
		  AND (SELECT count(*) FROM item_image WHERE item_id = ?) < ?
		  AND (SELECT COALESCE(sum(bytes), 0) FROM item_image) + ? <= ?`,
		img.ID, itemID, comment, itemID, commentID,
		drawn.Full.Bytes, string(drawn.Full.Format), img.Width, img.Height,
		drawn.Thumb.Bytes, string(drawn.Thumb.Format), stored, now.Unix(),
		itemID, commentID, limit,
		itemID, MaxImagesPerThread,
		stored, ImageQuota)
	if err != nil {
		return Image{}, fmt.Errorf("storing the picture: %w", err)
	}
	if n, err := res.RowsAffected(); err != nil {
		return Image{}, fmt.Errorf("storing the picture: %w", err)
	} else if n == 0 {
		// Refused by one of the ceilings. Which one is asked afterwards, only
		// to say it: the decision was the statement's.
		var used int64
		_ = s.DB.QueryRow(`SELECT COALESCE(sum(bytes), 0) FROM item_image`).Scan(&used)
		if used+int64(stored) > ImageQuota {
			return Image{}, ErrImageQuota
		}
		return Image{}, ErrTooManyImages
	}
	if err := s.DB.QueryRow(`SELECT position FROM item_image WHERE id = ?`, img.ID).Scan(&img.Position); err != nil {
		return Image{}, fmt.Errorf("reading the picture back: %w", err)
	}
	return img, nil
}

// Images lists the pictures on a report and on every message on it: the
// report's first, then each message's, in the order they were added.
func (s Store) Images(itemID int64) ([]Image, error) {
	rows, err := s.DB.Query(`SELECT id, item_id, COALESCE(comment_id, 0), position, width, height, bytes, created_at
		FROM item_image WHERE item_id = ? ORDER BY COALESCE(comment_id, 0), position, created_at`, itemID)
	if err != nil {
		return nil, fmt.Errorf("listing pictures: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []Image
	for rows.Next() {
		var img Image
		var created int64
		if err := rows.Scan(&img.ID, &img.ItemID, &img.CommentID, &img.Position,
			&img.Width, &img.Height, &img.Bytes, &created); err != nil {
			return nil, err
		}
		img.CreatedAt = time.Unix(created, 0)
		out = append(out, img)
	}
	return out, rows.Err()
}

// ImageData is a picture's bytes at one of its two sizes, with what it belongs
// to so the caller can decide whether this reader may see it.
func (s Store) ImageData(id string, thumb bool) (data []byte, contentType string, img Image, err error) {
	column, typeColumn := "full", "full_type"
	if thumb {
		column, typeColumn = "thumb", "thumb_type"
	}
	var created int64
	err = s.DB.QueryRow(`SELECT `+column+`, `+typeColumn+`, id, item_id, COALESCE(comment_id, 0),
		position, width, height, bytes, created_at FROM item_image WHERE id = ?`, id).
		Scan(&data, &contentType, &img.ID, &img.ItemID, &img.CommentID,
			&img.Position, &img.Width, &img.Height, &img.Bytes, &created)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, "", Image{}, ErrNotFound
	}
	if err != nil {
		return nil, "", Image{}, fmt.Errorf("reading the picture: %w", err)
	}
	img.CreatedAt = time.Unix(created, 0)
	return data, contentType, img, nil
}

// RemoveImage deletes one picture: the team taking out the one that should not
// be shown, or the reporter taking back their own.
func (s Store) RemoveImage(id string) error {
	res, err := s.DB.Exec(`DELETE FROM item_image WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("removing the picture: %w", err)
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		return ErrNotFound
	}
	return nil
}

// PurgeImages deletes the pictures nothing should be showing any more:
//
//   - a closed report's, once ImageGrace has passed since it closed;
//   - a report turned down or marked spam, at once — a merged duplicate is one
//     of those, and Merge moves its pictures before it gets here;
//   - a message turned down or marked spam, at once.
//
// Run hourly. Turning a report down also deletes its pictures straight away;
// this is what makes sure nothing is missed whichever way something got there.
func (s Store) PurgeImages(now time.Time) (int64, error) {
	res, err := s.DB.Exec(`DELETE FROM item_image WHERE
		item_id IN (SELECT id FROM item WHERE
			(closed_at IS NOT NULL AND closed_at < ?)
			OR moderation_state IN (?, ?))
		OR comment_id IN (SELECT id FROM comment WHERE moderation_state IN (?, ?))`,
		now.Add(-ImageGrace).Unix(), string(Rejected), string(Spam), string(Rejected), string(Spam))
	if err != nil {
		return 0, fmt.Errorf("purging pictures: %w", err)
	}
	return res.RowsAffected()
}

// forgetImagesOf deletes what one report carries, when it is turned down.
func (s Store) forgetImagesOf(itemID int64) error {
	if _, err := s.DB.Exec(`DELETE FROM item_image WHERE item_id = ?`, itemID); err != nil {
		return fmt.Errorf("removing the report's pictures: %w", err)
	}
	return nil
}
