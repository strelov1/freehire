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
	"context"
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
	// Largest place is Quezon City, a district of Metro Manila.
	2845: "Manila",
	// Largest place is Kanayannur, the municipality that makes up most of Kochi.
	8900: "Kochi",
	// Essen, Dortmund, Duisburg and neighbours form one conurbation seekers call the Ruhr.
	536: "Ruhr",
	// Al Ahmadi is a Kuwait City suburb; the metro is known as Kuwait City.
	429: "Kuwait City",

	// Shares the name "Māndvi" with another IN metro; named by its district.
	702: "Māndvi Kutch",
	// Shares the name "Jodhpur" with another IN metro; named by its province.
	2769: "Jodhpur Gujarat",
	// Shares the name "Ereğli" with another TR metro; named by its province.
	3279: "Ereğli Zonguldak",
	// Shares the name "Nūrābād" with another IR metro; named by its province.
	3500: "Nūrābād Lorestan",
	// Shares the name "Ereğli" with another TR metro; named by its province.
	4071: "Ereğli Konya",
	// Shares the name "Jalālābād" with another IN metro; named by its province.
	4296: "Jalālābād Punjab",
	// Shares the name "Baoshan" with another CN metro; named by its province.
	5101: "Baoshan Yunnan",
	// Shares the name "Una" with another IN metro; named by its province.
	5238: "Una Gujarat",
	// Shares the name "Jodhpur" with another IN metro; named by its province.
	5324: "Jodhpur Rajasthan",
	// Shares the name "Nūrābād" with another IR metro; named by its province.
	5537: "Nūrābād Fars",
	// Shares the name "Planaltina" with another BR metro; named by its province.
	5842: "Planaltina Federal",
	// Shares the name "Planaltina" with another BR metro; named by its province.
	5854: "Planaltina Goias",
	// Shares the name "Mianyang" with another CN metro; named by its province.
	5963: "Mianyang Sichuan",
	// Shares the name "Mānsa" with another IN metro; named by its province.
	6041: "Mānsa Gujarat",
	// Shares the name "Mānsa" with another IN metro; named by its province.
	6125: "Mānsa Punjab",
	// Shares the name "Una" with another IN metro; named by its province.
	6254: "Una Himachal Pradesh",
	// Shares the name "Fatehābād" with another IN metro; named by its province.
	6318: "Fatehābād Haryana",
	// Shares the name "Tongchuan" with another CN metro; named by its province.
	6426: "Tongchuan Sichuan",
	// Shares the name "Fatehpur" with another IN metro; named by its district.
	6429: "Fatehpur Sikar",
	// Shares the name "Udaipur" with another IN metro; named by its district.
	6492: "Udaipur Udaipur",
	// Shares the name "Mianyang" with another CN metro; named by its province.
	6507: "Mianyang Shaanxi",
	// Shares the name "Rājgarh" with another IN metro; named by its district.
	6563: "Rājgarh Churu",
	// Shares the name "Gangāpur" with another IN metro; named by its district.
	6813: "Gangāpur Bhilwara",
	// Shares the name "Yangzhou" with another CN metro; named by its province.
	6883: "Yangzhou Shaanxi",
	// Shares the name "Tongchuan" with another CN metro; named by its province.
	6919: "Tongchuan Shaanxi",
	// Shares the name "Udaipur" with another IN metro; named by its district.
	6939: "Udaipur Jhunjhunu",
	// Shares the name "Māndvi" with another IN metro; named by its district.
	7127: "Māndvi Surat",
	// Shares the name "Shāhābād" with another IN metro; named by its district.
	7169: "Shāhābād Kurukshetra",
	// Shares the name "Shāhpura" with another IN metro; named by its district.
	7199: "Shāhpura Bhilwara",
	// Shares the name "Xinzhou" with another CN metro; named by its province.
	7269: "Xinzhou Shanxi",
	// Shares the name "Longgang" with another CN metro; named by its province.
	7279: "Longgang Chongqing",
	// Shares the name "Shāhpura" with another IN metro; named by its district.
	7355: "Shāhpura Jaipur",
	// Shares the name "Rājgarh" with another IN metro; named by its district.
	7591: "Rājgarh Alwar",
	// Shares the name "Kaiyuan" with another CN metro; named by its province.
	7610: "Kaiyuan Yunnan",
	// Shares the name "Fengcheng" with another CN metro; named by its province.
	7664: "Fengcheng Chongqing",
	// Shares the name "Gangāpur" with another IN metro; named by its district.
	7701: "Gangāpur Sawai Madhopur",
	// Shares the name "Sanhe" with another CN metro; named by its province.
	7766: "Sanhe Chongqing",
	// Shares the name "Mingshui" with another CN metro; named by its province.
	7830: "Mingshui Heilongjiang",
	// Shares the name "Sanhe" with another CN metro; named by its province.
	7896: "Sanhe Hebei",
	// Shares the name "Sirsi" with another IN metro; named by its province.
	7990: "Sirsi Uttar Pradesh",
	// Shares the name "Rājgarh" with another IN metro; named by its district.
	7994: "Rājgarh Rajgarh",
	// Shares the name "Rāmnagar" with another IN metro; named by its province.
	8022: "Rāmnagar Uttarakhand",
	// Shares the name "Qianjiang" with another CN metro; named by its province.
	8037: "Qianjiang Chongqing",
	// Shares the name "Shamsābād" with another IN metro; named by its district.
	8056: "Shamsābād Agra",
	// Shares the name "Fatehābād" with another IN metro; named by its province.
	8100: "Fatehābād Uttar Pradesh",
	// Shares the name "Shāhābād" with another IN metro; named by its district.
	8108: "Shāhābād Rampur",
	// Shares the name "Lanxi" with another CN metro; named by its province.
	8155: "Lanxi Heilongjiang",
	// Shares the name "Sirsi" with another IN metro; named by its province.
	8234: "Sirsi Karnataka",
	// Shares the name "Dengzhou" with another CN metro; named by its province.
	8245: "Dengzhou Henan",
	// Shares the name "Puyang" with another CN metro; named by its province.
	8302: "Puyang Henan",
	// Shares the name "Shamsābād" with another IN metro; named by its district.
	8357: "Shamsābād Ferrukhabad",
	// Shares the name "Jalālābād" with another IN metro; named by its province.
	8403: "Jalālābād Uttar Pradesh",
	// Shares the name "Zhicheng" with another CN metro; named by its province.
	8462: "Zhicheng Hubei",
	// Shares the name "Shāhābād" with another IN metro; named by its district.
	8487: "Shāhābād Hardoi",
	// Shares the name "Feicheng" with another CN metro; named by its district.
	8524: "Feicheng Tai'an Shi",
	// Shares the name "Mingshui" with another CN metro; named by its province.
	8586: "Mingshui Shandong",
	// Shares the name "Kaiyuan" with another CN metro; named by its province.
	8614: "Kaiyuan Liaoning",
	// Shares the name "Akbarpur" with another IN metro; named by its district.
	8663: "Akbarpur Kanpur Dehat",
	// Shares the name "Shāhābād" with another IN metro; named by its district.
	8690: "Shāhābād Gulbarga",
	// Shares the name "Qianjiang" with another CN metro; named by its province.
	8733: "Qianjiang Hubei",
	// Shares the name "Fengcheng" with another CN metro; named by its province.
	8811: "Fengcheng Jiangsu",
	// Shares the name "Fatehpur" with another IN metro; named by its district.
	8918: "Fatehpur Bara Banki",
	// Shares the name "Fatehpur" with another IN metro; named by its district.
	8950: "Fatehpur Fatehpur",
	// Shares the name "Fuyang" with another CN metro; named by its province.
	8963: "Fuyang Anhui",
	// Shares the name "Feicheng" with another CN metro; named by its district.
	9010: "Feicheng Linyi Shi",
	// Shares the name "Longgang" with another CN metro; named by its province.
	9018: "Longgang Shandong",
	// Shares the name "Baoshan" with another CN metro; named by its province.
	9024: "Baoshan Heilongjiang",
	// Shares the name "Dengzhou" with another CN metro; named by its province.
	9076: "Dengzhou Shandong",
	// Shares the name "Changsha" with another CN metro; named by its province.
	9100: "Changsha Hunan",
	// Shares the name "Suzhou" with another CN metro; named by its province.
	9108: "Suzhou Anhui",
	// Shares the name "Xinzhou" with another CN metro; named by its province.
	9118: "Xinzhou Hubei",
	// Shares the name "Fengcheng" with another CN metro; named by its province.
	9160: "Fengcheng Liaoning",
	// Shares the name "Cumbum" with another IN metro; named by its province.
	9227: "Cumbum Tamil Nadu",
	// Shares the name "Xinyi" with another CN metro; named by its province.
	9259: "Xinyi Jiangsu",
	// Shares the name "Akbarpur" with another IN metro; named by its district.
	9370: "Akbarpur Ambedkar Nagar",
	// Shares the name "Xinyi" with another CN metro; named by its province.
	9425: "Xinyi Guangdong",
	// Shares the name "Cumbum" with another IN metro; named by its province.
	9436: "Cumbum Andhra Pradesh",
	// Shares the name "Ji’an" with another CN metro; named by its province.
	9443: "Ji’an Jilin",
	// Shares the name "Muhammadābād" with another IN metro; named by its district.
	9616: "Muhammadābād Mau",
	// Shares the name "Ji’an" with another CN metro; named by its province.
	9667: "Ji’an Jiangxi",
	// Shares the name "Changsha" with another CN metro; named by its province.
	9747: "Changsha Guangdong",
	// Shares the name "Yangzhou" with another CN metro; named by its province.
	9769: "Yangzhou Jiangsu",
	// Shares the name "Rāmnagar" with another IN metro; named by its province.
	9778: "Rāmnagar Bihar",
	// Shares the name "Muhammadābād" with another IN metro; named by its district.
	9800: "Muhammadābād Ghazipur",
	// Shares the name "Fuzhou" with another CN metro; named by its province.
	9815: "Fuzhou Jiangxi",
	// Shares the name "Taizhou" with another CN metro; named by its province.
	9831: "Taizhou Jiangsu",
	// Shares the name "Huicheng" with another CN metro; named by its province.
	9925: "Huicheng Anhui",
	// Shares the name "Suzhou" with another CN metro; named by its province.
	9963: "Suzhou Jiangsu",
	// Shares the name "Sāhibganj" with another IN metro; named by its province.
	9966: "Sāhibganj Bihar",
	// Shares the name "Zhicheng" with another CN metro; named by its province.
	9991: "Zhicheng Zhejiang",
	// Shares the name "Islāmpur" with another IN metro; named by its province.
	10083: "Islāmpur Bihar",
	// Shares the name "Fuyang" with another CN metro; named by its province.
	10090: "Fuyang Zhejiang",
	// Shares the name "Lanxi" with another CN metro; named by its province.
	10108: "Lanxi Zhejiang",
	// Shares the name "Puyang" with another CN metro; named by its province.
	10145: "Puyang Zhejiang",
	// Shares the name "Huicheng" with another CN metro; named by its province.
	10205: "Huicheng Guangdong",
	// Shares the name "Fuzhou" with another CN metro; named by its province.
	10328: "Fuzhou Fujian",
	// Shares the name "Sāhibganj" with another IN metro; named by its province.
	10344: "Sāhibganj Jharkhand",
	// Shares the name "Islāmpur" with another IN metro; named by its province.
	10352: "Islāmpur West Bengal",
	// Shares the name "Taizhou" with another CN metro; named by its province.
	10392: "Taizhou Zhejiang",
	// Shares the name "Udaipur" with another IN metro; named by its district.
	10511: "Udaipur Gomati",
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
	defer func() { _ = dbh.Close() }()
	rows, err := dbh.QueryContext(context.Background(), `SELECT CAST(eFUA_ID AS INTEGER), Cntry_ISO, geom FROM "`+fuaTable+`"`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
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
