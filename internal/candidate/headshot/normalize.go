package headshot

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	_ "image/png" // decode PNG uploads

	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp" // decode WebP uploads (Safari and Android share them)
)

// Bounds on what may be uploaded and what is stored.
const (
	// maxUploadBytes caps the request body. A phone photo is 2–5 MiB.
	maxUploadBytes = 8 << 20
	// maxSourcePixels caps the DECODED size, checked on the header before any decode: a
	// decompression bomb is small on disk and huge in memory, which the byte cap does not
	// bound. It limits what one request may cost the API; the output is 512 px, so more
	// pixels buy nothing.
	maxSourcePixels = 30_000_000
	// outputEdge is the stored square's side in pixels: over 300 dpi at the ~1.4in print frame,
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

// Normalize turns an arbitrary upload into the one stored shape: a centred square, outputEdge
// on a side, upright per the declared orientation, encoded as JPEG. It never returns the
// caller's bytes unchanged: re-encoding guarantees the object is an image and nothing else
// (trailing payload, EXIF GPS and embedded thumbnails are dropped).
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
	// Resize BEFORE rotating. The square commutes with a quarter turn, but rotating the source
	// would copy a second full-size buffer (seconds of CPU on a 30 MP upload); rotating the
	// 512 px square is microseconds.
	square := orient(scale(src, centredSquare(src.Bounds())), readOrientation(data))

	var out bytes.Buffer
	if err := jpeg.Encode(&out, square, &jpeg.Options{Quality: jpegQuality}); err != nil {
		return nil, fmt.Errorf("headshot: encode: %w", err)
	}
	return out.Bytes(), nil
}

// orient rotates the image upright per the declared EXIF orientation. Only the ROTATION is
// applied, never the mirroring: a flip is deliberate, and undoing it hands back a photo unlike
// the one the member sees elsewhere. The transposed orientations (5 and 7) get the turn and
// keep the flip; the pure mirrors (2 and 4) are left alone.
func orient(src image.Image, orientation int) image.Image {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	switch orientation {
	case orientationRotate180:
		return remap(src, w, h, func(x, y int) (int, int) { return w - 1 - x, h - 1 - y })
	case orientationRotate90, orientationTransverse:
		return remap(src, h, w, func(x, y int) (int, int) { return h - 1 - y, x })
	case orientationRotate270, orientationTranspose:
		return remap(src, h, w, func(x, y int) (int, int) { return y, w - 1 - x })
	default:
		return src
	}
}

// remap rebuilds the image at dstW×dstH, placing each source pixel where at says. A quarter
// turn swaps the dimensions, so the caller states the destination size.
func remap(src image.Image, dstW, dstH int, at func(x, y int) (int, int)) image.Image {
	b := src.Bounds()
	dst := image.NewRGBA(image.Rect(0, 0, dstW, dstH))
	for y := range b.Dy() {
		for x := range b.Dx() {
			dx, dy := at(x, y)
			dst.Set(dx, dy, src.At(b.Min.X+x, b.Min.Y+y))
		}
	}
	return dst
}

// centredSquare is the largest square inside the bounds. A portrait keeps its middle band
// rather than its top, since a face in an arm's-length headshot sits mid-frame.
func centredSquare(b image.Rectangle) image.Rectangle {
	edge := min(b.Dx(), b.Dy())
	return image.Rect(0, 0, edge, edge).Add(image.Point{
		X: b.Min.X + (b.Dx()-edge)/2,
		Y: b.Min.Y + (b.Dy()-edge)/2,
	})
}

// scale resamples the source rectangle into an outputEdge square, cropping and resizing in one
// pass. CatmullRom is the sharpest x/image kernel and cost is irrelevant at 512 px.
//
// The destination starts opaque white and the source is drawn Over it: JPEG has no alpha, and
// a cut-out PNG drawn with draw.Src would encode its transparent pixels as black, silently.
func scale(src image.Image, r image.Rectangle) image.Image {
	dst := image.NewRGBA(image.Rect(0, 0, outputEdge, outputEdge))
	draw.Draw(dst, dst.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
	draw.CatmullRom.Scale(dst, dst.Bounds(), src, r, draw.Over, nil)
	return dst
}
