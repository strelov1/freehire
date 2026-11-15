package location

import (
	"strings"
)

// The words a geography token is cleaned of before it is resolved as a place: city
// markers, work-mode and site words, prepositions and area phrases.

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

// siteWords are the noiseTokenWords that name a site rather than a work mode.
var siteWords = map[string]bool{"office": true, "hq": true, "headquarters": true}

// siteDescriptors say which KIND of site the word after them is, so they go with it: a
// school district's "Central Office" is its administration building, not Central,
// Louisiana, and a "Branch Office" is not Branch, Arkansas. Cutting the site word alone
// left the descriptor to be read as a town.
var siteDescriptors = map[string]bool{
	"central": true, "main": true, "head": true, "home": true, "district": true,
	"regional": true, "corporate": true, "branch": true, "field": true, "front": true,
	"back": true, "satellite": true, "administrative": true, "admin": true,
}

// trimSiteDescriptors drops the run of siteDescriptors closing words ("central regional"),
// stopping at a town whose own name ends in one: "Olive Branch Office" is Olive Branch.
func trimSiteDescriptors(words []string) []string {
	for n := len(words); n > 0 && siteDescriptors[words[n-1]]; n-- {
		if n > 1 && knownAsWritten(strings.Join(words[:n], " ")) {
			return words[:n]
		}
		words = words[:n-1]
	}
	return words
}

// stripWorkmodeWords drops any noise words from a token, returning the rest
// (possibly ""). "us remote" -> "us"; "remote" -> ""; "san francisco office" ->
// "san francisco"; "central office" -> "" (see siteDescriptors).
func stripWorkmodeWords(tok string) string {
	fields := strings.Fields(tok)
	kept := fields[:0]
	for _, f := range fields {
		if _, drop := noiseTokenWords[f]; drop {
			if siteWords[f] {
				kept = trimSiteDescriptors(kept)
			}
			continue
		}
		kept = append(kept, f)
	}
	return strings.Join(kept, " ")
}

// placePrepositions introduce the area a remote line is open to ("Remote within Canada",
// "Based in Germany", "Remote across LATAM").
var placePrepositions = map[string]bool{
	"in": true, "within": true, "from": true, "across": true, "throughout": true,
}

// placeLeadWords are the words that may stand before such a preposition — a work mode or a
// phrase introducing a place ("Anywhere in Europe", "Work from anywhere in Canada", "Fully
// remote, based in"). Any other word is a place of its own ("Kirchheim in Schwaben") and
// keeps the token whole.
var placeLeadWords = map[string]bool{
	"remote": true, "hybrid": true, "onsite": true, "on-site": true, "fully": true,
	"anywhere": true, "based": true, "located": true, "only": true, "work": true,
	"working": true, "from": true,
}

// stripPlacePreposition returns the country or region name after a lead-in preposition
// ("remote within canada" -> "canada", "anywhere in the us" -> "us"), or tok unchanged.
// Only a name the dictionary resolves is taken — the preposition is cut to read a stated
// place, never to guess one ("in zzyzx", "in the world" stay as they are). A bare "IN"
// opening the token is also Indiana's code ("IN Lebanon", "IN Angola"), so with nothing
// before it a name the US has a town of is left alone.
func stripPlacePreposition(tok string) string {
	fields := strings.Fields(tok)
	for k, f := range fields {
		if !placePrepositions[f] {
			continue
		}
		lead := fields[:k]
		if !allLeadWords(lead) {
			return tok
		}
		bareIn := len(lead) == 0 && f == "in"
		// The name as written first ("The Woodlands" is a Texan town), then without the
		// article ("the United States").
		name := stripWorkmodeWords(strings.Join(fields[k+1:], " "))
		if namesPlace(name, bareIn) {
			return name
		}
		if rest, ok := strings.CutPrefix(name, "the "); ok && namesPlace(rest, bareIn) {
			return rest
		}
	}
	return tok
}

// allLeadWords reports whether every word is a placeLeadWords entry.
func allLeadWords(words []string) bool {
	for _, w := range words {
		if !placeLeadWords[w] {
			return false
		}
	}
	return true
}

// namesPlace reports whether name is a country, city or region name. A US state or
// Canadian province is left out: resolveSubdivision already reads one closing a token
// ("Remote in California"), and cut loose it would also name a town ("Oregon"). A city of
// the main table counts under its own name only, never an alternate one ("market" is
// GeoNames' alias of Marquette). After a bare "IN", which is also Indiana's code, only a
// curated name the US has no town of counts: "IN Lebanon", "IN Munster" are Indiana towns.
func namesPlace(name string, bareIn bool) bool {
	if isSubdivisionName(name) {
		return false
	}
	if bareIn {
		_, country := nameToCountry[name]
		_, region := nameToRegion[name]
		return (country || region) && !hasPlaceIn(name, "us")
	}
	if _, ok := nameToCountry[name]; ok {
		return true
	}
	if ce, ok := cityDict[name]; ok && sameName(ce.Name, name) {
		return true
	}
	_, ok := nameToRegion[name]
	return ok
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
		"umkreis ", "standort ", "kanton ", "regio ", "omgeving ", "okolica ", "v okolici "}
	areaSuffixes = []string{" und umgebung", " und umland", " en omgeving", " e.o.",
		" in okolica", " z okolico", " z bližnjo okolico", " in bližnja okolica",
		" in širša okolica", " z širšo okolico"}
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
