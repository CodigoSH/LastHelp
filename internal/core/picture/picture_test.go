package picture

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"testing"
)

// The one-pixel lossless WebP that feature detection has used for a decade: a
// real file from a real encoder, which is the point — a fixture written to
// suit the decoder proves nothing about the decoder.
const onePixelWebP = "UklGRhoAAABXRUJQVlA4TA0AAAAvAAAAEAcQERGIiP4HAA=="

func solid(w, h int, c color.Color) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, c)
		}
	}
	return img
}

func pngOf(t *testing.T, img image.Image) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func jpegOf(t *testing.T, img image.Image) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestTheFormatIsReadFromTheBytes(t *testing.T) {
	webpBytes, _ := base64.StdEncoding.DecodeString(onePixelWebP)
	orange := color.NRGBA{0xe8, 0x52, 0x0e, 0xff}
	cases := map[Format][]byte{
		WebP: webpBytes,
		JPEG: jpegOf(t, solid(4, 4, orange)),
		PNG:  pngOf(t, solid(4, 4, orange)),
	}
	for want, data := range cases {
		got, err := Sniff(data)
		if err != nil || got != want {
			t.Errorf("Sniff gave %q, %v for a %s", got, err, want)
		}
	}
	for _, junk := range [][]byte{nil, []byte("GIF89a"), []byte("<svg xmlns='http://www.w3.org/2000/svg'/>"), []byte("RIFF....AVI ")} {
		if _, err := Sniff(junk); !errors.Is(err, ErrNotAPicture) {
			t.Errorf("Sniff(%q) gave %v, want ErrNotAPicture", junk, err)
		}
	}
}

// TestAHugeImageIsRefusedBeforeItIsOpened: a few bytes can declare a picture
// that needs gigabytes to decode. The header is rewritten to say 20000×20000
// with a valid checksum, so the only thing that can refuse it is the size.
func TestAHugeImageIsRefusedBeforeItIsOpened(t *testing.T) {
	data := pngOf(t, solid(2, 2, color.White))
	// IHDR starts after the 8-byte signature: length (4), type (4), then width
	// and height, then the CRC over type and data.
	binary.BigEndian.PutUint32(data[16:20], 20000)
	binary.BigEndian.PutUint32(data[20:24], 20000)
	binary.BigEndian.PutUint32(data[29:33], crc32.ChecksumIEEE(data[12:29]))

	if _, err := Inspect(data); !errors.Is(err, ErrTooManyPixels) {
		t.Fatalf("a 20000×20000 header gave %v, want ErrTooManyPixels", err)
	}
	if _, err := Redraw(data, 1600, 320); !errors.Is(err, ErrTooManyPixels) {
		t.Errorf("Redraw went past a 20000×20000 header: %v", err)
	}
}

func TestSomethingThatOnlyLooksLikeAPictureIsRefused(t *testing.T) {
	fake := append([]byte("\x89PNG\r\n\x1a\n"), []byte("not really a png at all")...)
	if _, err := Redraw(fake, 1600, 320); !errors.Is(err, ErrNotAPicture) {
		t.Errorf("a PNG signature over junk gave %v, want ErrNotAPicture", err)
	}
}

// TestOnlyThePixelsSurvive: what a stranger's file carried besides the picture
// is not in what the server keeps — the metadata a phone writes, and anything
// appended after the image data.
func TestOnlyThePixelsSurvive(t *testing.T) {
	data := jpegOf(t, solid(64, 48, color.NRGBA{0x20, 0x60, 0xa0, 0xff}))
	// An APP1 segment straight after the start-of-image marker, where a camera
	// writes its EXIF block, and a payload after the end of the image.
	secret := []byte("Exif\x00\x00GPS 40.4168N 3.7038W")
	app1 := append([]byte{0xFF, 0xE1, 0, byte(len(secret) + 2)}, secret...)
	data = append(append(append([]byte{}, data[:2]...), app1...), data[2:]...)
	data = append(data, []byte("<script>alert(1)</script>")...)

	if _, err := Inspect(data); err != nil {
		t.Fatalf("the doctored JPEG did not read as one: %v", err)
	}
	out, err := Redraw(data, 1600, 320)
	if err != nil {
		t.Fatal(err)
	}
	for _, kept := range [][]byte{out.Full.Bytes, out.Thumb.Bytes} {
		for _, leak := range []string{"GPS", "Exif", "<script>"} {
			if bytes.Contains(kept, []byte(leak)) {
				t.Errorf("%q survived the redraw", leak)
			}
		}
	}
}

func TestTransparencyIsKeptAndOpaqueGoesSmall(t *testing.T) {
	logo := solid(40, 40, color.NRGBA{0xe8, 0x52, 0x0e, 0xff})
	logo.Set(0, 0, color.NRGBA{0, 0, 0, 0})
	out, err := Redraw(pngOf(t, logo), 1600, 320)
	if err != nil {
		t.Fatal(err)
	}
	if out.Full.Format != PNG || out.Thumb.Format != PNG {
		t.Errorf("a picture with a transparent pixel came out as %s/%s; as JPEG it gets a black square",
			out.Full.Format, out.Thumb.Format)
	}

	// An opaque PNG is a screenshot or a photo saved the long way: JPEG.
	out, err = Redraw(pngOf(t, solid(40, 40, color.White)), 1600, 320)
	if err != nil {
		t.Fatal(err)
	}
	if out.Full.Format != JPEG {
		t.Errorf("an opaque PNG came out as %s, want JPEG", out.Full.Format)
	}
}

func TestBothSizesFitAndKeepTheShape(t *testing.T) {
	out, err := Redraw(jpegOf(t, solid(3000, 1000, color.Gray{0x80})), 1600, 320)
	if err != nil {
		t.Fatal(err)
	}
	if out.Full.Width != 1600 || out.Full.Height != 533 {
		t.Errorf("full is %d×%d, want 1600×533", out.Full.Width, out.Full.Height)
	}
	if out.Thumb.Width != 320 || out.Thumb.Height != 106 {
		t.Errorf("thumb is %d×%d, want 320×106", out.Thumb.Width, out.Thumb.Height)
	}
	if got, err := Inspect(out.Full.Bytes); err != nil || got.Width != 1600 {
		t.Errorf("what was written does not read back: %+v %v", got, err)
	}

	// Smaller than the box: left at its own size.
	out, err = Redraw(jpegOf(t, solid(200, 100, color.Gray{0x80})), 1600, 320)
	if err != nil {
		t.Fatal(err)
	}
	if out.Full.Width != 200 || out.Thumb.Width != 200 {
		t.Errorf("a 200px picture was resized to %d / %d", out.Full.Width, out.Thumb.Width)
	}
}

func TestAWebPIsRead(t *testing.T) {
	data, _ := base64.StdEncoding.DecodeString(onePixelWebP)
	info, err := Inspect(data)
	if err != nil || info.Format != WebP || info.Width != 1 || info.Height != 1 {
		t.Fatalf("Inspect gave %+v %v", info, err)
	}
	if _, err := Redraw(data, 1600, 320); err != nil {
		t.Errorf("a real WebP did not redraw: %v", err)
	}
}
