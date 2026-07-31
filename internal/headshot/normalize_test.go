package headshot

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

// bands builds an image split into vertical or horizontal colour bands, so a crop or a
// rotation is observable from a couple of sampled pixels.
func bands(w, h int, horizontal bool, colors ...color.RGBA) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			pos, span := x, w
			if horizontal {
				pos, span = y, h
			}
			i := pos * len(colors) / span
			img.Set(x, y, colors[i])
		}
	}
	return img
}

func encode(t *testing.T, img image.Image, format string) []byte {
	t.Helper()
	var buf bytes.Buffer
	var err error
	if format == "png" {
		err = png.Encode(&buf, img)
	} else {
		err = jpeg.Encode(&buf, img, &jpeg.Options{Quality: 95})
	}
	if err != nil {
		t.Fatalf("encode %s: %v", format, err)
	}
	return buf.Bytes()
}

var (
	red   = color.RGBA{R: 220, A: 255}
	green = color.RGBA{G: 200, A: 255}
	blue  = color.RGBA{B: 220, A: 255}
)

// dominant names the strongest channel of a pixel, which survives JPEG's lossiness where
// an exact colour comparison would not.
func dominant(c color.Color) string {
	r, g, b, _ := c.RGBA()
	switch {
	case r > g && r > b:
		return "red"
	case g > r && g > b:
		return "green"
	case b > r && b > g:
		return "blue"
	}
	return "none"
}

func decodeResult(t *testing.T, data []byte) image.Image {
	t.Helper()
	img, format, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("decode normalized output: %v", err)
	}
	if format != "jpeg" {
		t.Errorf("normalized output format = %q, want jpeg", format)
	}
	return img
}

// webpFixture is a 300x100 lossless WebP of three vertical bands (red, green, blue) —
// the same shape the crop test uses. It is committed as bytes because x/image/webp is
// decode-only, and it exists because the blank webp import is the ONLY thing making the
// format decodable: without a test that reads one, a tidy-up that drops the import turns
// every Safari and Android upload into a 400 with nothing going red.
const webpFixture = "UklGRjwAAABXRUJQVlA4TC8AAAAvK8EYABcwyAKBJJj9iYYRCCTB7E80zPwHdwYIsm0mecmTzmAR/Z+A+ffx818BAQA="

func decodeFixture(t *testing.T, b64 string) []byte {
	t.Helper()
	data, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		t.Fatalf("decode fixture: %v", err)
	}
	return data
}

func TestNormalize_ProducesFixedSquareJPEG(t *testing.T) {
	for _, src := range []struct {
		name string
		data []byte
	}{
		{"landscape jpeg", encode(t, bands(400, 200, false, red, green, blue), "jpeg")},
		{"portrait png", encode(t, bands(120, 480, true, red, green, blue), "png")},
		{"already square", encode(t, bands(64, 64, false, green), "jpeg")},
		{"webp", decodeFixture(t, webpFixture)},
	} {
		t.Run(src.name, func(t *testing.T) {
			out, err := Normalize(src.data)
			if err != nil {
				t.Fatalf("Normalize: %v", err)
			}
			got := decodeResult(t, out).Bounds()
			if got.Dx() != outputEdge || got.Dy() != outputEdge {
				t.Errorf("output is %dx%d, want %dx%d", got.Dx(), got.Dy(), outputEdge, outputEdge)
			}
		})
	}
}

func TestNormalize_CropsTheCentredSquare(t *testing.T) {
	// Three equal vertical bands in a 300x100 image: the centred square is exactly the
	// green one, so nothing red or blue may survive.
	out, err := Normalize(encode(t, bands(300, 100, false, red, green, blue), "jpeg"))
	if err != nil {
		t.Fatalf("Normalize: %v", err)
	}
	img := decodeResult(t, out)
	for _, p := range []image.Point{{X: 8, Y: 8}, {X: 256, Y: 256}, {X: 500, Y: 500}} {
		if got := dominant(img.At(p.X, p.Y)); got != "green" {
			t.Errorf("pixel %v is %s, want green — the crop is not centred", p, got)
		}
	}
}

func TestNormalize_AppliesDeclaredOrientation(t *testing.T) {
	// A tall image, red on top and blue at the bottom. A quarter turn makes the bands
	// vertical, so each case names the edge that must be red and what it expects there.
	cases := []struct {
		name        string
		orientation uint16
		bigEndian   bool
		probe       image.Point
		want        string
	}{
		{"upright is untouched", 1, false, image.Point{X: 256, Y: 20}, "red"},
		{"180 degrees puts the bottom on top", 3, false, image.Point{X: 256, Y: 20}, "blue"},
		{"90 CW is applied", 6, true, image.Point{X: 20, Y: 256}, "blue"},
		{"90 CCW is applied", 8, false, image.Point{X: 20, Y: 256}, "red"},
		// 5 and 7 are a mirror plus a quarter turn: the turn is applied (so the bands end
		// up vertical, like 8 and 6 respectively), the mirror is not.
		{"transpose turns like 270", 5, false, image.Point{X: 20, Y: 256}, "red"},
		{"transverse turns like 90", 7, false, image.Point{X: 20, Y: 256}, "blue"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			src := injectAPP1(t, encode(t, bands(200, 400, true, red, blue), "jpeg"),
				exifOrientationPayload(c.orientation, c.bigEndian))
			out, err := Normalize(src)
			if err != nil {
				t.Fatalf("Normalize: %v", err)
			}
			if got := dominant(decodeResult(t, out).At(c.probe.X, c.probe.Y)); got != c.want {
				t.Errorf("pixel %v is %s, want %s", c.probe, got, c.want)
			}
		})
	}
}

// A pure mirror must NOT be undone — and proving that needs an image a horizontal flip
// actually changes, which horizontal bands do not.
func TestNormalize_LeavesAMirrorAlone(t *testing.T) {
	src := injectAPP1(t, encode(t, bands(400, 400, false, red, blue), "jpeg"),
		exifOrientationPayload(2, false)) // 2 = mirror horizontal
	out, err := Normalize(src)
	if err != nil {
		t.Fatalf("Normalize: %v", err)
	}
	if got := dominant(decodeResult(t, out).At(20, 256)); got != "red" {
		t.Errorf("left edge is %s, want red — the mirror was applied", got)
	}
}

// JPEG has no alpha, so a cut-out PNG has to land on a background the encoder can express.
// Premultiplied transparent pixels encode as pure black, which is a face on a black square.
func TestNormalize_TransparentBackgroundBecomesWhite(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 400, 400))
	for y := range 400 {
		for x := range 400 {
			// An opaque blob in the middle, everything else fully transparent.
			if (x-200)*(x-200)+(y-200)*(y-200) < 60*60 {
				img.Set(x, y, red)
			}
		}
	}
	out, err := Normalize(encode(t, img, "png"))
	if err != nil {
		t.Fatalf("Normalize: %v", err)
	}
	result := decodeResult(t, out)
	r, g, b, _ := result.At(10, 10).RGBA()
	if r < 0xF000 || g < 0xF000 || b < 0xF000 {
		t.Errorf("transparent corner encoded as rgb(%d,%d,%d), want near-white", r>>8, g>>8, b>>8)
	}
	if got := dominant(result.At(256, 256)); got != "red" {
		t.Errorf("the subject is %s, want red — the background fill covered it", got)
	}
}

func TestNormalize_RejectsNonImages(t *testing.T) {
	cases := map[string][]byte{
		"empty":          nil,
		"pdf":            []byte("%PDF-1.7\n1 0 obj\n<<>>\nendobj\n"),
		"text":           []byte("this is not an image, it is a cover letter"),
		"truncated jpeg": encode(t, bands(64, 64, false, green), "jpeg")[:20],
	}
	for name, data := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := Normalize(data); !errors.Is(err, ErrUnsupportedImage) {
				t.Errorf("Normalize error = %v, want ErrUnsupportedImage", err)
			}
		})
	}
}

func TestNormalize_RejectsOversizedUpload(t *testing.T) {
	if _, err := Normalize(make([]byte, maxUploadBytes+1)); !errors.Is(err, ErrTooLarge) {
		t.Errorf("Normalize error = %v, want ErrTooLarge", err)
	}
}

// A decompression bomb is small on disk and enormous in memory, so the dimension check
// must happen on the header — before the full decode allocates anything.
func TestNormalize_RejectsOversizedDimensionsBeforeDecoding(t *testing.T) {
	data := pngClaimingSize(t, 40000, 40000)
	if len(data) > 1024 {
		t.Fatalf("the bomb should be tiny on disk, got %d bytes", len(data))
	}
	if _, err := Normalize(data); !errors.Is(err, ErrTooLarge) {
		t.Errorf("Normalize error = %v, want ErrTooLarge", err)
	}
}

// pngClaimingSize rewrites a real PNG's IHDR to declare huge dimensions (fixing the
// chunk CRC so the header still parses), leaving the pixel data untouched.
func pngClaimingSize(t *testing.T, w, h uint32) []byte {
	t.Helper()
	data := encode(t, image.NewRGBA(image.Rect(0, 0, 1, 1)), "png")
	const ihdr = 8 + 4 // signature + chunk length field
	binary.BigEndian.PutUint32(data[ihdr+4:], w)
	binary.BigEndian.PutUint32(data[ihdr+8:], h)
	// The CRC covers the chunk type and its data (13 bytes for IHDR).
	crc := crc32.ChecksumIEEE(data[ihdr : ihdr+4+13])
	binary.BigEndian.PutUint32(data[ihdr+4+13:], crc)
	return data
}
