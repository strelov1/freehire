package location

import (
	"regexp"
	"strings"
)

// postalPrefixRE matches a continental postal code written BEFORE its town: German and
// Austrian/Swiss style, optionally behind a country letter ("85462 Eitting", "CH-3400
// Burgdorf", "A-1010 Wien", "D-80331 München"), or Polish ("02-797 Warszawa"). The code states no country on its own —
// four and five digits are half of Europe's formats — so it is only cut away for the town
// to resolve. A US ZIP comes after its state and is untouched (isUSZip).
var postalPrefixRE = regexp.MustCompile(`^(?:[a-z]{1,2}-)?(?:\d{4,5}|\d{2}-\d{3})\s+(\pL.*)$`)

// dutchPostcodeRE matches the Dutch postcode before its town: four digits and two
// letters ("1012 AB Amsterdam", "3011AA Rotterdam"). Like postalPrefixRE it states no
// country; it is cut away so the town resolves.
var dutchPostcodeRE = regexp.MustCompile(`^\d{4}\s?[a-z]{2}\s+(\pL.*)$`)

// stripPostalPrefix returns the town after a leading postal code, or the token unchanged.
func stripPostalPrefix(tok string) string {
	// The Dutch form first: postalPrefixRE would take its letters for the town.
	if m := dutchPostcodeRE.FindStringSubmatch(tok); m != nil {
		return m[1]
	}
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
	// A US town below the main city table's floor ("Atkins, AR", "Mountain Home, AR"):
	// the state, unless the colliding country has a place of that name too.
	if _, us := smallPlaceCandidates(cityTok)["us"]; us && !hasPlaceIn(cityTok, code) {
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
