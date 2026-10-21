// Command gen-metros writes internal/dict/location/metros.tsv: every GeoNames place
// (cities1000, CC BY 4.0) of at least statingPopulation, with the GHS-FUA R2019A
// functional urban area (European Commission JRC, CC BY 4.0) it lies in — or an empty
// metro column when it lies in none, which the loader counts as a value of its own. Run by hand
// (`make gen-metros`); the output is committed so the build never depends on it.
//
//go:generate go run .
package main

import (
	"archive/zip"
	"bufio"
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"slices"
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

	admin1URL = "https://download.geonames.org/export/dump/admin1CodesASCII.txt"
	admin2URL = "https://download.geonames.org/export/dump/admin2Codes.txt"

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
	colAdmin2     = 11
	colPopulation = 14
	minColumns    = 15
)

// metroOverrides renames a FUA whose most-populous-place name misleads a job seeker.
// Key: eFUA_ID. Each entry says why.
var metroOverrides = map[int]string{
	// The Bay Area FUA's largest city is San Jose, but "San Jose area" for a posting in
	// San Francisco or Oakland reads as the wrong place.
	1255: "San Francisco Bay Area",
	// Largest place is Quezon City, a district of Metro Manila.
	2845: "Manila",
	// Largest place is Kanayannur, the municipality that makes up most of Kochi.
	8900: "Kochi",
	// Essen, Dortmund, Duisburg and neighbours form one conurbation seekers call the Ruhr.
	536: "Ruhr",
	// Al Ahmadi is a Kuwait City suburb; the metro is known as Kuwait City.
	429: "Kuwait City",
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
	admin1Names, err := fetchNames(admin1URL)
	if err != nil {
		return err
	}
	admin2Names, err := fetchNames(admin2URL)
	if err != nil {
		return err
	}
	places, err := parsePlaces(gn, admin1Names, admin2Names)
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
	pops := map[int]float64{}
	for _, f := range fuas {
		pops[f.id] = f.pop
	}
	names, err := metroNames(as, pops, metroOverrides)
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "gen-metros: qualified %d by admin1, %d by admin2\n", qualifyStats["admin1"], qualifyStats["admin2"])
	return writeTSV(outputPath, as, names)
}

func fetchMember(url, member string) ([]byte, error) {
	c := http.Client{Timeout: 5 * time.Minute}
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.Do(req)
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

// fetchNames reads a GeoNames admin-code table into code → name (the accented column).
func fetchNames(url string) (map[string]string, error) {
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := (&http.Client{Timeout: 5 * time.Minute}).Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: %s", url, resp.Status)
	}
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	out := map[string]string{}
	for _, l := range strings.Split(string(raw), "\n") {
		f := strings.Split(l, "\t")
		if len(f) >= 2 {
			out[f[0]] = f[1]
		}
	}
	return out, nil
}

func parsePlaces(txt []byte, admin1Names, admin2Names map[string]string) ([]gnPlace, error) {
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
		// A GeoNames name can itself carry commas ("Mianzhu, Deyang, Sichuan"); Parse
		// splits a location line on them, so such a key could never match, and a comma
		// in a metro value would shred it in a comma-joined filter. Keep the leading part.
		name, _, _ := strings.Cut(f[colName], ",")
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		a1 := strings.ToUpper(cc) + "." + f[colAdmin1]
		out = append(out, gnPlace{
			name: name, country: cc, admin1: admin1, lat: lat, lon: lon, pop: pop,
			region:   admin1Names[a1],
			district: admin2Names[a1+"."+f[colAdmin2]],
		})
	}
	return out, sc.Err()
}

func readFUAs(path string) ([]fua, error) {
	dbh, err := sql.Open("sqlite", "file:"+path+"?mode=ro")
	if err != nil {
		return nil, err
	}
	defer func() { _ = dbh.Close() }()
	rows, err := dbh.QueryContext(context.Background(), `SELECT CAST(eFUA_ID AS INTEGER), Cntry_ISO, CAST(FUA_p_2015 AS REAL), geom FROM "`+fuaTable+`"`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []fua
	for rows.Next() {
		var id int
		var iso3 string
		var pop float64
		var blob []byte
		if err := rows.Scan(&id, &iso3, &pop, &blob); err != nil {
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
		f := fua{id: id, country: cc, pop: pop, geom: g}
		f.minX, f.minY, f.maxX, f.maxY = g.bbox()
		out = append(out, f)
	}
	return out, rows.Err()
}

func writeTSV(path string, as []assigned, names map[int]string) error {
	lines := make([]string, 0, len(as))
	for _, a := range as {
		metro := ""
		if a.fuaID != noFUA {
			metro = names[a.fuaID]
		}
		lines = append(lines, strings.Join([]string{
			strings.ToLower(a.place.name), a.place.country, a.place.admin1, metro,
		}, "\t"))
	}
	sort.Strings(lines)
	lines = slices.Compact(lines)
	var b strings.Builder
	b.WriteString("# Code generated by cmd/gen-metros from GeoNames cities1000 and GHS-FUA R2019A (both CC BY 4.0). DO NOT EDIT.\n")
	b.WriteString("# Regenerate with `make gen-metros`. Columns: city-lowercase<TAB>country<TAB>admin1 (us/ca postal, else empty)<TAB>metro value (empty: in no metro)\n")
	for _, l := range lines {
		b.WriteString(l)
		b.WriteByte('\n')
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(b.String()), 0o644)
}
