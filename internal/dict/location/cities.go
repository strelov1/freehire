package location

import (
	"bufio"
	_ "embed"
	"strings"
)

// citiesTSV is the GeoNames-derived city dataset from cmd/gen-cities, committed so
// the build never depends on that tool. Tab-separated: canonical-name, ISO country
// code, '|'-joined lowercase aliases. Rows are population-sorted so first-wins alias
// registration picks the largest place for a shared name.
//
//go:embed cities1000.tsv
var citiesTSV string

// cityEntry is a resolved city: its canonical display name (feeds the cities facet),
// its ISO 3166-1 alpha-2 country code, and whether more than one country claims the
// alias.
//
// Country is always the most-populous claimant, because isRecognizedUSCACity asks
// whether the parser knows the name as a North American place, where a shared name is
// still evidence. Contested gates *emitting* the country as the posting's geography:
// "Birmingham" (gb, us) and "Valencia" (six countries) state nothing, while "Colorado
// Springs" and "Benidorm" state theirs.
type cityEntry struct {
	Name      string
	Country   string
	Contested bool
}

// cityDict maps a lowercase city alias to its canonical name and country, built at
// init from the embedded dataset with the curated overrides layered on top.
var cityDict, cityRenamedFrom = pinCityAliases(loadCityDict(citiesTSV, withSlovenianCities(cityOverrides)))

// contestMark suffixes an alias in the dataset when more than one country claims it.
// cmd/gen-cities counts across the WHOLE GeoNames dump, hamlets included (that is what
// makes "Taft" and "Somerset" contested), then writes only places large enough for the
// parser to state; counting there keeps the embedded file small.
const contestMark = "*"

// loadCityDict parses the embedded TSV into the alias lookup. The first alias seen
// wins (most-populous, since the file is population sorted); an alias carrying
// contestMark states no country. Overrides are applied last and win outright,
// uncontested: they are hand-pinned spellings, so their country is asserted.
//
// An override renames the PLACE, not just its own alias: every alias that reached the
// place the override's alias resolved to takes the curated name too, otherwise a city
// splits across two facet values ("Frankfurt" vs "Frankfurt am Main"). The place is
// the first-wins (most populous) one, so Frankfurt (Oder) keeps its own name.
//
// The second result maps each curated name (lowercased, with its country) back to the
// GeoNames name it replaced, which is what the metro table is keyed by (metroFor).
func loadCityDict(tsv string, overrides map[string]cityEntry) (map[string]cityEntry, map[[2]string]string) {
	dict := map[string]cityEntry{}
	sc := bufio.NewScanner(strings.NewReader(tsv))
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20) // the widest alias row (Shanghai) is a few KB
	for sc.Scan() {
		line := strings.TrimSuffix(sc.Text(), "\r") // tolerate a CRLF checkout
		if line == "" || line[0] == '#' {
			continue
		}
		parts := strings.SplitN(line, "\t", 3)
		if len(parts) < 3 {
			continue
		}
		name := parts[0]
		if parts[1] == "in" {
			name = foldIndianName(name)
		}
		for _, alias := range strings.Split(parts[2], "|") {
			contested := strings.HasSuffix(alias, contestMark)
			alias = strings.TrimSuffix(alias, contestMark)
			if _, seen := dict[alias]; !seen {
				dict[alias] = cityEntry{Name: name, Country: parts[1], Contested: contested}
			}
		}
	}
	renamedFrom := map[[2]string]string{}
	for alias, ov := range overrides {
		base, ok := dict[alias]
		if !ok || base.Country != ov.Country || base.Name == ov.Name {
			continue
		}
		for a, e := range dict {
			if e.Name == base.Name && e.Country == base.Country {
				e.Name = ov.Name
				dict[a] = e
			}
		}
		renamedFrom[[2]string{strings.ToLower(ov.Name), ov.Country}] = strings.ToLower(base.Name)
	}
	for alias, e := range overrides {
		dict[alias] = e
	}
	return dict, renamedFrom
}
