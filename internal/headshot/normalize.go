package headshot

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	_ "image/png" // decode PNG uploads

	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp" // decode WebP uploads (Safari and Android share them)
)

// Bounds on what may be uploaded and what is stored.
const (
	// maxUploadBytes caps the request body. A phone photo is 2–5 MiB.
	maxUploadBytes = 8 << 20
	// maxSourcePixels caps the DECODED size, checked on the header before any decode:
	// a decompression bomb is small on disk and enormous in memory. 50 MP is past any
	// consumer camera.
	maxSourcePixels = 50_000_000
	// outputEdge is the stored square's side in pixels. At the ~1.4in frame the templates
	// print, 512 px is over 300 dpi — enough for print, small enough to keep the object
	// around 40 KB.
	outputEdge  = 512
	jpegQuality = 85
)

var (
	// ErrUnsupportedImage is returned when the payload does not decode as a supported
	// image. The handler maps it to 400 — nothing is stored.
	ErrUnsupportedImage = errors.New("headshot: unsupported image")
	// ErrTooLarge is returned when the upload exceeds the byte cap or would decode to
	// more pixels than allowed. Also a 400.
	ErrTooLarge = errors.New("headshot: image is too large")
)

// Normalize turns an arbitrary upload into the one shape that gets stored: a centred
// square, outputEdge on a side, upright per the file's declared orientation, encoded as
// JPEG. It never returns the caller's bytes unchanged — re-encoding is what guarantees
// the stored object is an image and nothing else (any trailing payload, EXIF GPS, or
// embedded thumbnail is dropped along with everything the decoder ignored).
func Normalize(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("%w: empty upload", ErrUnsupportedImage)
	}
	if len(data) > maxUploadBytes {
		return nil, fmt.Errorf("%w: %d bytes exceeds the %d byte limit", ErrTooLarge, len(data), maxUploadBytes)
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnsupportedImage, err)
	}
	if cfg.Width <= 0 || cfg.Height <= 0 {
		return nil, fmt.Errorf("%w: empty dimensions", ErrUnsupportedImage)
	}
	if cfg.Width*cfg.Height > maxSourcePixels {
		return nil, fmt.Errorf("%w: %dx%d exceeds %d pixels", ErrTooLarge, cfg.Width, cfg.Height, maxSourcePixels)
	}

	src, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnsupportedImage, err)
	}
	square := scale(crop(orient(src, readOrientation(data))))

	var out bytes.Buffer
	if err := jpeg.Encode(&out, square, &jpeg.Options{Quality: jpegQuality}); err != nil {
		return nil, fmt.Errorf("headshot: encode: %w", err)
	}
	return out.Bytes(), nil
}

// orient rotates the image so it is upright, per the EXIF orientation its file declared.
// Only the three rotations a camera produces are applied; the mirrored orientations
// (2, 4, 5, 7) come from a deliberate flip, and silently un-mirroring someone's photo is
// worse than leaving it as they see it everywhere else.
func orient(src image.Image, orientation int) image.Image {
	switch orientation {
	case orientationRotate180:
		return rotate(src, func(x, y, w, h int) (int, int) { return w - 1 - x, h - 1 - y })
	case orientationRotate90:
		return rotate(src, func(x, y, w, h int) (int, int) { return h - 1 - y, x })
	case orientationRotate270:
		return rotate(src, func(x, y, w, h int) (int, int) { return y, w - 1 - x })
	default:
		return src
	}
}

// rotate rebuilds the image by mapping each source pixel through at, which reports the
// destination coordinate for a source (x, y) in a w×h image. The destination is square
// for the quarter turns and same-shaped for the half turn, so its bounds come from the
// mapped corners rather than being assumed.
func rotate(src image.Image, at func(x, y, w, h int) (int, int)) image.Image {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	maxX, maxY := at(w-1, h-1, w, h)
	if x, y := at(0, 0, w, h); x > maxX || y > maxY {
		maxX, maxY = max(maxX, x), max(maxY, y)
	}
	dst := image.NewRGBA(image.Rect(0, 0, maxX+1, maxY+1))
	for y := range h {
		for x := range w {
			dx, dy := at(x, y, w, h)
			dst.Set(dx, dy, src.At(b.Min.X+x, b.Min.Y+y))
		}
	}
	return dst
}

// crop returns the largest centred square of the image. A portrait photo therefore keeps
// its middle band rather than its top, which is where a face usually sits in a headshot
// shot at arm's length.
func crop(src image.Image) image.Image {
	b := src.Bounds()
	edge := min(b.Dx(), b.Dy())
	r := image.Rect(0, 0, edge, edge).Add(image.Point{
		X: b.Min.X + (b.Dx()-edge)/2,
		Y: b.Min.Y + (b.Dy()-edge)/2,
	})
	if sub, ok := src.(interface {
		SubImage(image.Rectangle) image.Image
	}); ok {
		return sub.SubImage(r)
	}
	dst := image.NewRGBA(image.Rect(0, 0, edge, edge))
	draw.Draw(dst, dst.Bounds(), src, r.Min, draw.Src)
	return dst
}

// scale resamples the square to outputEdge. CatmullRom is the sharpest of the kernels
// x/image offers and the cost is irrelevant on a 512 px target; the standard library has
// no resampling scaler at all.
func scale(src image.Image) image.Image {
	dst := image.NewRGBA(image.Rect(0, 0, outputEdge, outputEdge))
	draw.CatmullRom.Scale(dst, dst.Bounds(), src, src.Bounds(), draw.Src, nil)
	return dst
}
