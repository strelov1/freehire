package location

import (
	"bufio"
	"strings"
)

// citySearchMaxResults bounds a single SearchCities call, matching the cap the other
// remote-search facets (company, subindustry) already apply to their result lists.
const citySearchMaxResults = 20

// CityMatch is one city-search result: a resolved place's canonical display data.
type CityMatch struct {
	Name    string
	Country string
}

// citySearchEntry is one deduplicated place in population order, carrying its own
// lowercase aliases so a search can fall back to alias-prefix matching. Distinct from
// cityDict (an alias -> entry map with no ordering) because search needs both order and,
// for a given place, every alias that reaches it.
type citySearchEntry struct {
	cityEntry
	aliases []string
}

// citySearchIndex is built once at init from the same embedded TSV cityDict uses, with
// the same curated overrides layered on top, so its display names match what cityDict
// itself resolves for the same place.
var citySearchIndex = loadCitySearchIndex(citiesTSV, cityOverrides)

// loadCitySearchIndex walks the TSV in file order (population-sorted, like loadCityDict),
// resolving each row independently so two places sharing a plain name ("Springfield" in two
// countries) keep their own entries.
//
// An override renames a row only when (a) the override's country matches the row's raw country
// and (b) no earlier (more populous) row has already claimed that override target. A GeoNames
// alternate-name list can list an incidental name shared with an unrelated place in the same
// country: Frankfurt (Oder) and Frankfurt am Main both list bare "frankfurt", but only the
// larger Frankfurt am Main is the intended target. Without the claim guard the smaller one
// would be renamed too and dedupe away.
//
// Rows resolving to the same final (Name, Country) collapse to the first (most populous),
// keeping its alias list for the alias-prefix fallback.
//
// Caveat for the next override: this assumes the curated target is the most populous same-
// country row carrying the alias. No test checks it; if it stops holding, the bigger unrelated
// place silently claims the display name.
func loadCitySearchIndex(tsv string, overrides map[string]cityEntry) []citySearchEntry {
	seen := map[cityEntry]bool{}
	claimed := map[cityEntry]bool{} // override targets already assigned to a row
	var out []citySearchEntry
	sc := bufio.NewScanner(strings.NewReader(tsv))
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for sc.Scan() {
		line := strings.TrimSuffix(sc.Text(), "\r")
		if line == "" || line[0] == '#' {
			continue
		}
		parts := strings.SplitN(line, "\t", 3)
		if len(parts) < 3 {
			continue
		}
		aliases := strings.Split(parts[2], "|")
		final := cityEntry{Name: parts[0], Country: parts[1]}
		if final.Country == "in" {
			final.Name = foldIndianName(final.Name)
		}
		for _, alias := range aliases {
			if ov, ok := overrides[alias]; ok && ov.Country == final.Country && !claimed[ov] {
				final = ov
				claimed[ov] = true
				break
			}
		}
		if seen[final] {
			continue
		}
		seen[final] = true
		out = append(out, citySearchEntry{cityEntry: final, aliases: aliases})
	}
	return out
}

// SearchCities returns the cities whose canonical name or a known alias starts with
// query (case-insensitively), most-populous first, optionally restricted to one ISO
// 3166-1 alpha-2 country. A blank query returns nothing.
func SearchCities(query, countryCode string, limit int) []CityMatch {
	return searchCitiesIn(citySearchIndex, query, countryCode, limit)
}

// CanonicalCity resolves a city spelling (a local-language name such as "München" or
// "Köln", or the canonical name itself) to the canonical display name Parse puts into
// Geo.Cities. It exists so a filter value typed in a local spelling reaches the facet
// value the index actually carries: postings are indexed under "Munich", so an exact
// match on "München" finds almost nothing. The dictionary is curated, never guessed, so
// an unknown spelling reports false and the caller keeps the value verbatim.
func CanonicalCity(alias string) (string, bool) {
	ce, ok := cityDict[strings.ToLower(strings.TrimSpace(alias))]
	if !ok {
		return "", false
	}
	return ce.Name, true
}

// searchCitiesIn is SearchCities' pure matching logic over an injected index, so tests
// can exercise it against a small fixture instead of the embedded 34k-row dataset.
func searchCitiesIn(index []citySearchEntry, query, countryCode string, limit int) []CityMatch {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return nil
	}
	country := strings.ToLower(countryCode)

	out := make([]CityMatch, 0, limit)
	matched := map[cityEntry]bool{}
	add := func(e citySearchEntry) {
		out = append(out, CityMatch{Name: e.Name, Country: e.Country})
		matched[e.cityEntry] = true
	}

	// Pass 1: canonical-name prefix, population order.
	for _, e := range index {
		if len(out) >= limit {
			return out
		}
		if country != "" && e.Country != country {
			continue
		}
		if strings.HasPrefix(strings.ToLower(e.Name), q) {
			add(e)
		}
	}

	// Pass 2: alias prefix fallback, for places pass 1 didn't already surface.
	for _, e := range index {
		if len(out) >= limit {
			return out
		}
		if (country != "" && e.Country != country) || matched[e.cityEntry] {
			continue
		}
		for _, alias := range e.aliases {
			if strings.HasPrefix(alias, q) {
				add(e)
				break
			}
		}
	}

	return out
}
