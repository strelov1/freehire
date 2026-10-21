package main

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
)

type ring [][2]float64
type polygon []ring
type multiPolygon []polygon

// earthRadius is the sphere EPSG:54009 (World Mollweide) is defined on.
const earthRadius = 6378137.0

// mollweide projects WGS84 degrees to World Mollweide metres, the CRS the GHS-FUA
// polygons are stored in. Newton iteration on 2θ + sin 2θ = π sin φ.
func mollweide(latDeg, lonDeg float64) (x, y float64) {
	phi := latDeg * math.Pi / 180
	lam := lonDeg * math.Pi / 180
	theta := phi
	for range 50 {
		f := 2*theta + math.Sin(2*theta) - math.Pi*math.Sin(phi)
		if math.Abs(f) < 1e-12 {
			break
		}
		d := 2 + 2*math.Cos(2*theta)
		if d == 0 { // the poles
			break
		}
		theta -= f / d
	}
	x = earthRadius * 2 * math.Sqrt2 / math.Pi * lam * math.Cos(theta)
	y = earthRadius * math.Sqrt2 * math.Sin(theta)
	return x, y
}

// envelopeBytes is the GPKG header envelope size by its 3-bit code.
var envelopeBytes = map[byte]int{0: 0, 1: 32, 2: 48, 3: 48, 4: 64}

// parseGPKG decodes a GeoPackage geometry blob holding a Polygon or MultiPolygon.
func parseGPKG(blob []byte) (multiPolygon, error) {
	if len(blob) < 8 || blob[0] != 'G' || blob[1] != 'P' {
		return nil, errors.New("not a GPKG geometry")
	}
	env, ok := envelopeBytes[(blob[3]>>1)&7]
	if !ok {
		return nil, fmt.Errorf("bad envelope code %d", (blob[3]>>1)&7)
	}
	r := &wkbReader{b: blob[8+env:]}
	return r.geometry()
}

type wkbReader struct {
	b   []byte
	err error
}

func (r *wkbReader) take(n int) []byte {
	if r.err != nil || len(r.b) < n {
		r.err = errors.New("truncated WKB")
		return make([]byte, n)
	}
	out := r.b[:n]
	r.b = r.b[n:]
	return out
}

func (r *wkbReader) order() binary.ByteOrder {
	if r.take(1)[0] == 1 {
		return binary.LittleEndian
	}
	return binary.BigEndian
}

func (r *wkbReader) geometry() (multiPolygon, error) {
	bo := r.order()
	switch typ := bo.Uint32(r.take(4)); typ {
	case 3:
		p := r.polygon(bo)
		return multiPolygon{p}, r.err
	case 6:
		n := bo.Uint32(r.take(4))
		out := make(multiPolygon, 0, n)
		for range n {
			inner := r.order()
			if t := inner.Uint32(r.take(4)); t != 3 {
				return nil, fmt.Errorf("multipolygon member type %d", t)
			}
			out = append(out, r.polygon(inner))
		}
		return out, r.err
	default:
		return nil, fmt.Errorf("unsupported WKB type %d", typ)
	}
}

func (r *wkbReader) polygon(bo binary.ByteOrder) polygon {
	nr := bo.Uint32(r.take(4))
	p := make(polygon, 0, nr)
	for range nr {
		np := bo.Uint32(r.take(4))
		rg := make(ring, np)
		for i := range rg {
			rg[i][0] = math.Float64frombits(bo.Uint64(r.take(8)))
			rg[i][1] = math.Float64frombits(bo.Uint64(r.take(8)))
		}
		p = append(p, rg)
	}
	return p
}

// contains is even-odd ray casting: inside a shell and outside all of its holes.
func (m multiPolygon) contains(x, y float64) bool {
	for _, p := range m {
		if len(p) == 0 || !p[0].contains(x, y) {
			continue
		}
		inHole := false
		for _, h := range p[1:] {
			if h.contains(x, y) {
				inHole = true
				break
			}
		}
		if !inHole {
			return true
		}
	}
	return false
}

func (rg ring) contains(x, y float64) bool {
	in := false
	for i, j := 0, len(rg)-1; i < len(rg); j, i = i, i+1 {
		xi, yi, xj, yj := rg[i][0], rg[i][1], rg[j][0], rg[j][1]
		if (yi > y) != (yj > y) && x < (xj-xi)*(y-yi)/(yj-yi)+xi {
			in = !in
		}
	}
	return in
}

func (m multiPolygon) bbox() (minX, minY, maxX, maxY float64) {
	minX, minY = math.Inf(1), math.Inf(1)
	maxX, maxY = math.Inf(-1), math.Inf(-1)
	for _, p := range m {
		for _, pt := range p[0] {
			minX, maxX = math.Min(minX, pt[0]), math.Max(maxX, pt[0])
			minY, maxY = math.Min(minY, pt[1]), math.Max(maxY, pt[1])
		}
	}
	return
}
