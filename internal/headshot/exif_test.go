package headshot

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"testing"
)

// jpegWithOrientation builds a decodable JPEG carrying an EXIF APP1 segment whose
// only IFD0 entry is Orientation (tag 0x0112) with the given value. bigEndian picks
// the TIFF byte order, since a camera may write either.
func jpegWithOrientation(t *testing.T, orientation uint16, bigEndian bool) []byte {
	t.Helper()
	return injectAPP1(t, encodeJPEG(t, 4, 2), exifOrientationPayload(orientation, bigEndian))
}

// exifOrientationPayload is the APP1 payload: the "Exif\0\0" marker, a TIFF header, and
// an IFD0 holding a single Orientation entry.
func exifOrientationPayload(orientation uint16, bigEndian bool) []byte {
	var order binary.ByteOrder = binary.LittleEndian
	tag := []byte("II")
	if bigEndian {
		order = binary.BigEndian
		tag = []byte("MM")
	}
	var tiff bytes.Buffer
	tiff.Write(tag)
	_ = binary.Write(&tiff, order, uint16(42)) // TIFF magic
	_ = binary.Write(&tiff, order, uint32(8))  // IFD0 starts right after the header
	_ = binary.Write(&tiff, order, uint16(1))  // one entry
	_ = binary.Write(&tiff, order, uint16(0x0112))
	_ = binary.Write(&tiff, order, uint16(3)) // type SHORT
	_ = binary.Write(&tiff, order, uint32(1)) // count
	_ = binary.Write(&tiff, order, orientation)
	tiff.Write([]byte{0, 0})                  // SHORT occupies the first half of the value field
	_ = binary.Write(&tiff, order, uint32(0)) // no next IFD

	return append([]byte("Exif\x00\x00"), tiff.Bytes()...)
}

// injectAPP1 splices an APP1 segment in right after the SOI marker of a JPEG.
func injectAPP1(t *testing.T, jpg, payload []byte) []byte {
	t.Helper()
	if len(jpg) < 2 || jpg[0] != 0xFF || jpg[1] != 0xD8 {
		t.Fatalf("input is not a JPEG (missing SOI)")
	}
	seg := []byte{0xFF, 0xE1, 0, 0}
	binary.BigEndian.PutUint16(seg[2:], uint16(len(payload)+2)) // length counts itself
	out := append([]byte{}, jpg[:2]...)
	out = append(out, seg...)
	out = append(out, payload...)
	return append(out, jpg[2:]...)
}

func encodeJPEG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.Set(x, y, color.RGBA{R: uint8(x * 20), G: uint8(y * 20), B: 128, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		t.Fatalf("encode jpeg: %v", err)
	}
	return buf.Bytes()
}

func encodePNG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	return buf.Bytes()
}

func TestReadOrientation(t *testing.T) {
	cases := []struct {
		name string
		data []byte
		want int
	}{
		{"little-endian rotate 90 CW", jpegWithOrientation(t, 6, false), 6},
		{"big-endian rotate 180", jpegWithOrientation(t, 3, true), 3},
		{"upright is reported as-is", jpegWithOrientation(t, 1, false), 1},
		{"mirrored value is read, not coerced", jpegWithOrientation(t, 2, false), 2},
		{"jpeg without exif", encodeJPEG(t, 4, 2), orientationUnset},
		{"png carries no exif", encodePNG(t, 4, 2), orientationUnset},
		{"empty input", nil, orientationUnset},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := readOrientation(c.data); got != c.want {
				t.Errorf("readOrientation = %d, want %d", got, c.want)
			}
		})
	}
}

// A truncated or malformed EXIF block must degrade to "unset" rather than panic:
// the input is an untrusted upload, and every prefix of a valid file is reachable.
func TestReadOrientation_MalformedInputDegrades(t *testing.T) {
	full := jpegWithOrientation(t, 6, false)
	for n := range len(full) {
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("panic on %d-byte prefix: %v", n, r)
				}
			}()
			if got := readOrientation(full[:n]); got < orientationUnset || got > 8 {
				t.Fatalf("prefix of %d bytes yielded orientation %d", n, got)
			}
		}()
	}
}

// An entry count large enough to run past the buffer must not be trusted.
func TestReadOrientation_OverlongIFDCountIsBounded(t *testing.T) {
	payload := exifOrientationPayload(6, false)
	// The entry count sits right after the 8-byte TIFF header, itself 6 bytes into the
	// payload ("Exif\0\0"); claim 4096 entries where one is present.
	binary.LittleEndian.PutUint16(payload[6+8:], 4096)
	if got := readOrientation(injectAPP1(t, encodeJPEG(t, 4, 2), payload)); got != 6 && got != orientationUnset {
		t.Errorf("readOrientation = %d, want the real entry (6) or unset", got)
	}
}
