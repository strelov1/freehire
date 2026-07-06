// Package location derives a job's geography — ISO 3166-1 alpha-2 country codes
// and region codes — and a work-mode hint deterministically from the free-text
// ATS location string.
//
// It is a curated dictionary, not a geocoder: it resolves the high-frequency
// country names, ATS shorthands ("USA", "UK"), macro-region names ("Europe",
// "APAC"), and a few beacon cities that real ATS location fields use, and emits
// nothing for anything it cannot resolve (it never guesses). Region codes are
// drawn from the same controlled vocabulary the enrichment contract defines
// (enrich.RegionValues), and work modes from enrich.WorkModeValues, so the
// parser, the enrichment payload, and the search facet all speak one set of
// values.
package location

import (
	"strings"

	"github.com/strelov1/freehire/internal/stringset"
)

// Geo is the geography parsed from a location string: zero or more country codes
// and region codes, and an optional work-mode hint. Each field is empty when the
// location states nothing the parser can resolve.
type Geo struct {
	Countries []string
	Regions   []string
	Cities    []string // canonical city names for resolved beacon cities; empty when none
	WorkMode  string   // "", "remote", "hybrid", or "onsite" — only on an explicit marker
}

// separatorReplacer normalizes every token separator to a comma in one pass so a
// single Split yields the geography tokens. The multi-character forms (" - ",
// " or ") and parentheses are included, so "Berlin (On-site)" -> "berlin",
// "on-site".
var separatorReplacer = strings.NewReplacer(
	";", ",", "/", ",", "|", ",", "(", ",", ")", ",", " - ", ",", " or ", ",",
)

// Parse maps a location string to its geography. Countries/regions are
// deduplicated and sorted; nil when nothing resolves. WorkMode is set only from
// an explicit marker, while a plain city/country yields geography with no
// WorkMode. A remote job that resolves NO geography (a bare "Remote", "WFH", …)
// is open-anywhere, so it falls into the "global" region — its remoteness stays
// on WorkMode (the separate work-type facet), which the global region never
// displaces. A remote marker alongside a real place ("US Remote") keeps that
// place and is not globalized.
func Parse(location string) Geo {
	lower := strings.ToLower(location)

	s := separatorReplacer.Replace(lower)

	countrySet := map[string]struct{}{}
	regionSet := map[string]struct{}{}
	citySet := map[string]struct{}{}
	for _, tok := range strings.Split(s, ",") {
		tok = strings.TrimSpace(tok)
		if tok == "" {
			continue
		}
		tok = stripCityPrefix(tok)
		// Strip embedded work-mode words so the place still resolves ("US Remote" ->
		// "us"); a token that is ONLY a work-mode marker ("Remote", "On-site") strips
		// to "" and is skipped — its work mode is detected separately from the whole
		// string. Skipping also keeps it out of the dash-split below.
		tok = stripWorkmodeWords(tok)
		if tok == "" {
			continue
		}
		// Beacon-city facet: a recognized city alias emits its canonical display name
		// (independent of the country/region resolution below, which also fires for a
		// city via nameToCountry). Unknown cities fall through — the served city facet
		// backfills them from the LLM at serve time (jobview), never from a guess here.
		if c, ok := nameToCity[tok]; ok {
			citySet[c] = struct{}{}
		}
		if resolveGeoToken(tok, countrySet, regionSet) {
			continue
		}
		// Dash-delimited exports carry the geography either first ("United
		// States-Utah-Roy", "TX-Houston") or last ("Nisku-Alberta-Canada"). Every
		// non-leading segment is resolved by NAME only, so a 2-letter code buried in a
		// hyphenated city name ("stoke-on-trent" -> "on") cannot misfire while a
		// country/region word ("alberta", "canada", "china") still does. The leading
		// segment gets the same name-only treatment first; a bare 2-letter code there
		// ("tx" in "TX-Houston") is accepted only when a following segment also
		// resolved — i.e. a real geographic dash-export, not a hyphenated common word
		// ("in-house", "de-witt") whose first segment merely happens to be a code.
		// Tried only after the whole token failed, so "cluj-napoca"/"nur-sultan"
		// (dictionary keys) still win as a unit.
		if segs := strings.Split(tok, "-"); len(segs) > 1 {
			tailResolved := false
			for _, seg := range segs[1:] {
				if resolveGeoName(strings.TrimSpace(seg), countrySet, regionSet) {
					tailResolved = true
				}
			}
			lead := strings.TrimSpace(segs[0])
			if !resolveGeoName(lead, countrySet, regionSet) && tailResolved {
				resolveGeoToken(lead, countrySet, regionSet)
			}
		}
	}

	countries := stringset.Sorted(countrySet)
	regions := stringset.Sorted(regionSet)
	mode := detectWorkMode(lower)

	// A remote job that resolved no country and no region is open-anywhere: treat it
	// as the global region so it joins the Global/Worldwide bucket instead of the
	// "geography not specified" one. Only fires when nothing else resolved, so
	// "US Remote" stays north_america and "Remote - Germany" stays eu.
	if mode == "remote" && len(countries) == 0 && len(regions) == 0 {
		regions = []string{"global"}
	}

	return Geo{
		Countries: countries,
		Regions:   regions,
		Cities:    stringset.Sorted(citySet),
		WorkMode:  mode,
	}
}

// resolveGeoToken resolves one already-normalized token to a country and/or
// region, writing into the sets, and reports whether anything matched. Order: a
// country/city name, a macro-region name, a US/Canada subdivision, then a bare
// ISO 3166-1 alpha-2 country code (last, so a same-spelled subdivision wins).
func resolveGeoToken(tok string, countrySet, regionSet map[string]struct{}) bool {
	if tok == "" {
		return false
	}
	if code, ok := nameToCountry[tok]; ok {
		countrySet[code] = struct{}{}
		if r, ok := countryToRegion[code]; ok {
			regionSet[r] = struct{}{}
		}
		return true
	}
	if r, ok := nameToRegion[tok]; ok {
		regionSet[r] = struct{}{}
		return true
	}
	if code, ok := resolveSubdivision(tok); ok {
		countrySet[code] = struct{}{}
		if r, ok := countryToRegion[code]; ok {
			regionSet[r] = struct{}{}
		}
		return true
	}
	if r, ok := countryToRegion[tok]; ok {
		countrySet[tok] = struct{}{}
		regionSet[r] = struct{}{}
		return true
	}
	return false
}

// resolveGeoName resolves a token by place NAME only — a country/city name, a
// macro-region name, or a full (len>2) US/Canada subdivision name. It deliberately
// skips bare 2-letter codes (subdivision or ISO), so it is safe to run on every
// non-leading dash segment of a hyphenated city ("stoke-on-trent") without "on"
// or "in" misfiring.
func resolveGeoName(tok string, countrySet, regionSet map[string]struct{}) bool {
	if code, ok := nameToCountry[tok]; ok {
		countrySet[code] = struct{}{}
		if r, ok := countryToRegion[code]; ok {
			regionSet[r] = struct{}{}
		}
		return true
	}
	if r, ok := nameToRegion[tok]; ok {
		regionSet[r] = struct{}{}
		return true
	}
	if len(tok) > 2 {
		if code, ok := subdivisionToCountry[tok]; ok {
			countrySet[code] = struct{}{}
			if r, ok := countryToRegion[code]; ok {
				regionSet[r] = struct{}{}
			}
			return true
		}
	}
	return false
}

// cityMarkerPrefixes are the Russian "city" abbreviations that RU-segment ATS
// data prepends to a bare city name ("г Москва", "город Самара"). Stripped from a
// token before lookup so the city resolves; checked longest-first so "город "
// wins over "г ". A city whose name merely starts with "г" ("Грозный") is
// untouched — every prefix ends in a separator the name doesn't.
var cityMarkerPrefixes = []string{"город ", "г. ", "г.", "г "}

// noiseTokenWords are dropped from a geography token so an embedded place still
// resolves: work-mode words ("US Remote" -> "us") and site suffixes ("San
// Francisco Office" / "... HQ" -> "san francisco"). Matched as whole
// space-separated words; the work mode is detected separately (detectWorkMode).
var noiseTokenWords = map[string]struct{}{
	"remote": {}, "hybrid": {}, "onsite": {}, "on-site": {},
	"office": {}, "hq": {}, "headquarters": {},
}

// stripWorkmodeWords drops any noise words from a token, returning the rest
// (possibly ""). "us remote" -> "us"; "remote" -> ""; "san francisco office" ->
// "san francisco".
func stripWorkmodeWords(tok string) string {
	fields := strings.Fields(tok)
	kept := fields[:0]
	for _, f := range fields {
		if _, drop := noiseTokenWords[f]; drop {
			continue
		}
		kept = append(kept, f)
	}
	return strings.Join(kept, " ")
}

// stripCityPrefix removes a leading Russian city marker from an already-lowercased,
// trimmed token, returning the bare city name (or the token unchanged).
func stripCityPrefix(tok string) string {
	for _, p := range cityMarkerPrefixes {
		if rest, ok := strings.CutPrefix(tok, p); ok {
			return strings.TrimSpace(rest)
		}
	}
	return tok
}

// resolveSubdivision resolves a US-state / Canadian-province token to its ISO
// country code, covering the "City, ST ZIP" and "City, Province" ATS formats. It
// tries, in order: a direct match ("tx", "texas", "ontario"); a trailing US ZIP
// preceded by a state code ("tx 76135" -> "tx"); a bare trailing code in a
// multi-word token ("austin tx"); and a standalone US ZIP ("94105") as a us
// signal. It returns ("", false) for anything it cannot resolve — it never
// guesses past the curated subdivision table.
func resolveSubdivision(tok string) (string, bool) {
	if code, ok := subdivisionToCountry[tok]; ok {
		return code, true
	}
	fields := strings.Fields(tok)
	switch len(fields) {
	case 0:
		return "", false
	case 1:
		if isUSZip(fields[0]) {
			return "us", true
		}
		return "", false
	}
	last := fields[len(fields)-1]
	if isUSZip(last) {
		if code, ok := subdivisionToCountry[fields[len(fields)-2]]; ok {
			return code, true
		}
		return "us", true
	}
	if code, ok := subdivisionToCountry[last]; ok {
		return code, true
	}
	return "", false
}

// isUSZip reports whether s is a US ZIP code: five digits, optionally followed by
// a "-" and the four-digit ZIP+4 extension ("76135" or "76135-1234").
func isUSZip(s string) bool {
	switch len(s) {
	case 5:
		return allDigits(s)
	case 10:
		return s[5] == '-' && allDigits(s[:5]) && allDigits(s[6:])
	default:
		return false
	}
}

func allDigits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return s != ""
}

// workModeMarkers maps a work mode to the substrings that signal it, checked in
// priority order: hybrid (most specific) beats a remote marker in the same
// string, and an explicit onsite marker is the last resort. A location with no
// marker yields "" — onsite is never assumed from a bare city.
var workModeMarkers = []struct {
	mode    string
	markers []string
}{
	{"hybrid", []string{"hybrid", "гибрид"}},
	{"remote", []string{"remote", "work from home", "wfh", "anywhere", "worldwide", "distributed", "удал"}},
	{"onsite", []string{"on-site", "onsite", "on site", "in office", "in-office"}},
}

// detectWorkMode scans the whole lowercased location for a work-mode marker,
// independent of tokenization so a marker embedded in a token ("Berlin
// (On-site)") is still found.
func detectWorkMode(lower string) string {
	for _, wm := range workModeMarkers {
		for _, m := range wm.markers {
			if strings.Contains(lower, m) {
				return wm.mode
			}
		}
	}
	return ""
}
