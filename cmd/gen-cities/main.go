// Command gen-cities regenerates internal/location/cities1000.tsv from the
// GeoNames cities1000 dump (populated places with population >= 1,000; CC-BY 4.0,
// https://www.geonames.org/).
//
// The threshold is 1,000 rather than 15,000 deliberately, and it makes the parser
// MORE conservative rather than less. The dictionary states a city's country only
// when exactly one country claims the name, so coverage and caution move together:
// at 15,000 the only Taft in the file is Iranian and the parser confidently
// mislabels a Californian one, while at 1,000 the Iranian, Filipino and Californian
// Tafts all appear, the name becomes contested, and it correctly says nothing. The
// same fix arrives for "Somerset", "San" and "Young". It also stops losing genuine
// small places: Ilminster (pop. 5,808) is why "Ilminster, Somerset" used to resolve
// to the United States.
//
// For each place it keeps a canonical display name, the
// ISO 3166-1 alpha-2 country code, and the filtered lowercase lookup aliases (name,
// ASCII name, and alternate names). Places are emitted sorted by population
// descending so the loader's first-wins alias registration picks the most-populous
// place for a shared name. Run it with `make gen-cities`; the output is committed,
// so the build never depends on this tool.
package main

//go:generate go run .

import (
	"archive/zip"
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	dumpURL    = "https://download.geonames.org/export/dump/cities1000.zip"
	dumpFile   = "cities1000.txt"
	outputPath = "internal/location/cities1000.tsv"
)

// GeoNames geoname-table column indices (tab-separated).
const (
	colName       = 1
	colASCII      = 2
	colAlternates = 3
	colCountry    = 8
	colPopulation = 14
	minColumns    = 15
)

type place struct {
	name    string
	country string
	pop     int64
	aliases []string
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "gen-cities:", err)
		os.Exit(1)
	}
}

func run() error {
	raw, err := download(dumpURL)
	if err != nil {
		return err
	}
	txt, err := unzipMember(raw, dumpFile)
	if err != nil {
		return err
	}
	places, err := parse(txt)
	if err != nil {
		return err
	}
	// Most-populous first, name-tie-broken, so the loader's first-wins registration
	// is deterministic and the diff is stable.
	sort.SliceStable(places, func(i, j int) bool {
		if places[i].pop != places[j].pop {
			return places[i].pop > places[j].pop
		}
		return places[i].name < places[j].name
	})
	return writeTSV(outputPath, places)
}

func download(url string) ([]byte, error) {
	client := &http.Client{Timeout: 2 * time.Minute}
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: %s", url, resp.Status)
	}
	return io.ReadAll(resp.Body)
}

func unzipMember(zipBytes []byte, member string) ([]byte, error) {
	zr, err := zip.NewReader(bytes.NewReader(zipBytes), int64(len(zipBytes)))
	if err != nil {
		return nil, err
	}
	for _, f := range zr.File {
		if f.Name != member {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return nil, err
		}
		defer rc.Close()
		return io.ReadAll(rc)
	}
	return nil, fmt.Errorf("%s not found in archive", member)
}

// parse turns the GeoNames dump into places carrying a filtered alias set, dropping
// rows with no country, no aliases, or too few columns.
func parse(txt []byte) ([]place, error) {
	var places []place
	sc := bufio.NewScanner(bytes.NewReader(txt))
	sc.Buffer(make([]byte, 0, 1024*1024), 1024*1024)
	for sc.Scan() {
		fields := strings.Split(sc.Text(), "\t")
		if len(fields) < minColumns {
			continue
		}
		country := strings.ToLower(strings.TrimSpace(fields[colCountry]))
		if country == "" {
			continue
		}
		aliases := buildAliases(fields[colName], fields[colASCII], fields[colAlternates])
		if len(aliases) == 0 {
			continue
		}
		pop, _ := strconv.ParseInt(strings.TrimSpace(fields[colPopulation]), 10, 64)
		places = append(places, place{
			name:    strings.TrimSpace(fields[colName]),
			country: country,
			pop:     pop,
			aliases: aliases,
		})
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return places, nil
}

// statingPopulation is the population a place must reach to be WRITTEN OUT. Smaller
// places still shape the file — they are what makes a shared name contested — but only
// as evidence, and they are dropped before writing. See contestedAliases.
const statingPopulation = 15000

// contestedAliases returns the aliases claimed by more than one country across EVERY
// place in the dump, hamlets included.
//
// Computing this here rather than in the loader is what keeps the dictionary small.
// The parser only ever states places above statingPopulation, so shipping the smaller
// ones would grow the embedded file fivefold and the process-start parse from ~44ms to
// ~205ms — all to recover one bit per alias that is already known at generation time.
func contestedAliases(places []place) map[string]bool {
	claimedBy := make(map[string]string, len(places)*4)
	contested := map[string]bool{}
	for _, p := range places {
		for _, alias := range p.aliases {
			switch prev, seen := claimedBy[alias]; {
			case !seen:
				claimedBy[alias] = p.country
			case prev != p.country:
				contested[alias] = true
			}
		}
	}
	return contested
}

// aliasesWithContestMarks renders a place's aliases, suffixing "*" on the contested
// ones. The marker rides along with each alias so the format stays one line per place.
func aliasesWithContestMarks(aliases []string, contested map[string]bool) string {
	marked := make([]string, len(aliases))
	for i, a := range aliases {
		if contested[a] {
			a += "*"
		}
		marked[i] = a
	}
	return strings.Join(marked, "|")
}

func writeTSV(path string, places []place) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	contested := contestedAliases(places)
	var b strings.Builder
	b.WriteString("# Code generated by cmd/gen-cities from GeoNames cities1000 (CC-BY 4.0). DO NOT EDIT.\n")
	b.WriteString("# Regenerate with `make gen-cities`. Columns: canonical-name<TAB>country<TAB>alias|alias|...\n")
	b.WriteString("# A trailing '*' marks an alias more than one country claims, counted across the\n")
	b.WriteString("# WHOLE dump including places too small to appear here. Such an alias states no country.\n")
	for _, p := range places {
		if p.pop < statingPopulation {
			continue // evidence only: it contested what it needed to, and is not written
		}
		b.WriteString(p.name)
		b.WriteByte('\t')
		b.WriteString(p.country)
		b.WriteByte('\t')
		b.WriteString(aliasesWithContestMarks(p.aliases, contested))
		b.WriteByte('\n')
	}
	return os.WriteFile(path, []byte(b.String()), 0o644)
}
