package headshot

import (
	"bytes"
	"encoding/binary"
)

// orientationUnset is what a file that declares nothing reads back as: the EXIF value
// for "upright", so a caller can apply it unconditionally.
const orientationUnset = 1

// EXIF orientation values this package acts on. The pure mirrors (2 and 4) are read but
// deliberately not applied; 5 and 7 are a mirror plus a quarter turn, and only the turn is
// applied — see orient in normalize.go.
const (
	orientationRotate180  = 3
	orientationRotate90   = 6 // 90° clockwise restores upright
	orientationRotate270  = 8 // 90° counter-clockwise restores upright
	orientationTranspose  = 5 // mirrored; its turn matches orientationRotate270
	orientationTransverse = 7 // mirrored; its turn matches orientationRotate90
)

// exifHeader is the APP1 payload prefix that marks the segment as EXIF rather than,
// say, XMP — which also lives in APP1 and must not be parsed as TIFF.
var exifHeader = []byte("Exif\x00\x00")

// readOrientation returns the EXIF orientation a JPEG declares (1–8), or
// orientationUnset when there is none. The input is an untrusted upload, so every read
// is bounds-checked and any malformed structure yields orientationUnset rather than an
// error: an unreadable orientation is not a reason to refuse an otherwise valid image.
//
// Only JPEG is handled. PNG has no orientation concept, and the WebP containers a phone
// produces carry the rotation baked into the pixels.
func readOrientation(data []byte) int {
	seg := exifSegment(data)
	if seg == nil {
		return orientationUnset
	}
	return orientationFromTIFF(seg)
}

// exifSegment walks the JPEG marker chain and returns the TIFF block of the first EXIF
// APP1 segment, or nil. It stops at SOS: everything past it is entropy-coded scan data
// where a 0xFF byte is not a marker.
func exifSegment(data []byte) []byte {
	if len(data) < 4 || data[0] != 0xFF || data[1] != 0xD8 { // SOI
		return nil
	}
	for i := 2; i+4 <= len(data); {
		if data[i] != 0xFF {
			return nil // not at a marker boundary — the chain is malformed
		}
		marker := data[i+1]
		switch {
		case marker == 0xFF: // fill byte; markers may be padded with them
			i++
			continue
		case marker >= 0xD0 && marker <= 0xD9: // RSTn, SOI, EOI
			i += 2 // standalone marker, no length field
			continue
		case marker == 0xDA: // SOS — scan data follows
			return nil
		}
		size := int(binary.BigEndian.Uint16(data[i+2:]))
		if size < 2 || i+2+size > len(data) {
			return nil
		}
		payload := data[i+4 : i+2+size]
		if marker == 0xE1 && bytes.HasPrefix(payload, exifHeader) {
			return payload[len(exifHeader):]
		}
		i += 2 + size
	}
	return nil
}

// orientationFromTIFF reads tag 0x0112 out of IFD0 of a TIFF block.
func orientationFromTIFF(tiff []byte) int {
	if len(tiff) < 8 {
		return orientationUnset
	}
	var order binary.ByteOrder
	switch string(tiff[:2]) {
	case "II":
		order = binary.LittleEndian
	case "MM":
		order = binary.BigEndian
	default:
		return orientationUnset
	}
	if order.Uint16(tiff[2:]) != 42 { // TIFF magic
		return orientationUnset
	}
	ifd := int(order.Uint32(tiff[4:]))
	if ifd+2 > len(tiff) {
		return orientationUnset
	}
	// The declared entry count is untrusted: clamp it to what the buffer can actually
	// hold rather than believing a header that claims thousands of entries.
	const entrySize = 12
	count := int(order.Uint16(tiff[ifd:]))
	if max := (len(tiff) - ifd - 2) / entrySize; count > max {
		count = max
	}
	for n := range count {
		entry := tiff[ifd+2+n*entrySize:]
		if order.Uint16(entry) != 0x0112 {
			continue
		}
		if order.Uint16(entry[2:]) != 3 { // type SHORT
			return orientationUnset
		}
		// A SHORT is stored in the first half of the 4-byte value field.
		if v := int(order.Uint16(entry[8:])); v >= 1 && v <= 8 {
			return v
		}
		return orientationUnset
	}
	return orientationUnset
}
