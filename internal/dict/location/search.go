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

// loadCitySearchIndex walks the TSV in file order (population-sorted, same as
// loadCityDict), resolving each row independently of every other row so two places that
// happen to share a plain canonical name — e.g. two "Springfield"s in different countries
// — each keep their own entry rather than colliding, unlike an alias-keyed lookup.
//
// An override renames a row only when (a) the override's country matches the row's own
// raw country, and (b) no earlier (more populous) row has already claimed that same
// override target. A GeoNames alternate-name list can legitimately list an incidental
// name shared with an unrelated place — sometimes in the very country the override
// targets: Frankfurt (Oder), DE and Frankfurt am Main, DE both list bare "frankfurt" as
// an alternate name, so the country guard alone cannot tell them apart, but only the
// larger, earlier-in-file Frankfurt am Main is the "frankfurt" -> Frankfurt override's
// intended target. Without the claim guard, the smaller Frankfurt (Oder) would also be
// renamed to "Frankfurt" and then dedupe away entirely as a false duplicate.
//
// Rows that resolve to the same final (Name, Country) — including a row an override
// left untouched but that happens to equal another row's raw identity — collapse to the
// first (most populous) occurrence, keeping that occurrence's own alias list for the
// alias-prefix fallback.
//
// Caveat for whoever adds the next override: this assumes the curated target is always
// the most populous same-country row carrying the override's alias — true for every
// entry in cityOverrides today (checked against the full dataset), but unverified by any
// test. If that ever stops holding, the bigger, unrelated place would silently claim the
// override's display name instead of the intended city — a mislabeling, not a
// disappearance, but still worth a spot-check (like the Frankfurt (Oder) one below) when
// curating a new alias that could plausibly collide.
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
