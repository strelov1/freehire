package main

import (
	"encoding/binary"
	"math"
	"testing"
)

// wkbMultiPolygon encodes one little-endian WKB MultiPolygon wrapped in a GPKG header
// with no envelope — the shape the GHS-FUA file stores.
func wkbMultiPolygon(polys [][][][2]float64) []byte {
	le := binary.LittleEndian
	b := []byte{'G', 'P', 0, 0x01} // magic, version, flags: little-endian, no envelope
	b = le.AppendUint32(b, 54009)
	b = append(b, 1)
	b = le.AppendUint32(b, 6)
	b = le.AppendUint32(b, uint32(len(polys)))
	for _, p := range polys {
		b = append(b, 1)
		b = le.AppendUint32(b, 3)
		b = le.AppendUint32(b, uint32(len(p)))
		for _, r := range p {
			b = le.AppendUint32(b, uint32(len(r)))
			for _, pt := range r {
				b = le.AppendUint64(b, math.Float64bits(pt[0]))
				b = le.AppendUint64(b, math.Float64bits(pt[1]))
			}
		}
	}
	return b
}

func square(x0, y0, side float64) [][2]float64 {
	return [][2]float64{{x0, y0}, {x0 + side, y0}, {x0 + side, y0 + side}, {x0, y0 + side}, {x0, y0}}
}

func TestParseGPKGAndContains(t *testing.T) {
	blob := wkbMultiPolygon([][][][2]float64{{square(0, 0, 10), square(4, 4, 2)}})
	m, err := parseGPKG(blob)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		x, y float64
		want bool
	}{
		{1, 1, true},   // inside the shell
		{5, 5, false},  // inside the hole
		{11, 1, false}, // outside
	} {
		if got := m.contains(c.x, c.y); got != c.want {
			t.Errorf("contains(%v,%v) = %v, want %v", c.x, c.y, got, c.want)
		}
	}
}

func TestParseGPKGSkipsEnvelope(t *testing.T) {
	blob := wkbMultiPolygon([][][][2]float64{{square(0, 0, 10)}})
	// Re-flag as an xy envelope (code 1, 32 bytes) and splice one in after srs_id.
	withEnv := append([]byte{}, blob[:8]...)
	withEnv[3] = 0x01 | 1<<1
	withEnv = append(withEnv, make([]byte, 32)...)
	withEnv = append(withEnv, blob[8:]...)
	m, err := parseGPKG(withEnv)
	if err != nil {
		t.Fatal(err)
	}
	if !m.contains(1, 1) {
		t.Error("point inside lost after envelope skip")
	}
}

// Sacramento in EPSG:54009. The reference is the same formula evaluated in Python;
// the end-to-end check that the projection matches the FUA polygons is Task 2 Step 6
// (Folsom must land in Sacramento).
func TestMollweide(t *testing.T) {
	x, y := mollweide(38.5816, -121.4944)
	if math.Abs(x - -10449779) > 1 || math.Abs(y-4630249) > 1 {
		t.Errorf("mollweide(Sacramento) = (%.0f, %.0f)", x, y)
	}
	if x, y := mollweide(0, 0); x != 0 || y != 0 {
		t.Errorf("origin = (%v, %v)", x, y)
	}
}
