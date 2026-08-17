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
	// Reused per-token scratch sets: a token's curated geography is resolved here first
	// so the city-name agreement check sees only THIS token's country (order-independent),
	// then merged into the result.
	tokCountry := map[string]struct{}{}
	tokRegion := map[string]struct{}{}
	// prevTok is the previous comma-token, after the same stripping, and is the
	// disambiguating context resolveGeoToken needs for a colliding subdivision code
	// ("Tel Aviv, IL" vs "Chicago, IL"). Updated at the end of the loop body.
	prevTok := ""
	// cityFallbackCountries collects the countries of the unambiguous long-tail cities
	// seen, held back until the whole string has been read and applied only if nothing
	// else stated one AND they agree. See its use below the loop.
	cityFallbackCountries := map[string]struct{}{}
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
		// Curated geography first, into the scratch sets (authoritative for country/region).
		clear(tokCountry)
		clear(tokRegion)
		resolved := resolveGeoToken(tok, prevTok, tokCountry, tokRegion)
		// Recorded now, before any of the branches below (including the early
		// "continue" on a resolved token) that would otherwise skip past the
		// end-of-loop assignment and leave the next token's collision check
		// looking at a stale or empty prevTok.
		prevTok = tok
		// City facet from the generated dictionary (cmd/gen-cities). cityDict supplies the
		// canonical display NAME only — never a country/region — so an ambiguous city name
		// ("Birmingham") can never *guess* a geography here; the country/region stay the
		// curated dictionaries' job (and the LLM's, at serve time). This keeps the parser's
		// "never guesses" contract while still populating the cities facet broadly.
		if ce, ok := cityDict[tok]; ok {
			if resolved {
				// The token is a curated place: emit the city name only when cityDict agrees
				// on its country, so a country/region token ("usa") never emits the unrelated
				// city buried in its GeoNames alternate names ("Yokkaichi").
				if _, agree := tokCountry[ce.Country]; agree {
					citySet[ce.Name] = struct{}{}
				}
			} else {
				// A long-tail city the curated maps do not place ("Recife", "Joinville"):
				// emit its name for the facet now, and remember its country as a
				// LAST-RESORT candidate. It is applied after the whole string is read
				// and only if nothing else stated a country — see cityFallback below.
				citySet[ce.Name] = struct{}{}
				resolved = true
				if !ce.Contested && ce.Country != "" {
					cityFallbackCountries[ce.Country] = struct{}{}
				}
			}
		}
		for c := range tokCountry {
			countrySet[c] = struct{}{}
		}
		for r := range tokRegion {
			regionSet[r] = struct{}{}
		}
		if resolved {
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
				// The dash-tail is the context that confirms a colliding lead code
				// ("il" in "IL-Cupertino") as a real US/CA subdivision, the same role
				// the preceding comma-token plays for "City, XX" — without it, tailResolved
				// having already added "us" via the tail's own city-name match would leave
				// the lead's country-code reading (Israel) alongside it, garbling the result.
				resolveGeoToken(lead, strings.Join(segs[1:], " "), countrySet, regionSet)
			}
		}
	}

	// Nothing in the line stated a country, so the long-tail cities get to — but only
	// with one voice. Two guards, and the change is wrong without either:
	//
	// The country must be unstated. A city is the WEAKEST geographic statement in a
	// location line, so it never contributes alongside a stated country: "Anna,
	// Illinois, United States" names a town in Russia AND the state that actually
	// places the job, and "Crossroads - London" names a US locality beside the city
	// that matters.
	//
	// The cities must agree. Each is individually unambiguous, but a line naming two
	// of them ("Recife, Benidorm") is not, and picking the first would make the answer
	// depend on word order — a guess wearing a determinism costume.
	if len(countrySet) == 0 && len(cityFallbackCountries) == 1 {
		for country := range cityFallbackCountries {
			countrySet[country] = struct{}{}
			if r, ok := countryToRegion[country]; ok {
				regionSet[r] = struct{}{}
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

// cityMarkerPrefixes are the "city" abbreviations that Cyrillic-writing sources
// prepend to a bare city name — Russian "г Москва" / "город Самара", Ukrainian
// "м. Львів" / "місто Київ". Stripped from a token before lookup so the city
// resolves; checked longest-first so "город " wins over "г ". A city whose name
// merely starts with "г" or "м" ("Грозный", "Мурманск") is untouched — every
// prefix ends in a separator the name doesn't.
var cityMarkerPrefixes = []string{"город ", "місто ", "г. ", "м. ", "г.", "м.", "г ", "м "}

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
//
// A direct or trailing-code match that lands on a collidingSubdivisions code (a
// code that also spells a curated country, e.g. "il") is accepted only when
// prevTok — the city portion, either the preceding comma-token for a direct match
// or the token's own leading words for a trailing-code match — names a recognized
// US/CA place (subdivisionAccepted); otherwise it is rejected here so the caller
// falls through to the bare-country-code reading ("Tel Aviv, IL" -> Israel, not
// Illinois). The ZIP-preceded branch is never gated: a ZIP code is a US signal on
// its own, so "il 60601" is unambiguous regardless of context.
func resolveSubdivision(tok, prevTok string) (string, bool) {
	if code, ok := subdivisionToCountry[tok]; ok {
		if !subdivisionAccepted(tok, prevTok) {
			return "", false
		}
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
		leading := strings.Join(fields[:len(fields)-1], " ")
		if !subdivisionAccepted(last, leading) {
			return "", false
		}
		return code, true
	}
	return "", false
}

// subdivisionAccepted reports whether a matched subdivision code should win over
// its identically-spelled country-code reading. A code outside
// collidingSubdivisions is unambiguous and always accepted; a colliding one is
// accepted only when cityTok is isRecognizedUSCACity.
func subdivisionAccepted(code, cityTok string) bool {
	if _, ambiguous := collidingSubdivisions[code]; !ambiguous {
		return true
	}
	return isRecognizedUSCACity(cityTok)
}

// isRecognizedUSCACity reports whether tok names a place the parser already knows
// is in the US or Canada — checked directly against nameToCountry and cityDict
// (not through resolveGeoToken), so a long-tail GeoNames beacon like "Baton Rouge"
// counts just as much as a curated one like "Minneapolis". This is the signal that
// disambiguates a colliding subdivision code from its identically-spelled country
// code: "Baton Rouge, LA" stays Louisiana because "baton rouge" resolves here to
// us, while "Vientiane, LA" does not, so "la" falls back to the country code Laos.
func isRecognizedUSCACity(tok string) bool {
	if code, ok := nameToCountry[tok]; ok {
		return code == "us" || code == "ca"
	}
	if ce, ok := cityDict[tok]; ok {
		return ce.Country == "us" || ce.Country == "ca"
	}
	return false
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
