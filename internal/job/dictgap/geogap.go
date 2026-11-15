package dictgap

import (
	"slices"
	"sort"
	"strings"

	"github.com/strelov1/freehire/internal/dict/location"
)

// LocationCountries is one group of jobs sharing a raw location string and the same
// LLM-recorded country list. The same location may repeat (one row per country list, per
// chunk); GeoGapCandidates folds them together.
type LocationCountries struct {
	Location     string
	LLMCountries []string
	Count        int
}

// GeoGapCandidate is one location string where the location dictionary and LLM enrichment
// answer the country question differently.
//
// LLMShare is the fraction of the location's jobs that named LLMCountry, the most-named
// country. It is the confidence signal, since the model guesses for bare "Remote"
// locations. DictionaryCountries is what location.Parse resolves; empty on a gap.
type GeoGapCandidate struct {
	Location            string
	Count               int
	LLMCountry          string
	LLMShare            float64
	DictionaryCountries []string
}

// GeoGapReport separates two findings with opposite fixes: a gap is a name the dictionary
// is missing, a conflict is a name it may resolve wrongly (the `georgia` shape).
type GeoGapReport struct {
	// Gaps are locations location.Parse places nowhere (no country and no region) while
	// the model named a country. A region-only answer ("Europe") is a placement.
	Gaps []GeoGapCandidate
	// Conflicts are locations where location.Parse names countries but most jobs name
	// none of them.
	Conflicts []GeoGapCandidate
}

// GeoGapCandidates recomputes location.Parse for every distinct location, so it reflects
// the dictionary as it stands today, and compares it with the LLM-recorded countries.
// Locations fold case- and space-insensitively, as Parse does. Lists rank by job count,
// then location.
func GeoGapCandidates(rows []LocationCountries) GeoGapReport {
	type tally struct {
		total     int
		byCountry map[string]int
		// rows stay whole so each group can be judged against the dictionary's answer.
		rows []LocationCountries
	}
	byLocation := make(map[string]*tally)
	for _, r := range rows {
		if len(r.LLMCountries) == 0 || r.Count <= 0 {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(r.Location))
		if key == "" {
			continue
		}
		t, ok := byLocation[key]
		if !ok {
			t = &tally{byCountry: make(map[string]int)}
			byLocation[key] = t
		}
		t.total += r.Count
		for _, c := range normalizeCountryCodes(r.LLMCountries) {
			t.byCountry[c] += r.Count
		}
		t.rows = append(t.rows, r)
	}

	var report GeoGapReport
	for key, t := range byLocation {
		top, topCount := mostNamed(t.byCountry)
		candidate := GeoGapCandidate{
			Location:   key,
			Count:      t.total,
			LLMCountry: top,
			LLMShare:   float64(topCount) / float64(t.total),
		}
		geo := location.Parse(key)
		if len(geo.Countries) == 0 {
			if len(geo.Regions) == 0 {
				report.Gaps = append(report.Gaps, candidate)
			}
			continue
		}
		agree, disagree := 0, 0
		for _, r := range t.rows {
			if namesAny(normalizeCountryCodes(r.LLMCountries), geo.Countries) {
				agree += r.Count
			} else {
				disagree += r.Count
			}
		}
		if disagree > agree {
			candidate.DictionaryCountries = geo.Countries
			report.Conflicts = append(report.Conflicts, candidate)
		}
	}
	sortGeoCandidates(report.Gaps)
	sortGeoCandidates(report.Conflicts)
	return report
}

// normalizeCountryCodes lowercases and trims the model's codes and drops blanks and
// repeats. It must not use location.NormalizeCountry, which returns "" for a country
// outside the curated set, the very countries this report must surface.
func normalizeCountryCodes(codes []string) []string {
	out := make([]string, 0, len(codes))
	for _, c := range codes {
		c = strings.ToLower(strings.TrimSpace(c))
		if c != "" && !slices.Contains(out, c) {
			out = append(out, c)
		}
	}
	return out
}

// mostNamed returns the country named by the most jobs, ties broken by code.
func mostNamed(byCountry map[string]int) (string, int) {
	best, bestCount := "", 0
	for c, n := range byCountry {
		if n > bestCount || (n == bestCount && c < best) {
			best, bestCount = c, n
		}
	}
	return best, bestCount
}

func namesAny(codes, want []string) bool {
	for _, c := range codes {
		if slices.Contains(want, c) {
			return true
		}
	}
	return false
}

func sortGeoCandidates(c []GeoGapCandidate) {
	sort.Slice(c, func(i, j int) bool {
		if c[i].Count != c[j].Count {
			return c[i].Count > c[j].Count
		}
		return c[i].Location < c[j].Location
	})
}
