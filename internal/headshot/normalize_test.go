package headshot

import (
	"bytes"
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

func TestNormalize_ProducesFixedSquareJPEG(t *testing.T) {
	for _, src := range []struct {
		name string
		data []byte
	}{
		{"landscape jpeg", encode(t, bands(400, 200, false, red, green, blue), "jpeg")},
		{"portrait png", encode(t, bands(120, 480, true, red, green, blue), "png")},
		{"already square", encode(t, bands(64, 64, false, green), "jpeg")},
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
	cases := []struct {
		name        string
		orientation uint16
		bigEndian   bool
		wantTop     string
	}{
		{"upright is untouched", 1, false, "red"},
		{"180 degrees puts the bottom on top", 3, false, "blue"},
		{"90 CW is applied", 6, true, "blue"},
		{"90 CCW is applied", 8, false, "red"},
		{"mirrored values are left alone", 2, false, "red"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			// A tall image, red on top and blue at the bottom. Under rotation by 90° the
			// bands become vertical, so only the pair {3, 1} keeps a horizontal split —
			// the 90° cases are asserted on their left edge instead.
			src := injectAPP1(t, encode(t, bands(200, 400, true, red, blue), "jpeg"),
				exifOrientationPayload(c.orientation, c.bigEndian))
			out, err := Normalize(src)
			if err != nil {
				t.Fatalf("Normalize: %v", err)
			}
			img := decodeResult(t, out)
			probe := image.Point{X: 256, Y: 20} // top edge
			if c.orientation == 6 || c.orientation == 8 {
				probe = image.Point{X: 20, Y: 256} // left edge
			}
			if got := dominant(img.At(probe.X, probe.Y)); got != c.wantTop {
				t.Errorf("pixel %v is %s, want %s", probe, got, c.wantTop)
			}
		})
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
