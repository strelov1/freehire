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
	"regexp"
	"strings"

	"github.com/strelov1/freehire/internal/platform/stringset"
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
	lower := strings.ToLower(location)

	// A token that opened a parenthesis is marked, so it can be told apart from a place
	// in its own right once the separators are gone: see parenMark.
	s := separatorReplacer.Replace(strings.ReplaceAll(lower, "(", "("+parenMark))

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
	// Subdivision codes the line states, and the cities it names, are collected over the
	// whole line: the state usually FOLLOWS the city ("Roseville, CA").
	statedSubdivisions := map[string]struct{}{}
	// pending holds the tokens that name a city only in a country the line states, resolved
	// once the whole line has been read (resolvePending).
	var pending []pendingCity
	for _, tok := range strings.Split(s, ",") {
		tok = strings.TrimSpace(tok)
		tok, parenthetical := strings.CutPrefix(tok, parenMark)
		tok = strings.TrimSpace(tok)
		if tok == "" {
			continue
		}
		tok = stripCityPrefix(tok)
		tok = stripAreaWords(tok)
		tok = stripPostalPrefix(tok)
		// Strip embedded work-mode words so the place still resolves ("US Remote" ->
		// "us"); a token that is ONLY a work-mode marker ("Remote", "On-site") strips
		// to "" and is skipped — its work mode is detected separately from the whole
		// string. Skipping also keeps it out of the dash-split below.
		tok = stripWorkmodeWords(tok)
		if tok == "" {
			continue
		}
		// A UK postcode states the country on its own, the way a US ZIP does, and is cut
		// out so the place it follows still resolves ("london sw1a 2hb" -> "london").
		if rest, ok := stripUKPostcode(tok); ok {
			countrySet["gb"] = struct{}{}
			regionSet[countryToRegion["gb"]] = struct{}{}
			if tok = rest; tok == "" {
				continue
			}
		}
		// An Australian state code closing the token ("perth wa", the form SEEK writes with
		// no comma) states the country and is cut off, so the place before it resolves as
		// a city of its own, with its city name and metro.
		if rest, ok := stripAustralianState(tok); ok {
			countrySet["au"] = struct{}{}
			regionSet[countryToRegion["au"]] = struct{}{}
			tok = rest
		}
		// Curated geography first, into the scratch sets (authoritative for country/region).
		clear(tokCountry)
		clear(tokRegion)
		// An Australian state is not a US/CA subdivision: recording the "wa" of "Perth, WA"
		// would key the metro lookup to Washington and find nothing in Perth.
		if _, ok := resolveSubdivision(tok, prevTok); ok && !isAustralianState(tok, prevTok) {
			if code, ok := statedSubdivisionCode(tok); ok {
				statedSubdivisions[code] = struct{}{}
			}
		}
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
		if ce, ok := cityDict[tok]; ok && !regionNotCity[tok] {
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
		if !resolved && !parenthetical {
			// Not a place either table states. It may still NAME one once the line has
			// stated a country (a small town beside its Land, "Köln-Niehl"); see pendingFor.
			if p := pendingFor(tok); p != nil {
				pending = append(pending, p)
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
			if !tailResolved {
				// "TX-El Paso", "FL-Orlando": a US/CA subdivision code leading a city the
				// curated maps do not name but GeoNames places in the US or Canada. The
				// city is the confirmation, so "on-site", "co-op" and "hi-tech" — whose
				// tails are no city — still resolve nothing.
				if code, ok := subdivisionToCountry[lead]; ok && len(lead) == 2 &&
					isRecognizedUSCACity(strings.TrimSpace(strings.Join(segs[1:], "-"))) {
					countrySet[code] = struct{}{}
					if r, ok := countryToRegion[code]; ok {
						regionSet[r] = struct{}{}
					}
				}
			}
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

	// Before the long-tail fallback below, never after: a pending city waits for a country
	// the line STATED, and a country inferred from another city is not that.
	resolvePending(pending, countrySet, citySet)

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

	// A metro needs one stated country: a line naming two cannot place a city in either
	// without guessing.
	var metros []string
	if len(countrySet) == 1 && len(citySet) > 0 {
		var country string
		for c := range countrySet {
			country = c
		}
		metroSet := map[string]struct{}{}
		for name := range citySet {
			if v, ok := metroFor(name, country, statedSubdivisions); ok {
				metroSet[v] = struct{}{}
			}
		}
		metros = stringset.Sorted(metroSet)
	}

	countries := stringset.Sorted(countrySet)
	regions := stringset.Sorted(regionSet)
	mode := detectWorkMode(lower)

	return Geo{
		Countries: countries,
		Regions:   regions,
		Cities:    stringset.Sorted(citySet),
		WorkMode:  mode,
		Metros:    metros,
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

// areaPrefixes name the area AROUND a place ("Großraum Hannover", "Region Stuttgart",
// "Raum München", "Standort Erkheim", "Kanton Bern"); areaSuffixes do the same from the
// other side ("Stuttgart und Umgebung"). The place is what remains. French "Canton" is
// not here: it is a city in Massachusetts and Ohio ("Canton MA"). English "Greater" is
// deliberately not here: "Greater London" and "Greater Manchester" are subdivision names in
// their own right (foreignSubdivisionNames), and stripping would hand "Manchester" to the
// contested city table.
var (
	areaPrefixes = []string{"großraum ", "grossraum ", "metropolregion ", "region ", "raum ",
		"umkreis ", "standort ", "kanton "}
	areaSuffixes = []string{" und umgebung", " und umland"}
)

// stripAreaWords removes one areaPrefixes word and one areaSuffixes phrase from a token.
func stripAreaWords(tok string) string {
	for _, p := range areaPrefixes {
		if rest, ok := strings.CutPrefix(tok, p); ok {
			tok = strings.TrimSpace(rest)
			break
		}
	}
	for _, s := range areaSuffixes {
		if rest, ok := strings.CutSuffix(tok, s); ok {
			return strings.TrimSpace(rest)
		}
	}
	return tok
}

// postalPrefixRE matches a continental postal code written BEFORE its town: German and
// Austrian/Swiss style, optionally behind a country letter ("85462 Eitting", "CH-3400
// Burgdorf", "A-1010 Wien", "D-80331 München"). The code states no country on its own —
// four and five digits are half of Europe's formats — so it is only cut away for the town
// to resolve. A US ZIP comes after its state and is untouched (isUSZip).
var postalPrefixRE = regexp.MustCompile(`^(?:[a-z]{1,2}-)?\d{4,5}\s+(\pL.*)$`)

// stripPostalPrefix returns the town after a leading postal code, or the token unchanged.
func stripPostalPrefix(tok string) string {
	if m := postalPrefixRE.FindStringSubmatch(tok); m != nil {
		return m[1]
	}
	return tok
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

// australianStateCodes are the postal abbreviations of Australia's states and territories.
// The value says whether the code is Australia's alone: "nsw" or "qld" can only mean an
// Australian state, while "wa" is also Washington, "nt" the Northwest Territories, "sa"
// Saudi Arabia's ISO code and "act" an English word.
var australianStateCodes = map[string]bool{
	"nsw": true, "vic": true, "qld": true, "tas": true,
	"wa": false, "sa": false, "nt": false, "act": false,
}

// isAustralianState reports whether code, stated after place, is an Australian state:
// the comma form passes the preceding token as the place ("Perth, WA"), the no-comma form
// the token's own leading words (stripAustralianState). A code only Australia uses needs
// nothing more than a place before it; a shared one needs that place to be a city the dictionary
// already puts in Australia, so "Seattle, WA" stays Washington and "Yellowknife, NT" stays
// Canadian.
func isAustralianState(code, place string) bool {
	only, ok := australianStateCodes[code]
	if !ok || place == "" {
		return false
	}
	return only || isRecognizedCityOf(place, "au")
}

// stripAustralianState cuts an Australian state code off the end of a multi-word token
// ("perth wa" -> "perth", "bondi junction nsw" -> "bondi junction"), under the same rule
// isAustralianState applies with the leading words as the place.
func stripAustralianState(tok string) (string, bool) {
	fields := strings.Fields(tok)
	if len(fields) < 2 {
		return tok, false
	}
	place := strings.Join(fields[:len(fields)-1], " ")
	if !isAustralianState(fields[len(fields)-1], place) {
		return tok, false
	}
	return place, true
}

// subdivisionAccepted reports whether a matched subdivision code should win over
// its identically-spelled country-code reading. A code outside
// collidingSubdivisions is unambiguous and always accepted; a colliding one is
// accepted when cityTok is isRecognizedUSCACity — or, for a stateFirstSubdivisions
// code, when there is a city at all and the dictionary does not place it in the
// colliding country itself.
func subdivisionAccepted(code, cityTok string) bool {
	if _, ambiguous := collidingSubdivisions[code]; !ambiguous {
		return true
	}
	if isRecognizedUSCACity(cityTok) {
		return true
	}
	if _, stateFirst := stateFirstSubdivisions[code]; stateFirst && cityTok != "" {
		return !isRecognizedCityOf(cityTok, code)
	}
	return false
}

// isRecognizedCityOf reports whether tok names a place the parser already knows is
// in the given country — the same two lookups isRecognizedUSCACity makes. It is what
// keeps "Casablanca, MA" Moroccan once MA defaults to Massachusetts.
func isRecognizedCityOf(tok, country string) bool {
	if code, ok := nameToCountry[tok]; ok {
		return code == country
	}
	if ce, ok := cityDict[tok]; ok {
		return ce.Country == country
	}
	return false
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

// ukPostcodePattern matches a full UK postcode as a whole word: an outward code (area
// letters, district digit, optional sub-district) and an inward code (a digit and two
// letters, never C, I, K, M, O or V). Only the FULL form is read — an outward code alone
// ("sw1", "b2") is indistinguishable from an ordinary token.
var ukPostcodePattern = regexp.MustCompile(`\b([a-z]{1,2})\d[a-z\d]? \d[abd-hjlnp-uw-z]{2}\b`)

// ukPostcodeAreas are the postcode areas of the United Kingdom. GY, JE and IM (Guernsey,
// Jersey, the Isle of Man) are left out: they are Crown dependencies with their own ISO
// codes, not gb. BF (forces mail) names no place.
var ukPostcodeAreas = map[string]struct{}{
	"ab": {}, "al": {}, "b": {}, "ba": {}, "bb": {}, "bd": {}, "bh": {}, "bl": {}, "bn": {},
	"br": {}, "bs": {}, "bt": {}, "ca": {}, "cb": {}, "cf": {}, "ch": {}, "cm": {}, "co": {},
	"cr": {}, "ct": {}, "cv": {}, "cw": {}, "da": {}, "dd": {}, "de": {}, "dg": {}, "dh": {},
	"dl": {}, "dn": {}, "dt": {}, "dy": {}, "e": {}, "ec": {}, "eh": {}, "en": {}, "ex": {},
	"fk": {}, "fy": {}, "g": {}, "gl": {}, "gu": {}, "ha": {}, "hd": {}, "hg": {}, "hp": {},
	"hr": {}, "hs": {}, "hu": {}, "hx": {}, "ig": {}, "ip": {}, "iv": {}, "ka": {}, "kt": {},
	"kw": {}, "ky": {}, "l": {}, "la": {}, "ld": {}, "le": {}, "ll": {}, "ln": {}, "ls": {},
	"lu": {}, "m": {}, "me": {}, "mk": {}, "ml": {}, "n": {}, "ne": {}, "ng": {}, "nn": {},
	"np": {}, "nr": {}, "nw": {}, "ol": {}, "ox": {}, "pa": {}, "pe": {}, "ph": {}, "pl": {},
	"po": {}, "pr": {}, "rg": {}, "rh": {}, "rm": {}, "s": {}, "sa": {}, "se": {}, "sg": {},
	"sk": {}, "sl": {}, "sm": {}, "sn": {}, "so": {}, "sp": {}, "sr": {}, "ss": {}, "st": {},
	"sw": {}, "sy": {}, "ta": {}, "td": {}, "tf": {}, "tn": {}, "tq": {}, "tr": {}, "ts": {},
	"tw": {}, "ub": {}, "w": {}, "wa": {}, "wc": {}, "wd": {}, "wf": {}, "wn": {}, "wr": {},
	"ws": {}, "wv": {}, "yo": {}, "ze": {},
}

// stripUKPostcode reports whether tok carries a UK postcode and returns tok without it.
// An inward code that reads as an ordinal ("b2 2nd floor") is not a postcode: the letters
// are legal, so only the shape tells them apart, and precision wins over the rare real
// postcode ending in ST, ND, RD or TH.
func stripUKPostcode(tok string) (string, bool) {
	if !strings.ContainsAny(tok, "0123456789") {
		return tok, false
	}
	for _, m := range ukPostcodePattern.FindAllStringSubmatchIndex(tok, -1) {
		if _, ok := ukPostcodeAreas[tok[m[2]:m[3]]]; !ok {
			continue
		}
		switch tok[m[1]-2 : m[1]] {
		case "st", "nd", "rd", "th":
			continue
		}
		return strings.Trim(strings.Join(strings.Fields(tok[:m[0]]+" "+tok[m[1]:]), " "), "."), true
	}
	return tok, false
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
