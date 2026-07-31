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
	// decompression bomb is small on disk and enormous in memory, and the byte cap does not
	// bound it — a 0.6 MiB JPEG can decode to 50 MP. The ceiling is what one request may
	// cost the API process, not what a camera can produce: the output is 512 px, so pixels
	// past this point buy nothing and only lengthen the resample.
	maxSourcePixels = 30_000_000
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
	// Resize BEFORE rotating. The centred square commutes with a quarter turn, so the
	// result is the same either way — but rotating the source would allocate a second
	// full-size buffer and copy it pixel by pixel, which on a 30 MP upload is seconds of
	// CPU and hundreds of megabytes. Rotating the 512 px square is microseconds.
	square := orient(scale(src, centredSquare(src.Bounds())), readOrientation(data))

	var out bytes.Buffer
	if err := jpeg.Encode(&out, square, &jpeg.Options{Quality: jpegQuality}); err != nil {
		return nil, fmt.Errorf("headshot: encode: %w", err)
	}
	return out.Bytes(), nil
}

// orient rotates the image so it is upright, per the EXIF orientation its file declared.
// Only the ROTATION each orientation carries is applied — never the mirroring: a flip is
// something a person did on purpose, and silently undoing it hands them back a photo that
// is not the one they see everywhere else. The transposed orientations (5 and 7) are a
// mirror AND a quarter turn, so they get the turn and keep the flip; the pure mirrors
// (2 and 4) are left entirely alone.
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
// turn swaps the dimensions, which is why the destination size is stated by the caller
// rather than inferred.
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

// centredSquare is the largest square inside the bounds. A portrait photo therefore keeps
// its middle band rather than its top, which is where a face usually sits in a headshot
// shot at arm's length.
func centredSquare(b image.Rectangle) image.Rectangle {
	edge := min(b.Dx(), b.Dy())
	return image.Rect(0, 0, edge, edge).Add(image.Point{
		X: b.Min.X + (b.Dx()-edge)/2,
		Y: b.Min.Y + (b.Dy()-edge)/2,
	})
}

// scale resamples the source rectangle into an outputEdge square — the crop and the resize
// in one resampling pass, since Scale reads a source rect natively. CatmullRom is the
// sharpest of the kernels x/image offers and the cost is irrelevant on a 512 px target; the
// standard library has no resampling scaler at all.
//
// The destination starts opaque white and the source is drawn Over it, because JPEG has no
// alpha: a cut-out PNG scaled with draw.Src would arrive premultiplied, and jpeg.Encode
// reads those transparent pixels as pure black — a face on a black square, with no error to
// notice it by.
func scale(src image.Image, r image.Rectangle) image.Image {
	dst := image.NewRGBA(image.Rect(0, 0, outputEdge, outputEdge))
	draw.Draw(dst, dst.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
	draw.CatmullRom.Scale(dst, dst.Bounds(), src, r, draw.Over, nil)
	return dst
}
