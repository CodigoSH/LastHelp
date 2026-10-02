// Package picture is what the server does with an image somebody sent: say
// what it really is, refuse what is too large to open, and, when the sender is
// a stranger, draw it again from its pixels so that nothing but the picture
// survives.
//
// Three formats, recognised by their bytes and never by what the request
// claims: WebP, JPEG and PNG. The browser prefers WebP and falls back to JPEG
// when it cannot write one — Safari asked for WebP returns a PNG without a
// word, measured on Safari 27 — so all three arrive in practice.
//
// Encoding is JPEG for an opaque picture and PNG for one with transparency.
// The server cannot write WebP without a C library, and a single static binary
// is worth more than the third of the weight WebP would save.
package picture

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"

	"golang.org/x/image/draw"
	"golang.org/x/image/webp"
)

// MaxPixels is the most an image may declare before it is opened.
//
// A file of a few hundred kilobytes can announce sixteen thousand pixels a
// side, and decoding it asks for a gigabyte. The check reads the header only,
// so a picture that would take the server down — or the browser of whoever
// opens it later — is refused before a single pixel is allocated. Forty
// million is a little over a 48-megapixel phone sensor.
const MaxPixels = 40_000_000

var (
	// ErrNotAPicture is a body that is none of the formats this reads.
	ErrNotAPicture = errors.New("that is not a WebP, JPEG or PNG image")
	// ErrTooManyPixels is an image whose header asks for more than MaxPixels.
	ErrTooManyPixels = errors.New("that image is larger than this server opens")
)

// Format is a kind of file this package recognises.
type Format string

const (
	WebP Format = "image/webp"
	JPEG Format = "image/jpeg"
	PNG  Format = "image/png"
)

// Sniff names the format from the first bytes, or returns ErrNotAPicture.
func Sniff(data []byte) (Format, error) {
	switch {
	case len(data) >= 12 && bytes.Equal(data[0:4], []byte("RIFF")) && bytes.Equal(data[8:12], []byte("WEBP")):
		return WebP, nil
	case len(data) >= 3 && data[0] == 0xFF && data[1] == 0xD8 && data[2] == 0xFF:
		return JPEG, nil
	case len(data) >= 8 && bytes.Equal(data[0:8], []byte("\x89PNG\r\n\x1a\n")):
		return PNG, nil
	}
	return "", ErrNotAPicture
}

// Info is what the header says, read without decoding the picture.
type Info struct {
	Format        Format
	Width, Height int
}

// Inspect reads the format and the size, and refuses an image too large to
// open. It decodes nothing past the header.
func Inspect(data []byte) (Info, error) {
	format, err := Sniff(data)
	if err != nil {
		return Info{}, err
	}
	cfg, err := decodeConfig(format, data)
	if err != nil {
		return Info{}, fmt.Errorf("%w: %v", ErrNotAPicture, err)
	}
	if cfg.Width <= 0 || cfg.Height <= 0 {
		return Info{}, ErrNotAPicture
	}
	if int64(cfg.Width)*int64(cfg.Height) > MaxPixels {
		return Info{}, ErrTooManyPixels
	}
	return Info{Format: format, Width: cfg.Width, Height: cfg.Height}, nil
}

func decodeConfig(format Format, data []byte) (image.Config, error) {
	r := bytes.NewReader(data)
	switch format {
	case WebP:
		return webp.DecodeConfig(r)
	case JPEG:
		return jpeg.DecodeConfig(r)
	default:
		return png.DecodeConfig(r)
	}
}

func decode(format Format, data []byte) (image.Image, error) {
	r := bytes.NewReader(data)
	switch format {
	case WebP:
		return webp.Decode(r)
	case JPEG:
		return jpeg.Decode(r)
	default:
		return png.Decode(r)
	}
}

// Encoded is a picture written again by this server.
type Encoded struct {
	Bytes         []byte
	Format        Format
	Width, Height int
}

// Redrawn is a picture at two sizes: the one somebody opens, and the small one
// a list or a report card shows.
type Redrawn struct {
	Full, Thumb Encoded
}

// jpegQuality is where a screenshot's text stays crisp and a photograph stays
// a fraction of what it came in as.
const jpegQuality = 85

// Redraw decodes a picture, scales it to fit within side and thumbSide, and
// encodes both again.
//
// Writing from the pixels is the point: whatever else the file carried — the
// place a photograph was taken, the camera, a second payload appended after
// the image data — is not pixels, so it does not come out the other side. And
// a file that only looks like a picture fails to decode and is refused.
func Redraw(data []byte, side, thumbSide int) (Redrawn, error) {
	info, err := Inspect(data)
	if err != nil {
		return Redrawn{}, err
	}
	src, err := decode(info.Format, data)
	if err != nil {
		return Redrawn{}, fmt.Errorf("%w: %v", ErrNotAPicture, err)
	}

	opaque := isOpaque(src)
	full, err := encode(scaled(src, side), opaque)
	if err != nil {
		return Redrawn{}, err
	}
	thumb, err := encode(scaled(src, thumbSide), opaque)
	if err != nil {
		return Redrawn{}, err
	}
	return Redrawn{Full: full, Thumb: thumb}, nil
}

// isOpaque reports whether every pixel is fully opaque. JPEG always is; a PNG
// or WebP is asked, because a logo with a transparent background re-encoded
// as JPEG comes back with a black square behind it.
func isOpaque(img image.Image) bool {
	if o, ok := img.(interface{ Opaque() bool }); ok {
		return o.Opaque()
	}
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if _, _, _, a := img.At(x, y).RGBA(); a != 0xffff {
				return false
			}
		}
	}
	return true
}

// scaled fits a picture within side pixels on its longest edge, keeping its
// shape. A picture already smaller is drawn at its own size: enlarging it adds
// bytes and no detail.
func scaled(src image.Image, side int) image.Image {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	if longest := max(w, h); longest > side {
		w = max(1, w*side/longest)
		h = max(1, h*side/longest)
	}
	dst := image.NewNRGBA(image.Rect(0, 0, w, h))
	draw.CatmullRom.Scale(dst, dst.Bounds(), src, b, draw.Src, nil)
	return dst
}

func encode(img image.Image, opaque bool) (Encoded, error) {
	var buf bytes.Buffer
	format := PNG
	var err error
	if opaque {
		format = JPEG
		err = jpeg.Encode(&buf, img, &jpeg.Options{Quality: jpegQuality})
	} else {
		err = (&png.Encoder{CompressionLevel: png.BestCompression}).Encode(&buf, img)
	}
	if err != nil {
		return Encoded{}, fmt.Errorf("encoding the picture: %w", err)
	}
	b := img.Bounds()
	return Encoded{Bytes: buf.Bytes(), Format: format, Width: b.Dx(), Height: b.Dy()}, nil
}
