# Metro Areas Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A `metros` job facet ("Sacramento (US)", "Moscow (RU)") derived at ingest from the
location line, filterable in search and selectable in the Location pane.

**Architecture:** A run-by-hand generator (`cmd/gen-metros`) assigns every GeoNames place to
a GHS-FUA commuting zone and commits `internal/dict/location/metros.tsv`. `location.Parse`
reads it — city + stated state picks the exact place, a bare city only when unambiguous —
and emits `Geo.Metros`. The value flows like `cities`: `jobderive` → `jobs.metros` column →
search document (closure-widened) → `metros` filter in the geography OR-group → a "Metro
areas" group in `LocationPane`.

**Tech Stack:** Go, `modernc.org/sqlite` (generator only — reads the GeoPackage), sqlc,
PostgreSQL, Meilisearch, SvelteKit 5.

Spec: `docs/superpowers/specs/2026-10-21-metro-areas-design.md`.
Work in the worktree `/Users/i_strelov/Projects/hire-metro-areas` (branch `metro-areas`).

## Global Constraints

- English only in code, comments, commits.
- Dictionary rule: never guess — an unresolvable or ambiguous city emits NO metro.
- Metro value format: `<Name> (<CC>)` (CC = upper-case ISO 3166-1 alpha-2). UI label: `<Name> area`.
- `metros` is dict-only (no LLM fallback), unlike `cities`.
- Before every commit touching Go: `gofmt -l .` prints nothing, `go vet ./...`, `go test ./...`. Before push: `go vet -tags=integration ./...`.
- `internal/platform/db/` is generated: edit `queries/*.sql`, run `make sqlc`.
- New migration file: `migrations/0186_jobs_metros.sql` (0185 is the current highest — re-check `ls migrations | tail -1` first and bump if main moved).
- Never commit with backticks inside `git commit -m`; use `-F` with a file.
- No prettier.

---

### Task 1: `cmd/gen-metros` — geometry and membership

**Files:**
- Create: `cmd/gen-metros/geom.go` (GPKG/WKB parse, Mollweide, point-in-polygon)
- Create: `cmd/gen-metros/geom_test.go`

**Interfaces:**
- Produces:
  - `type ring [][2]float64`
  - `type polygon []ring` (ring 0 = shell, rest = holes)
  - `type multiPolygon []polygon`
  - `func parseGPKG(blob []byte) (multiPolygon, error)`
  - `func mollweide(latDeg, lonDeg float64) (x, y float64)`
  - `func (m multiPolygon) contains(x, y float64) bool`
  - `func (m multiPolygon) bbox() (minX, minY, maxX, maxY float64)`

- [ ] **Step 1: Write the failing tests**

```go
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
```



- [ ] **Step 2: Run** `go test ./cmd/gen-metros/` — expect FAIL (undefined: parseGPKG).

- [ ] **Step 3: Implement `geom.go`**

```go
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
```

- [ ] **Step 4: Run** `go test ./cmd/gen-metros/` — expect PASS.

- [ ] **Step 5: Commit** — `git add cmd/gen-metros && git commit -F <msg>` with message
`gen-metros: read GHS-FUA polygons and test point membership`.

---

### Task 2: `cmd/gen-metros` — assignment, naming, output; generate `metros.tsv`

**Files:**
- Create: `cmd/gen-metros/main.go`, `cmd/gen-metros/assign.go`, `cmd/gen-metros/assign_test.go`
- Modify: `go.mod`/`go.sum` (`go get modernc.org/sqlite`)
- Modify: `Makefile` (add `gen-metros` target after `gen-cities`, line ~47)
- Create (generated): `internal/dict/location/metros.tsv`

**Interfaces:**
- Consumes: Task 1 geometry.
- Produces: `metros.tsv`, format (tab-separated, `#` comment header):
  `city-name-lowercase<TAB>cc<TAB>admin1<TAB>Metro Value`
  e.g. `roseville	us	CA	Sacramento (US)`. `admin1` is the US/CA postal code
  (`CA`, `ON`), empty for every other country.

Pure functions in `assign.go`:

```go
type fua struct {
	id      int
	country string // lower-case alpha-2
	geom    multiPolygon
	minX, minY, maxX, maxY float64
}

type gnPlace struct {
	name     string // GeoNames display name
	country  string // lower-case alpha-2
	admin1   string // postal code for us/ca, "" otherwise
	lat, lon float64
	pop      int64
}

type assigned struct {
	place gnPlace
	fuaID int
}

// assign returns, for each place, the FUA of ITS country containing it (FUAs are
// already split per country in the source: Tijuana MEX and Tijuana USA are two
// features). Places in no FUA are dropped.
func assign(places []gnPlace, fuas []fua) []assigned

// metroNames names each (fuaID) by the most populous place assigned to it, unless
// overrides names it; returns fuaID → "Name, CC". Two FUAs of one country that
// end up with the same value get " <admin1>" inserted before the comma when both
// have one ("Springfield MA (US)"); a collision that survives that is an error
// naming both FUA ids, to be settled with an override.
func metroNames(as []assigned, overrides map[int]string) (map[int]string, error)
```

`assign` filters FUAs by country first, then by bbox, then `contains`.

- [ ] **Step 1: Write failing tests** in `assign_test.go`:

```go
func TestAssignMatchesCountryAndPolygon(t *testing.T) {
	us := fua{id: 1, country: "us", geom: multiPolygon{{square(0, 0, 10)}}}
	us.minX, us.minY, us.maxX, us.maxY = us.geom.bbox()
	mx := fua{id: 2, country: "mx", geom: multiPolygon{{square(0, 0, 10)}}}
	mx.minX, mx.minY, mx.maxX, mx.maxY = mx.geom.bbox()
	// Places are given in degrees; to keep the test independent of the projection,
	// projectFn is swapped for identity.
	defer swapProject(func(lat, lon float64) (float64, float64) { return lon, lat })()
	got := assign([]gnPlace{
		{name: "San Diego", country: "us", lat: 5, lon: 5, pop: 1_400_000},
		{name: "Tijuana", country: "mx", lat: 5, lon: 5, pop: 1_900_000},
		{name: "Nowhere", country: "us", lat: 50, lon: 50, pop: 20_000},
	}, []fua{us, mx})
	if len(got) != 2 || got[0].fuaID != 1 || got[1].fuaID != 2 {
		t.Fatalf("assign = %+v", got)
	}
}

func TestMetroNamesByPopulationWithOverride(t *testing.T) {
	as := []assigned{
		{gnPlace{name: "San Jose", country: "us", pop: 1_000_000}, 1255},
		{gnPlace{name: "San Francisco", country: "us", pop: 870_000}, 1255},
		{gnPlace{name: "Sacramento", country: "us", pop: 500_000}, 7},
		{gnPlace{name: "Folsom", country: "us", pop: 80_000}, 7},
	}
	got, err := metroNames(as, map[int]string{1255: "San Francisco Bay Area"})
	if err != nil {
		t.Fatal(err)
	}
	if got[1255] != "San Francisco Bay Area (US)" || got[7] != "Sacramento (US)" {
		t.Errorf("metroNames = %v", got)
	}
}

func TestMetroNamesDisambiguatesByAdmin1ThenFails(t *testing.T) {
	as := []assigned{
		{gnPlace{name: "Springfield", country: "us", admin1: "MA", pop: 150_000}, 1},
		{gnPlace{name: "Springfield", country: "us", admin1: "MO", pop: 160_000}, 2},
	}
	got, err := metroNames(as, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got[1] != "Springfield MA (US)" || got[2] != "Springfield MO (US)" {
		t.Errorf("metroNames = %v", got)
	}
	_, err = metroNames([]assigned{
		{gnPlace{name: "Valencia", country: "es", pop: 800_000}, 3},
		{gnPlace{name: "Valencia", country: "es", pop: 10_000}, 4},
	}, nil)
	if err == nil {
		t.Error("same-country collision without admin1 must fail")
	}
}
```

`swapProject` swaps a package-level `var project = mollweide` used by `assign`:

```go
var project = mollweide

func swapProject(f func(lat, lon float64) (float64, float64)) func() {
	old := project
	project = f
	return func() { project = old }
}
```
(`swapProject` lives in `assign_test.go`.)

- [ ] **Step 2: Run** `go test ./cmd/gen-metros/` — FAIL (undefined: assign).

- [ ] **Step 3: Implement `assign.go`**

```go
package main

import (
	"fmt"
	"sort"
	"strings"
)

var project = mollweide

func assign(places []gnPlace, fuas []fua) []assigned {
	byCountry := map[string][]fua{}
	for _, f := range fuas {
		byCountry[f.country] = append(byCountry[f.country], f)
	}
	var out []assigned
	for _, p := range places {
		x, y := project(p.lat, p.lon)
		for _, f := range byCountry[p.country] {
			if x < f.minX || x > f.maxX || y < f.minY || y > f.maxY {
				continue
			}
			if f.geom.contains(x, y) {
				out = append(out, assigned{place: p, fuaID: f.id})
				break
			}
		}
	}
	return out
}

func metroNames(as []assigned, overrides map[int]string) (map[int]string, error) {
	type top struct{ p gnPlace }
	best := map[int]top{}
	for _, a := range as {
		if b, ok := best[a.fuaID]; !ok || a.place.pop > b.p.pop {
			best[a.fuaID] = top{p: a.place}
		}
	}
	names := map[int]string{}
	owners := map[string][]int{}
	for id, b := range best {
		name := b.p.name
		if o, ok := overrides[id]; ok {
			name = o
		}
		v := name + ", " + strings.ToUpper(b.p.country)
		names[id] = v
		owners[v] = append(owners[v], id)
	}
	var clashes []string
	for v, ids := range owners {
		if len(ids) < 2 {
			continue
		}
		resolved := true
		for _, id := range ids {
			a1 := best[id].p.admin1
			if a1 == "" {
				resolved = false
				break
			}
			name, cc, _ := strings.Cut(v, ", ")
			names[id] = name + " " + a1 + ", " + cc
		}
		if !resolved {
			sort.Ints(ids)
			clashes = append(clashes, fmt.Sprintf("%q: FUA ids %v", v, ids))
		}
	}
	if len(clashes) > 0 {
		sort.Strings(clashes)
		return nil, fmt.Errorf("metro names collide, add an override for each: %s", strings.Join(clashes, "; "))
	}
	return names, nil
}
```

- [ ] **Step 4: Run** — PASS.

- [ ] **Step 5: Implement `main.go`** (I/O only; mirrors `cmd/gen-cities/main.go`):

```go
// Command gen-metros writes internal/dict/location/metros.tsv: every GeoNames place
// (cities1000, CC BY 4.0) that lies inside a GHS-FUA R2019A functional urban area
// (European Commission JRC, CC BY 4.0), with the metro it belongs to. Run by hand
// (`make gen-metros`); the output is committed so the build never depends on it.
//
//go:generate go run .
package main

import (
	"archive/zip"
	"bufio"
	"bytes"
	"database/sql"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

const (
	geonamesURL  = "https://download.geonames.org/export/dump/cities1000.zip"
	geonamesFile = "cities1000.txt"
	fuaURL       = "https://jeodpp.jrc.ec.europa.eu/ftp/jrc-opendata/GHSL/GHS_FUA_UCDB2015_GLOBE_R2019A/V1-0/GHS_FUA_UCDB2015_GLOBE_R2019A_54009_1K_V1_0.zip"
	fuaFile      = "GHS_FUA_UCDB2015_GLOBE_R2019A_54009_1K_V1_0.gpkg"
	fuaTable     = "GHS_FUA_UCDB2015_GLOBE_R2019A_54009_1K_V1_0"
	outputPath   = "internal/dict/location/metros.tsv"

	// statingPopulation matches cmd/gen-cities: only places that large reach the city
	// dictionary, so only they can be looked up. Smaller places still NAME nothing —
	// naming uses the same set, so a metro is never named after a place the parser
	// cannot read.
	statingPopulation = 15000
)

// GeoNames geoname-table column indices.
const (
	colName       = 1
	colLat        = 4
	colLon        = 5
	colCountry    = 8
	colAdmin1     = 10
	colPopulation = 14
	minColumns    = 15
)

// metroOverrides renames a FUA whose most-populous-place name misleads a job seeker.
// Key: eFUA_ID. Each entry says why.
var metroOverrides = map[int]string{
	// The Bay Area FUA's largest city is San Jose, but "San Jose area" for a posting in
	// San Francisco or Oakland reads as the wrong place.
	1255: "San Francisco Bay Area",
}

// caAdmin1 maps GeoNames' numeric Canadian admin1 codes to postal codes, the form
// the location parser reads. US admin1 codes are already postal.
var caAdmin1 = map[string]string{
	"01": "AB", "02": "BC", "03": "MB", "04": "NB", "05": "NL", "07": "NS",
	"08": "ON", "09": "PE", "10": "QC", "11": "SK", "12": "YT", "13": "NT", "14": "NU",
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "gen-metros:", err)
		os.Exit(1)
	}
}

func run() error {
	tmp, err := os.MkdirTemp("", "gen-metros-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)

	gn, err := fetchMember(geonamesURL, geonamesFile)
	if err != nil {
		return err
	}
	places, err := parsePlaces(gn)
	if err != nil {
		return err
	}
	gpkg, err := fetchMember(fuaURL, fuaFile)
	if err != nil {
		return err
	}
	gpkgPath := filepath.Join(tmp, fuaFile)
	if err := os.WriteFile(gpkgPath, gpkg, 0o600); err != nil {
		return err
	}
	fuas, err := readFUAs(gpkgPath)
	if err != nil {
		return err
	}
	as := assign(places, fuas)
	names, err := metroNames(as, metroOverrides)
	if err != nil {
		return err
	}
	return writeTSV(outputPath, as, names)
}

func fetchMember(url, member string) ([]byte, error) {
	c := http.Client{Timeout: 5 * time.Minute}
	resp, err := c.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: %s", url, resp.Status)
	}
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	zr, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		return nil, err
	}
	for _, f := range zr.File {
		if f.Name == member {
			rc, err := f.Open()
			if err != nil {
				return nil, err
			}
			defer rc.Close()
			return io.ReadAll(rc)
		}
	}
	return nil, fmt.Errorf("%s: no member %s", url, member)
}

func parsePlaces(txt []byte) ([]gnPlace, error) {
	var out []gnPlace
	sc := bufio.NewScanner(bytes.NewReader(txt))
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		f := strings.Split(sc.Text(), "\t")
		if len(f) < minColumns || f[colCountry] == "" {
			continue
		}
		pop, _ := strconv.ParseInt(f[colPopulation], 10, 64)
		if pop < statingPopulation {
			continue
		}
		lat, err1 := strconv.ParseFloat(f[colLat], 64)
		lon, err2 := strconv.ParseFloat(f[colLon], 64)
		if err1 != nil || err2 != nil {
			continue
		}
		cc := strings.ToLower(f[colCountry])
		admin1 := ""
		switch cc {
		case "us":
			admin1 = f[colAdmin1]
		case "ca":
			admin1 = caAdmin1[f[colAdmin1]]
		}
		out = append(out, gnPlace{name: f[colName], country: cc, admin1: admin1, lat: lat, lon: lon, pop: pop})
	}
	return out, sc.Err()
}

func readFUAs(path string) ([]fua, error) {
	dbh, err := sql.Open("sqlite", "file:"+path+"?mode=ro")
	if err != nil {
		return nil, err
	}
	defer dbh.Close()
	rows, err := dbh.Query(`SELECT CAST(eFUA_ID AS INTEGER), Cntry_ISO, geom FROM "` + fuaTable + `"`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []fua
	for rows.Next() {
		var id int
		var iso3 string
		var blob []byte
		if err := rows.Scan(&id, &iso3, &blob); err != nil {
			return nil, err
		}
		cc, ok := iso3to2[iso3]
		if !ok {
			return nil, fmt.Errorf("FUA %d: unknown country %q — add it to iso3to2", id, iso3)
		}
		g, err := parseGPKG(blob)
		if err != nil {
			return nil, fmt.Errorf("FUA %d: %w", id, err)
		}
		f := fua{id: id, country: cc, geom: g}
		f.minX, f.minY, f.maxX, f.maxY = g.bbox()
		out = append(out, f)
	}
	return out, rows.Err()
}

func writeTSV(path string, as []assigned, names map[int]string) error {
	lines := make([]string, 0, len(as))
	for _, a := range as {
		lines = append(lines, strings.Join([]string{
			strings.ToLower(a.place.name), a.place.country, a.place.admin1, names[a.fuaID],
		}, "\t"))
	}
	sort.Strings(lines)
	var b strings.Builder
	b.WriteString("# Code generated by cmd/gen-metros from GeoNames cities1000 and GHS-FUA R2019A (both CC BY 4.0). DO NOT EDIT.\n")
	b.WriteString("# Regenerate with `make gen-metros`. Columns: city-lowercase<TAB>country<TAB>admin1 (us/ca postal, else empty)<TAB>metro value\n")
	for _, l := range lines {
		b.WriteString(l)
		b.WriteByte('\n')
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(b.String()), 0o644)
}
```

`iso3to2` is a `map[string]string` in a new `cmd/gen-metros/iso.go`, ISO 3166-1 alpha-3 →
lower-case alpha-2. Build it from the FUA file's actual set:
`sqlite3 -readonly <gpkg> "select distinct Cntry_ISO from <table> order by 1"` (run once by
hand against `/tmp/ghsfua/...gpkg`, or re-download), and write one entry per code that
appears. `readFUAs` fails loudly on a missing one, so the map cannot silently drop a country.

Makefile, after the `gen-cities` target:

```make
gen-metros: ## Regenerate internal/dict/location/metros.tsv from GeoNames + GHS-FUA
	go run ./cmd/gen-metros
```

- [ ] **Step 6: Generate and verify**

Run: `go get modernc.org/sqlite && go mod tidy && make gen-metros`
Then check these rows exist exactly:

```bash
grep -E '^(folsom|roseville|elk grove|rancho cordova|sacramento)	us	' internal/dict/location/metros.tsv
grep -E '^(khimki|podolsk)	ru	' internal/dict/location/metros.tsv
grep -E '^(hortolândia|sumaré)	br	' internal/dict/location/metros.tsv
grep -E '^san diego	us	' internal/dict/location/metros.tsv
```
Expected: Folsom/Roseville CA/Elk Grove/Rancho Cordova/Sacramento → `Sacramento (US)`;
Roseville MI → `Detroit (US)`; Khimki, Podolsk → `Moscow (RU)`; Hortolândia, Sumaré →
`Campinas (BR)`; San Diego → `San Diego (US)` (not Tijuana).

Then review naming: `cut -f4 internal/dict/location/metros.tsv | sort | uniq -c | sort -rn | head -60`.
For any metro whose name a job seeker would not recognise as the area (the Bay Area case),
add a `metroOverrides` entry with its reason and regenerate. If generation fails on a
collision, add overrides for the listed ids.

- [ ] **Step 7: Commit** generator + `metros.tsv` + go.mod/go.sum + Makefile.
Message: `gen-metros: assign GeoNames places to GHS-FUA metros`.

---

### Task 3: `location.Parse` emits `Metros`

**Files:**
- Create: `internal/dict/location/metros.go`, `internal/dict/location/metros_test.go`
- Modify: `internal/dict/location/location.go` (`Geo` struct :24-29, `Parse` loop :70-170, return :194)
- Modify: `internal/dict/location/dictionaries.go` (add `subdivisionCode` beside `subdivisionToCountry`)
- Modify: `internal/dict/location/AGENTS.md` (one paragraph under "Always true")

**Interfaces:**
- Produces: `Geo.Metros []string` (sorted, deduped metro values);
  `func MetroLabel(value string) string` is NOT needed in Go (UI only).
- Internal: `type metroKey struct{ name, country, admin1 string }`;
  `var metroByPlace map[metroKey]string`; `var metroByName map[[2]string]string` (only
  names whose every place in that country shares one metro); `var subdivisionCode map[string]string`.

- [ ] **Step 1: Failing tests** in `metros_test.go`:

```go
package location

import (
	"slices"
	"testing"
)

func TestParseMetros(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"Folsom, CA", []string{"Sacramento (US)"}},
		{"Roseville, California, United States", []string{"Sacramento (US)"}},
		{"Roseville, MI", []string{"Detroit (US)"}},
		{"Roseville", nil},                         // three US Rosevilles, three metros
		{"Folsom", []string{"Sacramento (US)"}},     // one US Folsom in the table
		{"Khimki, Russia", []string{"Moscow (RU)"}},
		{"San Diego, CA", []string{"San Diego (US)"}},
		{"Remote", nil},
		{"Unknownville, CA", nil},
	}
	for _, c := range cases {
		if got := Parse(c.in).Metros; !slices.Equal(got, c.want) {
			t.Errorf("Parse(%q).Metros = %v, want %v", c.in, got, c.want)
		}
	}
}

// Every subdivision the parser places in a country must also yield a code, or a
// "City, <that subdivision>" line silently loses its metro.
func TestEverySubdivisionHasACode(t *testing.T) {
	for tok := range subdivisionToCountry {
		if _, ok := subdivisionCode[tok]; !ok {
			t.Errorf("subdivision %q has a country but no code", tok)
		}
	}
}

func TestMetroTableLoads(t *testing.T) {
	if len(metroByPlace) < 10000 {
		t.Fatalf("metro table has %d places — embed or parse broken", len(metroByPlace))
	}
}
```

Note: if a case depends on the real data and fails for a data reason (e.g. GeoNames lists a
second US Folsom ≥15k), verify with `grep '^folsom	us' internal/dict/location/metros.tsv`
and replace the case with one that is unambiguous in the table — do not weaken the rule.

- [ ] **Step 2: Run** `go test ./internal/dict/location/ -run 'Metro|Subdivision'` — FAIL.

- [ ] **Step 3: Implement `metros.go`**

```go
package location

import (
	_ "embed"
	"strings"
)

// metrosTSV is generated by cmd/gen-metros: every GeoNames place inside a GHS-FUA
// functional urban area, with its metro value ("Sacramento (US)").
//
//go:embed metros.tsv
var metrosTSV string

type metroKey struct{ name, country, admin1 string }

var metroByPlace, metroByName = loadMetros(metrosTSV)

// loadMetros returns the exact (name, country, admin1) table and the bare-name table.
// A bare name is kept only when EVERY place of that name in that country belongs to one
// metro — "Roseville" in the US is three places in three metros and is left out, so a
// line that names only "Roseville" gets no metro rather than a guessed one.
func loadMetros(tsv string) (map[metroKey]string, map[[2]string]string) {
	exact := map[metroKey]string{}
	seen := map[[2]string]string{}
	ambiguous := map[[2]string]bool{}
	for _, line := range strings.Split(tsv, "\n") {
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		f := strings.Split(line, "\t")
		if len(f) != 4 {
			continue
		}
		exact[metroKey{f[0], f[1], f[2]}] = f[3]
		k := [2]string{f[0], f[1]}
		if prev, ok := seen[k]; ok && prev != f[3] {
			ambiguous[k] = true
		}
		seen[k] = f[3]
	}
	for k := range ambiguous {
		delete(seen, k)
	}
	return exact, seen
}

// metroFor resolves one city of a location line. admin1 is the line's single stated
// US/CA subdivision code, or "" when none (or several) was stated.
func metroFor(cityName, country, admin1 string) (string, bool) {
	name := strings.ToLower(cityName)
	if admin1 != "" {
		v, ok := metroByPlace[metroKey{name, country, admin1}]
		return v, ok
	}
	v, ok := metroByName[[2]string{name, country}]
	return v, ok
}
```

- [ ] **Step 4: Add `subdivisionCode`** in `dictionaries.go` right after `subdivisionToCountry`
(:374). It maps every key of `subdivisionToCountry` to its upper-case postal code: postal
keys map to themselves upper-cased (`"ca": "CA"`), full names to their code
(`"california": "CA"`, `"ontario": "ON"`, `"british columbia": "BC"`, …). Write it out
literally, one entry per key in `subdivisionToCountry` — `TestEverySubdivisionHasACode`
holds the two together.

```go
// subdivisionCode is the postal code of each subdivision subdivisionToCountry places,
// read by the metro lookup to tell Roseville, CA from Roseville, MI.
var subdivisionCode = map[string]string{
	"al": "AL", "ak": "AK", /* … every postal key … */
	"alabama": "AL", "alaska": "AK", /* … every full name … */
	"on": "ON", "ontario": "ON", /* … Canada … */
}
```

- [ ] **Step 5: Wire into `Parse`** (`location.go`):

1. `Geo` gains `Metros []string // metro values ("Sacramento (US)"); dict-only, see metros.go`.
2. Before the loop, add:
```go
	// Subdivision codes the line states, and the cities it names, are collected over the
	// whole line: the state usually FOLLOWS the city ("Roseville, CA").
	statedSubdivisions := map[string]struct{}{}
```
3. In the loop, right before `resolved := resolveGeoToken(tok, prevTok, tokCountry, tokRegion)`:
```go
		if _, ok := resolveSubdivision(tok, prevTok); ok {
			if code, ok := subdivisionCode[tok]; ok {
				statedSubdivisions[code] = struct{}{}
			}
		}
```
(`resolveSubdivision` applies the colliding-code gate, so "Berlin (DE)" does not count as
Delaware. It also matches "san francisco ca" multi-word forms through its last-field path;
for those, look up the LAST field: use `subdivisionCode[lastField(tok)]` when `tok` itself
is not a key. Implement `lastField` as `strings.Fields(tok)[len-1]`.)
4. After the `cityFallbackCountries` block and before `countries := …`:
```go
	var metros []string
	if len(countrySet) == 1 && len(citySet) > 0 {
		var country string
		for c := range countrySet {
			country = c
		}
		admin1 := ""
		if len(statedSubdivisions) == 1 {
			for s := range statedSubdivisions {
				admin1 = s
			}
		}
		metroSet := map[string]struct{}{}
		for name := range citySet {
			if v, ok := metroFor(name, country, admin1); ok {
				metroSet[v] = struct{}{}
			}
		}
		metros = stringset.Sorted(metroSet)
	}
```
5. Return `Metros: metros,`.

Why a single stated country: a metro needs one; a line naming two countries cannot place a
city in either without guessing.

- [ ] **Step 6: Run** `go test ./internal/dict/location/` — all PASS (including the existing
suite: `Metros` is a new field, so existing `Geo` equality tests may need `Metros` added
to their expected values — update only where the new value is correct).

- [ ] **Step 7: AGENTS.md** — add under "Always true":
"**Metros are dict-only and need one stated country.** `Geo.Metros` comes from
`metros.tsv` (generated by `cmd/gen-metros` from GeoNames + GHS-FUA). A city with a stated
US/CA subdivision is looked up exactly; a bare city only when every same-named place in
that country shares one metro. Regenerating the table needs `backfill-derive` + `reindex`
like any dictionary change."

- [ ] **Step 8: Commit** — `location: derive metro areas from the location line`.

---

### Task 4: Store `metros` on jobs

**Files:**
- Create: `migrations/0186_jobs_metros.sql`
- Modify: `internal/platform/db/queries/jobs.sql` — `UpsertJob` (:282, :323, :332, :370),
  `UpsertManualJob` (:1195/:1205/:1241), `InsertPrivateJob` (:1318/:1327),
  `UpdateManualJob` (:1379), `UpdateEmployerJob` (:1442), `UpdateJobDerived` (:2296),
  `DuplicateClosureGeoAll` (:818-831), `DuplicateClosureGeoFor` (:864-877)
- Regenerate: `make sqlc`
- Modify: `internal/job/jobderive/jobderive.go` (`Derived.Metros`, set from `geo.Metros`)
- Modify: `internal/job/job/job.go` (`Fields.Metros` after `Cities` :86; `New()` :192;
  every params mapper that sets `Cities` — :250, :341, :404, :448, :493)
- Modify: `internal/job/job/repository.go:72` (`Metros: r.Metros,`)
- Modify: `cmd/backfill-derive/main.go` (`facetsMoved` :285, params :303)
- Modify: `cmd/backfill-us-subdivision-collision/main.go` (:163, :182) — same two edits
- Test: `internal/job/jobderive/jobderive_test.go`, `internal/job/job/job_test.go`

**Interfaces:**
- Consumes: `location.Geo.Metros`.
- Produces: `db.Job.Metros []string`; `jobderive.Derived.Metros []string`;
  `job.Fields.Metros []string`; `DuplicateClosureGeoAllRow.Metros`,
  `DuplicateClosureGeoForRow.Metros`.

- [ ] **Step 1: Migration**

```sql
-- Metro areas (GHS-FUA functional urban areas) a posting's location falls in,
-- "Sacramento (US)". Derived by internal/dict/location like cities; see
-- docs/superpowers/specs/2026-10-21-metro-areas-design.md.
-- A constant default is a metadata-only change on PG 11+: no table rewrite.
ALTER TABLE jobs ADD COLUMN metros text[] DEFAULT '{}'::text[] NOT NULL;
```

Run `pnpm check:sql` — must pass (squawk may flag `adding-required-field`; it does not apply
to a constant default on PG 11+ — suppress with `-- squawk-ignore adding-required-field`
on the line before, reason beside it, only if it fires).

- [ ] **Step 2: Queries.** In every query listed above, add `metros` exactly where
`cities` appears: column list, `COALESCE(sqlc.arg(metros)::text[], '{}')` in VALUES/SET,
`metros = EXCLUDED.metros` in ON CONFLICT. In both closure queries add:

```sql
    array_agg(DISTINCT t.val) FILTER (WHERE t.kind = 'm' AND t.val <> '')::text[] AS metros
...
    UNION ALL
    SELECT 'm', e FROM unnest(o.metros) AS e
```

Do NOT add `metros` to `RefreshUnchangedJob`'s match key: `metros` is a pure function of
`location`, which `content_hash` already covers (as `countries`/`regions` are). Add one
line to the comment at :485: "metros, like countries/regions, is derived from location,
which the hash covers."

- [ ] **Step 3:** `make sqlc`, then `go build ./...` — compile errors list every params
struct that now has a `Metros` field to fill.

- [ ] **Step 4: Failing test** in `jobderive_test.go`:

```go
func TestDeriveCarriesMetros(t *testing.T) {
	d := Derive(Input{Title: "Backend Engineer", Location: "Folsom, CA"})
	if !slices.Equal(d.Metros, []string{"Sacramento (US)"}) {
		t.Errorf("Metros = %v", d.Metros)
	}
}
```
Run — FAIL. Then add `Metros []string` to `Derived` and `Metros: geo.Metros,` in the return.
(No `Input.Metros`: no adapter states a metro.) Run — PASS.

- [ ] **Step 5: Thread through `job.Fields`** — add `Metros []string` after `Cities`, set
`Metros: der.Metros` in `New()`, `Metros: f.Metros` in every mapper that sets `Cities`,
`Metros: r.Metros` in `repository.go`. In backfill-derive (and the subdivision-collision
pass) add `|| !slices.Equal(d.Metros, j.Metros)` to `facetsMoved` and `Metros: d.Metros`
to the params.

- [ ] **Step 6:** `go build ./... && go vet ./... && go vet -tags=integration ./... && go test ./internal/job/... ./cmd/backfill-derive/...`
— PASS, including `TestUpsertParams_CheapWriteMatchKeyCoversEveryColumnItWrites` (it
mutates `Input` fields; `Location` is hashed, so `Metros` is covered).

- [ ] **Step 7: Commit** — `jobs: store derived metro areas`.

---

### Task 5: Search — document, settings, filter, closure widening

**Files:**
- Modify: `internal/job/jobview/jobview.go` (`Job.Metros` after `Cities` :69; `FromDomain` :229)
- Modify: `internal/search/search/document.go` (`MergeClosureGeography` :266 → takes `ClosureGeo`)
- Modify: `internal/search/search/client.go` (FilterableAttributes :650)
- Modify: `internal/search/search/query_filter.go` (`StringFacets` :30, `locationFacets` :75)
- Modify: `internal/api/handler/facets.go:215` (`locationFacetParams`)
- Modify: `cmd/search-drain/indexer.go:134`, `cmd/reindex/main.go:353,519-532,574`,
  `internal/ingest/linkimport/linkimport.go:340`
- Modify: `web/static/openapi.yaml` (param def beside `Cities` :1126; refs beside :205,
  :278, :522; OR-group prose :31-41 and :1111-1113; Job schema beside :1706)
- Regenerate: `make gen-contracts` (adds `metros` to `web/src/lib/generated/contracts.ts`)
- Test: `internal/search/search/query_filter_test.go`, `document_test.go` (or the file
  holding `MergeClosureGeography` tests), `settings_test.go`

**Interfaces:**
- Consumes: `db.Job.Metros`, closure rows' `Metros`.
- Produces:
  ```go
  // ClosureGeo is the geography union across a document's duplicate closure.
  type ClosureGeo struct{ Countries, Regions, Cities, Metros []string }
  func (d *JobDocument) MergeClosureGeography(g ClosureGeo)
  ```
  Search param `metros` / `metros_exclude`, Meili attribute `metros`.

- [ ] **Step 1: Failing tests**

`query_filter_test.go`:
```go
func TestMetrosJoinTheLocationOrGroup(t *testing.T) {
	v := url.Values{"metros": {"Sacramento (US)"}, "cities": {"Austin"}}
	got := FilterFromValues(v) // use the exported builder the existing location-group test uses
	want := `(metros = "Sacramento (US)" OR cities = "Austin")`
	if !strings.Contains(got, `metros = "Sacramento (US)"`) || !strings.Contains(got, `cities = "Austin"`) || strings.Count(got, " OR ") < 1 {
		t.Errorf("filter = %s, want both in one OR group like %s", got, want)
	}
}

func TestMetrosExclude(t *testing.T) {
	got := FilterFromValues(url.Values{"metros_exclude": {"Detroit (US)"}})
	if !strings.Contains(got, `metros != "Detroit (US)"`) {
		t.Errorf("filter = %s", got)
	}
}
```
(Match the exact builder name and quoting style of the existing test at
`query_filter_test.go:306-320`; copy its shape.)

Closure test (next to existing `MergeClosureGeography` tests):
```go
func TestMergeClosureGeographyWidensMetros(t *testing.T) {
	d := JobDocument{}
	d.Metros = []string{"Sacramento (US)"}
	d.MergeClosureGeography(ClosureGeo{Metros: []string{"Detroit (US)", "Sacramento (US)"}})
	if !slices.Equal(d.Metros, []string{"Detroit (US)", "Sacramento (US)"}) {
		t.Errorf("Metros = %v", d.Metros)
	}
}
```

`settings_test.go`: add `metros` to the per-attribute `contains(FilterableAttributes, …)` check.

- [ ] **Step 2: Run** `go test ./internal/search/search/` — FAIL.

- [ ] **Step 3: Implement**
- `jobview.Job`: `// Metros is dict-only: the metro areas (GHS-FUA) the location line places the posting in, "Sacramento (US)".` then `Metros []string \`json:"metros"\``; in `FromDomain`: `Metros: normalizeSet(f.Metros),` — check `normalizeSet` does not lower-case; if it does, use `f.Metros` as-is (values are already canonical).
- `document.go`: add `ClosureGeo`, change the method:
  ```go
  func (d *JobDocument) MergeClosureGeography(g ClosureGeo) {
  	d.Countries = unionSorted(d.Countries, g.Countries)
  	d.Regions = unionSorted(d.Regions, g.Regions)
  	d.Cities = unionSorted(d.Cities, g.Cities)
  	d.Metros = unionSorted(d.Metros, g.Metros)
  }
  ```
  Update its doc comment ("multi-city/multi-country" → also metros).
- Callers: `search-drain/indexer.go:134` and `linkimport.go:340` →
  `doc.MergeClosureGeography(search.ClosureGeo{Countries: g.Countries, Regions: g.Regions, Cities: g.Cities, Metros: g.Metros})`.
  `reindex/main.go`: `type closureGeoLookup func(ownerID int64) search.ClosureGeo`;
  `buildClosureGeoLookup` stores `search.ClosureGeo{…, Metros: r.Metros}` per owner and
  returns `m[ownerID]`; `:574` becomes `doc.MergeClosureGeography(geo(j.ID))`. Fix the
  reindex tests that build a fake lookup.
- `client.go:650`: `"work_mode", "regions", "countries", "cities", "metros", "skills", "collections",`
- `query_filter.go`: `"metros": "metros",` after `"cities"` in `StringFacets`;
  `locationFacets` gains `"metros": true`. Update the comment at :147 that lists the group.
- `facets.go:215`: `{"regions", "countries", "cities", "metros"}`.
- `make gen-contracts`.
- `openapi.yaml`:
  ```yaml
    Metros:
      name: metros
      in: query
      style: form
      explode: true
      schema:
        type: array
        items:
          type: string
      description: |
        Metro areas ("Sacramento (US)", "Moscow (RU)") — commuting zones from GHS-FUA.
        A posting is in a metro when its location names a city inside it. Joins the
        geography OR-group.
  ```
  plus `- $ref: "#/components/parameters/Metros"` beside each `Cities` ref, `metros` in
  the OR-group prose, and a `metros: {type: array, items: {type: string}}` property beside
  `cities` in the Job schema.

- [ ] **Step 4: Run** `go build ./... && go vet ./... && go vet -tags=integration ./... && go test ./...`
— PASS. Expect `TestFilterSchemaDeclaresEveryFacetAndNothingElse` to pass on its own (the
schema derives from `StringFacets`); `TestEveryJobDocumentWriterWidensGeography` must still
pass with the new signature. `searchintent` is untouched: it does not offer metros (out of
scope), and its tests only require its OWN offered set to be a subset of `StringFacets`.

- [ ] **Step 5: Commit** — `search: filter and facet by metro area`.

---

### Task 6: Web — Metro areas group in the Location pane

**Files:**
- Create: `web/src/lib/metro.ts`, `web/src/lib/metro.test.ts`
- Modify: `web/src/lib/facets.ts` (`FACETS` :734-755, after the `cities` entry :746)
- Modify: `web/src/lib/components/filters/LocationPane.svelte`
- Modify: `web/src/lib/components/HeaderLocationFilter.svelte:69` (`CLEAR_PARAMS.jobs`)
- Modify: `web/src/lib/headerScope.ts:34` (`JOBS_SCOPE.geo`), and :43/:72 if they enumerate the triple
- Modify: `web/src/lib/components/FilterSummary.svelte:55`, `FilterModal.svelte:178`, `JobsView.svelte:562`
- Modify: `web/src/lib/filterSections.test.ts:20` (`HOSTED_ELSEWHERE` gains `metros: 'location pane'`)
- Modify: `web/src/lib/docs/filters.ts:117` (OR-group prose)

**Interfaces:**
- Produces: `export function metroLabel(value: string): string` — `"Sacramento (US)"` → `"Sacramento area"`.

- [ ] **Step 1: Failing test** `metro.test.ts`:

```ts
import { describe, expect, it } from 'vitest';
import { metroLabel } from './metro';

describe('metroLabel', () => {
  it('replaces the country suffix with "area"', () => {
    expect(metroLabel('Sacramento (US)')).toBe('Sacramento area');
    expect(metroLabel('Springfield MA (US)')).toBe('Springfield MA area');
    expect(metroLabel('San Francisco Bay Area (US)')).toBe('San Francisco Bay Area');
  });
  it('leaves a value without a suffix readable', () => {
    expect(metroLabel('Moscow')).toBe('Moscow area');
  });
});
```

- [ ] **Step 2: Run** `cd web && pnpm vitest run src/lib/metro.test.ts` — FAIL.

- [ ] **Step 3: Implement `metro.ts`**

```ts
/** A metro facet value is "<Name> (<CC>)" (internal/dict/location, metros.tsv). The pane
 *  shows "<Name> area" — unless the name already says it is an area. */
export function metroLabel(value: string): string {
  const name = value.replace(/ \([A-Z]{2}\)$/, '');
  return /\barea$/i.test(name) ? name : `${name} area`;
}
```

- [ ] **Step 4: Run** — PASS.

- [ ] **Step 5: `FACETS`** — after the `cities` entry:
```ts
  { param: 'metros', label: 'Metro area', control: 'select', dynamic: true, excludable: true, placeholder: 'Search metro areas' },
```

- [ ] **Step 6: `LocationPane.svelte`** — mirror the cities group, placed ABOVE it:

Script additions next to the cities ones:
```ts
  import { metroLabel } from '$lib/metro';
  const metroCounts = $derived(counts?.facets?.metros ?? {});
  const metroF = $derived(store.facet('metros'));
  const metroSel = $derived([...metroF.include, ...metroF.exclude]);
  const allMetros = $derived.by(() => {
    const set = new Set([...Object.keys(metroCounts), ...metroSel]);
    return [...set].sort((a, b) => (metroCounts[b] ?? 0) - (metroCounts[a] ?? 0));
  });
  const METRO_LIMIT = 12;
  const matchMetro = (m: string) => !q || metroLabel(m).toLowerCase().includes(q);
  const metroMatches = $derived(allMetros.filter(matchMetro));
  const metrosShown = $derived(q ? metroMatches : metroMatches.slice(0, METRO_LIMIT));
```
Chips (next to `push('cities', …)` :106-112):
```ts
    push('metros', metroF.include, metroLabel, false);
    ...
    push('metros', metroF.exclude, metroLabel, true);
```
`hasGeo` adds `metroSel.length`; `clearGeo` adds `store.clearFacet('metros');`.

Markup, immediately before `{#if citiesShown.length}`:
```svelte
{#if metrosShown.length}
  <div class="border-t border-border pt-3">
    <div class="mb-2 text-sm font-semibold tracking-tight">Metro areas</div>
    <div class="flex flex-wrap gap-2">
      {#each metrosShown as metro (metro)}
        {@const mExc = metroF.exclude.includes(metro)}
        {@const mInc = metroF.include.includes(metro)}
        <button
          type="button"
          onclick={() => store.cycle('metros', metro)}
          title={pillTitle(mInc, mExc, true)}
          class={pillClass(mInc || mExc, mExc, 'px-3 py-1.5 text-sm')}
        >
          {metroLabel(metro)}
        </button>
      {/each}
    </div>
  </div>
{/if}
```
(Keep the exact attribute formatting the cities block uses; `{#each}` keyed by the value,
which is unique — see the memory note on duplicate each-keys.)

- [ ] **Step 7: The triples** — add `'metros'` wherever `'regions', 'countries', 'cities'`
is enumerated: `HeaderLocationFilter.svelte:69`, `headerScope.ts:34` (and :43/:72 if they
list facets rather than special-case "worldwide"), `FilterSummary.svelte:55`,
`FilterModal.svelte:178`, `JobsView.svelte:562` (`untouched('metros')`),
`filterSections.test.ts` `HOSTED_ELSEWHERE`, `docs/filters.ts:117` prose. For each, read
the surrounding code first and add `metros` only if `cities` is there for the same reason.

- [ ] **Step 8: Run** `cd web && pnpm check && pnpm vitest run && pnpm lint` — PASS.

- [ ] **Step 9: Commit** — `web: pick a metro area in the Location pane`.

---

### Task 7: Attribution

**Files:**
- Modify: `web/src/routes/sources/+page.svelte` — a short "Geographic data" note at the
  bottom of the page.

- [ ] **Step 1:** Add, matching the page's existing paragraph styling:

```svelte
<p class="mt-10 text-xs text-muted-foreground">
  Place names and coordinates:
  <a class="underline" href="https://www.geonames.org/" rel="noopener">GeoNames</a> (CC BY 4.0).
  Metro areas:
  <a class="underline" href="https://human-settlement.emergency.copernicus.eu/ghs_fua.php" rel="noopener">GHS Functional Urban Areas</a>,
  European Commission JRC (CC BY 4.0).
</p>
```

- [ ] **Step 2:** `cd web && pnpm check && pnpm lint` — PASS. Commit — `sources: credit GeoNames and GHS-FUA`.

---

### Task 8: Ship and roll out

Not code. Follow `hire-always-finish-through-to-prod`.

- [ ] **Step 1:** Push the branch, open a PR, wait for CI green (including `integration`,
  `dead-code`, `dead-code-go`, `docs`, `sqlc`). Merge.
- [ ] **Step 2: Meili settings BEFORE the binary.** On the prod host, patch the live jobs
  index settings so `filterableAttributes` includes `metros` (the full list from
  `facetSettings()` — a patch replaces the list wholesale). Confirm
  `search-settings-drift` reports 0 afterwards.
- [ ] **Step 3: Deploy** — `ssh root@136.243.174.114 /opt/freehire/bin/release.sh freehire`
  (runs migration 0186).
- [ ] **Step 4: Backfill** — `backfill-derive` with `BACKFILL_DERIVE_CLOSED_WITHIN_DAYS=30`,
  `BACKFILL_CONCURRENCY=2`, after 03:00 UTC, via `systemd-run` (see memory
  `hire-run-prod-worker-by-hand-systemd-run`). Resume with `BACKFILL_DERIVE_FROM_ID` if cut.
- [ ] **Step 5: Reindex** — stop `freehire-reindexw.timer`, run a full `reindex`, start the
  timer again.
- [ ] **Step 6: Verify on prod:**
  `curl -s "https://freehire.me/api/v1/jobs/search?metros=Sacramento%2C%20US&limit=1" | jq .meta.total`
  is > 0, and the Location pane shows "Sacramento area" when typing "Sacramento".
- [ ] **Step 7:** Reply to Joe Taormina (thread "Re: What are you looking for?", 2026-10-19)
  that metro areas are live — draft for the user's approval before sending.
