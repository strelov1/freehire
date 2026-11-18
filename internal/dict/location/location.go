// Package location derives a job's geography — ISO 3166-1 alpha-2 country codes
// and region codes — and a work-mode hint deterministically from the free-text
// ATS location string.
//
// It is a curated dictionary, not a geocoder: it resolves the high-frequency
// country names, ATS shorthands ("USA", "UK"), macro-region names ("Europe",
// "APAC"), and a few beacon cities that real ATS location fields use, and emits
// nothing for anything it cannot resolve (it never guesses). Region codes are
// drawn from the same controlled vocabulary the enrichment contract defines
// (vocab.RegionValues), and work modes from vocab.WorkModeValues, so the
// parser, the enrichment payload, and the search facet all speak one set of
// values.
package location

import (
	"slices"
	"strings"
)

// Geo is the geography parsed from a location string: zero or more country codes
// and region codes, and an optional work-mode hint. Each field is empty when the
// location states nothing the parser can resolve.
type Geo struct {
	Countries []string
	Regions   []string
	Cities    []string // canonical city names for resolved beacon cities; empty when none
	WorkMode  string   // "", "remote", "hybrid", or "onsite" — only on an explicit marker
	Metros    []string // metro values ("Sacramento (US)"); dict-only, see metros.go
}

// parenMark prefixes a token that followed "(" in the location line. A parenthesis
// qualifies the place before it — "Schönebeck (Elbe)", "Halle (Saale)", "Kempten
// (Allgäu)" — and the qualifier is a river or a region that the small-places table may
// also list as a village ("Elbe" is one, in Lower Saxony). Such a token still resolves
// through the curated tables ("Berlin (Germany)"), but is never read as a small place.
const parenMark = "\x01"

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
// states nothing about where it is open to, so it stays in the "region not
// specified" bucket (empty Regions/Countries) — never guessed into "global".
// An explicit open-anywhere word ("Anywhere", "Worldwide", "International", …)
// is a curated dictionary entry in its own right (see nameToRegion) and still
// resolves to the global region through the ordinary token loop below. A remote
// marker alongside a real place ("US Remote") keeps that place, as before.
func Parse(location string) Geo {
	return ParseIn(location, nil)
}

// resolveGeoToken resolves one already-normalized token to a country and/or
// region, writing into the sets, and reports whether anything matched. Order: a
// country/city name, a macro-region name, a US/Canada subdivision, then a bare
// ISO 3166-1 alpha-2 country code (last, so a same-spelled subdivision wins) —
// except for a colliding subdivision code, where resolveSubdivision itself defers
// to prevTok and, unconfirmed, leaves the bare-code fallback to win instead.
// prevTok is the preceding comma-token (already stripped; "" when none), the
// context that confirms a colliding code as a real US/CA subdivision reading.
func resolveGeoToken(tok, prevTok string, countrySet, regionSet map[string]struct{}) bool {
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
	if isAustralianState(tok, prevTok) {
		countrySet["au"] = struct{}{}
		regionSet[countryToRegion["au"]] = struct{}{}
		return true
	}
	if code, ok := resolveSubdivision(tok, prevTok); ok {
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

// workModeMarkers maps a work mode to the substrings that signal it, checked in
// priority order: hybrid (most specific) beats a remote marker in the same
// string, and an explicit onsite marker is the last resort. A location with no
// marker yields "" — onsite is never assumed from a bare city.
var workModeMarkers = []struct {
	mode    string
	markers []string
}{
	// The Portuguese and Spanish words ("Remoto, Remoto, BR", "São Paulo - SP - Presencial")
	// are spelled with and without the accent; a location line is short, so the bare
	// stem cannot meet the "soporte remoto" sense a description has.
	{"hybrid", []string{"hybrid", "гибрид", "híbrid", "hibrid"}},
	{"remote", []string{"remote", "work from home", "wfh", "anywhere", "worldwide", "distributed", "удал", "remoto", "remota"}},
	// "WFO" is India's "work from office".
	{"onsite", []string{"on-site", "onsite", "on site", "in office", "in-office", "work from office", "wfo", "presencial"}},
}

// detectWorkMode scans the whole lowercased location for a work-mode marker,
// independent of tokenization so a marker embedded in a token ("Berlin
// (On-site)") is still found. A Recruitee remote label (remoteLabels) counts only as the
// whole line.
func detectWorkMode(lower string) string {
	if remoteLabels[strings.TrimSpace(lower)] {
		return "remote"
	}
	for _, wm := range workModeMarkers {
		for _, m := range wm.markers {
			if strings.Contains(lower, m) {
				return wm.mode
			}
		}
	}
	return ""
}

// statedSubdivisionCode returns the postal code a token that resolveSubdivision accepted
// states: the token itself ("ca"), its last field ("san francisco ca"), or the field
// before a trailing ZIP ("tx 76135").
func statedSubdivisionCode(tok string) (string, bool) {
	if code, ok := subdivisionCode[tok]; ok {
		return code, true
	}
	fields := strings.Fields(tok)
	if n := len(fields); n > 0 && isUSZip(fields[n-1]) {
		fields = fields[:n-1]
	}
	if len(fields) == 0 {
		return "", false
	}
	code, ok := subdivisionCode[fields[len(fields)-1]]
	return code, ok
}

// withdrawNamesake withdraws the countries the town before a US state or Canadian province
// stated (lastCountries), when the subdivision's country code is not among them: "Paris,
// TX", "London, ON", "Dublin, CA, US" name the North American namesake. A country another
// token also stated stays, and so does a town that is itself a province ("Saint-Laurent,
// Quebec, CA").
func withdrawNamesake(code, town string, lastCountries []string, votes map[string]int, countrySet, regionSet map[string]struct{}) {
	if len(lastCountries) == 0 || slices.Contains(lastCountries, code) || isSubdivisionName(town) {
		return
	}
	for _, c := range lastCountries {
		if votes[c]--; votes[c] == 0 {
			withdrawCountry(c, countrySet, regionSet)
		}
	}
}
