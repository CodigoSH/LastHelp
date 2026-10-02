package item

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"testing"
	"time"
)

// photo is a real JPEG, small, so the tests exercise the whole redraw.
func photo(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 40, 30))
	for y := 0; y < 30; y++ {
		for x := 0; x < 40; x++ {
			img.Set(x, y, color.RGBA{uint8(x * 6), uint8(y * 8), 0x80, 0xff})
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func countImages(t *testing.T, s Store, itemID int64) int {
	t.Helper()
	list, err := s.Images(itemID)
	if err != nil {
		t.Fatal(err)
	}
	return len(list)
}

func TestAReportTakesFivePicturesAndAMessageThree(t *testing.T) {
	s := store(t)
	it, _ := report(t, s, Bug, "crash", "")
	for i := 0; i < MaxImagesPerReport; i++ {
		img, err := s.AddImage(it.ID, 0, photo(t), epoch)
		if err != nil {
			t.Fatalf("picture %d on the report: %v", i+1, err)
		}
		if img.Position != i {
			t.Errorf("picture %d has position %d", i+1, img.Position)
		}
	}
	if _, err := s.AddImage(it.ID, 0, photo(t), epoch); !errors.Is(err, ErrTooManyImages) {
		t.Errorf("a sixth picture on the report gave %v, want ErrTooManyImages", err)
	}

	c, err := s.Comment(it.ID, AuthorTeam, "alex", "this one?", epoch)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < MaxImagesPerComment; i++ {
		if _, err := s.AddImage(it.ID, c.ID, photo(t), epoch); err != nil {
			t.Fatalf("picture %d on the message: %v", i+1, err)
		}
	}
	if _, err := s.AddImage(it.ID, c.ID, photo(t), epoch); !errors.Is(err, ErrTooManyImages) {
		t.Errorf("a fourth picture on the message gave %v, want ErrTooManyImages", err)
	}
	if got := countImages(t, s, it.ID); got != MaxImagesPerReport+MaxImagesPerComment {
		t.Errorf("the thread holds %d pictures", got)
	}
}

func TestAThreadHasACeilingAcrossItsMessages(t *testing.T) {
	s := store(t)
	it, _ := report(t, s, Idea, "dark mode", "")
	added := 0
	for added < MaxImagesPerThread {
		c, err := s.Comment(it.ID, AuthorTeam, "alex", "more", epoch)
		if err != nil {
			t.Fatal(err)
		}
		for i := 0; i < MaxImagesPerComment && added < MaxImagesPerThread; i++ {
			if _, err := s.AddImage(it.ID, c.ID, photo(t), epoch); err != nil {
				t.Fatalf("picture %d: %v", added+1, err)
			}
			added++
		}
	}
	c, _ := s.Comment(it.ID, AuthorTeam, "alex", "one more", epoch)
	if _, err := s.AddImage(it.ID, c.ID, photo(t), epoch); !errors.Is(err, ErrTooManyImages) {
		t.Errorf("picture %d in the thread gave %v, want ErrTooManyImages", MaxImagesPerThread+1, err)
	}
}

func TestAPictureGoesOnlyWhereItMayGo(t *testing.T) {
	s := store(t)
	it, _ := report(t, s, Bug, "crash", "")
	other, _ := report(t, s, Bug, "freeze", "")
	elsewhere, _ := s.Comment(other.ID, AuthorTeam, "alex", "on another report", epoch)

	if _, err := s.AddImage(it.ID, elsewhere.ID, photo(t), epoch); !errors.Is(err, ErrNotFound) {
		t.Errorf("a picture for a message on another report gave %v, want ErrNotFound", err)
	}
	if _, err := s.AddImage(it.ID, 0, []byte("not a picture"), epoch); err == nil {
		t.Error("something that is not a picture was stored")
	}
	if _, err := s.AddImage(it.ID, 0, make([]byte, MaxImageUpload+1), epoch); !errors.Is(err, ErrImageTooLarge) {
		t.Errorf("a picture over the upload size gave %v", err)
	}

	if _, err := s.DB.Exec(`UPDATE repository SET accepts_images = 0 WHERE id = 1`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AddImage(it.ID, 0, photo(t), epoch); !errors.Is(err, ErrImagesOff) {
		t.Errorf("a board with pictures off gave %v, want ErrImagesOff", err)
	}
}

func TestAnAnsweredReportTakesNoPictures(t *testing.T) {
	s := store(t)
	it, _ := report(t, s, Bug, "crash", "")
	if err := s.SetStatus(it.ID, StatusClosed, epoch); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AddImage(it.ID, 0, photo(t), epoch); !errors.Is(err, ErrConversationClosed) {
		t.Errorf("a picture on a closed report gave %v, want ErrConversationClosed", err)
	}
}

// TestAClosedReportKeepsItsPicturesForTheGrace: thirty days to reopen it, and
// then they go. Reopened in time, nothing is lost.
func TestAClosedReportKeepsItsPicturesForTheGrace(t *testing.T) {
	s := store(t)
	closed, _ := report(t, s, Bug, "crash", "")
	reopened, _ := report(t, s, Bug, "freeze", "")
	open, _ := report(t, s, Idea, "dark mode", "")
	for _, it := range []Item{closed, reopened, open} {
		if _, err := s.AddImage(it.ID, 0, photo(t), epoch); err != nil {
			t.Fatal(err)
		}
	}
	for _, it := range []Item{closed, reopened} {
		if err := s.SetStatus(it.ID, StatusClosed, epoch); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.SetStatus(reopened.ID, StatusConfirmed, epoch.Add(ImageGrace-time.Hour)); err != nil {
		t.Fatal(err)
	}

	if n, err := s.PurgeImages(epoch.Add(ImageGrace - time.Minute)); err != nil || n != 0 {
		t.Fatalf("inside the grace %d pictures were swept (%v)", n, err)
	}
	if n, err := s.PurgeImages(epoch.Add(ImageGrace + time.Minute)); err != nil || n != 1 {
		t.Fatalf("past the grace %d pictures were swept (%v), want the closed report's one", n, err)
	}
	if countImages(t, s, closed.ID) != 0 {
		t.Error("the closed report still has its picture")
	}
	if countImages(t, s, reopened.ID) != 1 || countImages(t, s, open.ID) != 1 {
		t.Error("a report that is open lost its picture")
	}
}

func TestATurnedDownReportOrMessageLosesItsPicturesAtOnce(t *testing.T) {
	s := store(t)
	rejected, _ := report(t, s, Bug, "crash", "")
	spam, _ := report(t, s, Bug, "buy now", "")
	kept, _ := report(t, s, Idea, "dark mode", "")
	for _, it := range []Item{rejected, spam, kept} {
		if _, err := s.AddImage(it.ID, 0, photo(t), epoch); err != nil {
			t.Fatal(err)
		}
	}
	c, _ := s.Comment(kept.ID, AuthorReporter, "", "here is mine", epoch)
	if _, err := s.AddImage(kept.ID, c.ID, photo(t), epoch); err != nil {
		t.Fatal(err)
	}

	if err := s.Reject(rejected.ID, epoch); err != nil {
		t.Fatal(err)
	}
	if err := s.MarkSpam(spam.ID, epoch); err != nil {
		t.Fatal(err)
	}
	if err := s.RejectComment(c.ID); err != nil {
		t.Fatal(err)
	}
	if countImages(t, s, rejected.ID) != 0 || countImages(t, s, spam.ID) != 0 {
		t.Error("a turned-down report kept its pictures until the sweep")
	}
	if got := countImages(t, s, kept.ID); got != 1 {
		t.Errorf("the report keeps %d pictures; its own should stay and the rejected message's go", got)
	}
}

func TestMergingMovesTheDuplicatesPictures(t *testing.T) {
	s := store(t)
	survivor, _ := report(t, s, Bug, "crash on import", "")
	duplicate, _ := report(t, s, Bug, "import crashes", "")
	if _, err := s.AddImage(survivor.ID, 0, photo(t), epoch); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if _, err := s.AddImage(duplicate.ID, 0, photo(t), epoch); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.Merge(duplicate.ID, survivor.ID, epoch); err != nil {
		t.Fatal(err)
	}
	if _, err := s.PurgeImages(epoch); err != nil {
		t.Fatal(err)
	}
	list, _ := s.Images(survivor.ID)
	if len(list) != 3 {
		t.Fatalf("the survivor holds %d pictures after the merge and the sweep, want 3", len(list))
	}
	for i, img := range list {
		if img.Position != i {
			t.Errorf("after the merge, picture %d has position %d", i, img.Position)
		}
	}
}

func TestDeletingAReportOrAMessageDeletesItsPictures(t *testing.T) {
	s := store(t)
	it, _ := report(t, s, Bug, "crash", "")
	c, _ := s.Comment(it.ID, AuthorTeam, "alex", "look", epoch)
	if _, err := s.AddImage(it.ID, c.ID, photo(t), epoch); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AddImage(it.ID, 0, photo(t), epoch); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteComment(c.ID); err != nil {
		t.Fatal(err)
	}
	if got := countImages(t, s, it.ID); got != 1 {
		t.Errorf("after deleting the message the report holds %d pictures, want 1", got)
	}
	if err := s.Delete(it.ID); err != nil {
		t.Fatal(err)
	}
	var left int
	_ = s.DB.QueryRow(`SELECT count(*) FROM item_image`).Scan(&left)
	if left != 0 {
		t.Errorf("%d pictures outlived the report they belonged to", left)
	}
}

func TestBothSizesAreServedAndARemovedPictureIsGone(t *testing.T) {
	s := store(t)
	it, _ := report(t, s, Bug, "crash", "")
	img, err := s.AddImage(it.ID, 0, photo(t), epoch)
	if err != nil {
		t.Fatal(err)
	}
	for _, thumb := range []bool{false, true} {
		data, kind, got, err := s.ImageData(img.ID, thumb)
		if err != nil || len(data) == 0 || kind != "image/jpeg" || got.ItemID != it.ID {
			t.Errorf("thumb=%v: %d bytes, %q, item %d, %v", thumb, len(data), kind, got.ItemID, err)
		}
	}
	if err := s.RemoveImage(img.ID); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := s.ImageData(img.ID, false); !errors.Is(err, ErrNotFound) {
		t.Errorf("a removed picture gave %v, want ErrNotFound", err)
	}
}

// TestAFullInstallationRefusesWithAReason: what every picture weighs together
// has a ceiling, and reaching it says so rather than filling the disk. The
// space already used is stated by one row that claims all of it.
func TestAFullInstallationRefusesWithAReason(t *testing.T) {
	s := store(t)
	it, _ := report(t, s, Bug, "crash", "")
	other, _ := report(t, s, Bug, "freeze", "")
	if _, err := s.DB.Exec(`INSERT INTO item_image
		(id, item_id, comment_id, position, full, full_type, width, height, thumb, thumb_type, bytes, created_at)
		VALUES ('ffffffffffffffffffffffffffffffff', ?, NULL, 0, x'00', 'image/jpeg', 1, 1, x'00', 'image/jpeg', ?, 0)`,
		other.ID, ImageQuota-100); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AddImage(it.ID, 0, photo(t), epoch); !errors.Is(err, ErrImageQuota) {
		t.Errorf("a picture past the installation's space gave %v, want ErrImageQuota", err)
	}
}
