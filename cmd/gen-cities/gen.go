package main

import (
	"strings"
	"unicode"

	"github.com/strelov1/freehire/internal/stringset"
)

// stoplist holds lowercase aliases that must never resolve to a city: the parser's
// own work-mode and open-anywhere markers, plus macro-region / continent words. A
// GeoNames place that happens to carry one of these names contributes no alias, so
// a location token like "Remote" or "Europe" stays a work-mode / region signal
// rather than misfiring a city.
//
// These mirror the marker sets in internal/location (noiseTokenWords, workModeMarkers,
// nameToRegion). They are duplicated deliberately: this is a build-time dev tool and
// cannot import that package's unexported maps. A miss is contained — the parser's
// country-agreement guard already rejects a region-word token that carries no country —
// but keep the two in rough sync when adding a marker there.
var stoplist = map[string]struct{}{
	// Work-mode markers.
	"remote": {}, "hybrid": {}, "onsite": {}, "on-site": {}, "on site": {},
	"wfh": {}, "work from home": {}, "home": {}, "office": {}, "hq": {}, "headquarters": {},
	// Open-anywhere markers.
	"anywhere": {}, "worldwide": {}, "world wide": {}, "global": {}, "globally": {},
	"international": {}, "distributed": {}, "everywhere": {},
	// Macro-region / continent words the region dictionary owns.
	"europe": {}, "europa": {}, "asia": {}, "asia pacific": {}, "asia-pacific": {},
	"africa": {}, "americas": {}, "america": {}, "north america": {}, "latin america": {},
	"south america": {}, "middle east": {}, "central asia": {},
	"apac": {}, "emea": {}, "latam": {}, "mena": {}, "cis": {},
}

// normalizeAlias lowercases a raw name and collapses internal whitespace, yielding
// the canonical lookup form used as a dictionary key.
func normalizeAlias(raw string) string {
	return strings.ToLower(strings.Join(strings.Fields(raw), " "))
}

// keepAlias reports whether a normalized alias is safe to use as a city key. It
// drops empties, tokens with no letter, tokens carrying a digit (postal codes /
// ZIP-bearing forms), tokens shorter than three runes (which collide with ISO /
// subdivision codes), and stoplisted markers.
func keepAlias(a string) bool {
	if a == "" {
		return false
	}
	if _, stop := stoplist[a]; stop {
		return false
	}
	letters := 0
	for _, r := range a {
		if unicode.IsDigit(r) {
			return false
		}
		if unicode.IsLetter(r) {
			// Keep only Latin/Cyrillic scripts: an IT job's location field is written
			// in one of these, and admitting every GeoNames script (CJK, Arabic, Indic,
			// Greek, …) only bloats the embedded dictionary with aliases that never match.
			if !unicode.Is(unicode.Latin, r) && !unicode.Is(unicode.Cyrillic, r) {
				return false
			}
			letters++
		}
	}
	if letters == 0 {
		return false
	}
	return len([]rune(a)) >= 3
}

// isShortCode reports whether a raw alternate name is an all-uppercase code of at
// most three letters (an IATA / abbreviation such as "MOW" or "FLN"), which is
// noise rather than a place name.
func isShortCode(raw string) bool {
	raw = strings.TrimSpace(raw)
	n := 0
	for _, r := range raw {
		if !unicode.IsLetter(r) || !unicode.IsUpper(r) {
			return false
		}
		n++
	}
	return n > 0 && n <= 3
}

// buildAliases turns a GeoNames row's name, ASCII name, and comma-separated
// alternate-name list into the sorted, deduped, filtered set of lookup aliases.
// Short uppercase codes are dropped before normalization; the rest pass through
// normalizeAlias + keepAlias.
func buildAliases(name, ascii, alternates string) []string {
	set := map[string]struct{}{}
	add := func(raw string) {
		if isShortCode(raw) {
			return
		}
		if a := normalizeAlias(raw); keepAlias(a) {
			set[a] = struct{}{}
		}
	}
	add(name)
	add(ascii)
	for _, alt := range strings.Split(alternates, ",") {
		add(alt)
	}
	return stringset.Sorted(set)
}
