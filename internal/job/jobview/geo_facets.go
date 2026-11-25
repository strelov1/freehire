package jobview

import (
	"sort"
	"strings"

	"github.com/strelov1/freehire/internal/dict/location"
)

// nonCityFallback are LLM-emitted "city" values that are really work-mode or
// open-anywhere markers. The dictionary never produces them; they leak in only through
// the enrichment.cities fallback and are dropped there. Bare-remote geography is
// carried by regions=global (see location.Parse).
var nonCityFallback = map[string]struct{}{
	"remote": {}, "remote-first": {}, "fully remote": {}, "worldwide": {},
	"anywhere": {}, "global": {}, "distributed": {}, "hybrid": {},
	"onsite": {}, "on-site": {}, "work from home": {}, "wfh": {},
}

// geoFacet builds the served country/region facets. The dictionary columns win
// whenever they pinned a place (a country, or a region more specific than "global").
// Only when geography is unpinned does it fall back wholesale to the LLM's
// enrichment.countries/regions, which catch restrictions stated only in prose
// ("Remote (SPAIN only)"). A pinned dictionary place is never overridden. Both outputs
// are lowercased, sorted, deduped and non-nil.
func geoFacet(dictCountries, dictRegions, llmCountries, llmRegions []string) (countries, regions []string) {
	countries = normalizeSet(dictCountries)
	regions = normalizeSet(dictRegions)
	if geoPinned(countries, regions) {
		return countries, regions
	}
	// The LLM writes regions as free text ("us", "emea", "gmt+1 to gmt+5"), so they
	// are folded into the region vocabulary before they can reach the facet.
	llmC := normalizeSet(llmCountries)
	llmR := location.NormalizeRegions(llmRegions)
	if llmR == nil {
		llmR = []string{}
	}
	if len(llmC) == 0 && len(llmR) == 0 {
		return countries, regions // the LLM knows no more than the dict — keep it
	}
	return llmC, llmR
}

// geoPinned reports whether the dictionary resolved a concrete place: a country, or a
// region more specific than the open-anywhere "global" bucket.
func geoPinned(countries, regions []string) bool {
	if len(countries) > 0 {
		return true
	}
	for _, r := range regions {
		if r != "global" {
			return true
		}
	}
	return false
}

// cityFacet builds the served city facet: the dictionary cities when present,
// otherwise a normalized fallback to the LLM's enrichment.cities (trimmed, ", Country"
// suffix dropped, deduped). Case is preserved: the value is its own display label.
func cityFacet(dict, llm []string) []string {
	if len(dict) > 0 {
		return dedupeSortedPreserveCase(dict)
	}
	cleaned := make([]string, 0, len(llm))
	for _, c := range llm {
		c = strings.TrimSpace(c)
		if i := strings.IndexByte(c, ','); i >= 0 {
			c = strings.TrimSpace(c[:i])
		}
		if c == "" {
			continue
		}
		if _, bad := nonCityFallback[strings.ToLower(c)]; bad {
			continue
		}
		cleaned = append(cleaned, c)
	}
	return dedupeSortedPreserveCase(cleaned)
}

// dedupeSortedPreserveCase sorts and dedupes case-insensitively, keeping the first-seen
// spelling. Non-nil so the facet serializes as [] not null; used for display-name cities.
func dedupeSortedPreserveCase(a []string) []string {
	seen := make(map[string]struct{}, len(a))
	out := make([]string, 0, len(a))
	for _, v := range a {
		k := strings.ToLower(v)
		if _, ok := seen[k]; ok {
			continue
		}
		seen[k] = struct{}{}
		out = append(out, v)
	}
	sort.Strings(out)
	return out
}
