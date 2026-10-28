package location

import (
	"bufio"
	"regexp"
	"strings"
	"sync"
)

// French location lines, measured 2026-10-28 over 33k open postings placed in France
// (wttj, flatchr, werecruit, smartrecruiters, teamtailor, workday, eures, recruitee,
// lever, join, workable, greenhouse, personio, whatjobs-fr). What the parser missed came
// in a few shapes, each handled here:
//
//   - a compound town written with spaces instead of hyphens ("Aix en Provence",
//     "Boulogne Billancourt", "Issy les Moulineaux, France"), which the tables know only
//     hyphenated (hyphenFolded);
//   - an arrondissement ("Lyon 3e", "Paris 8e arrondissement", "9th arrondissement of
//     Paris"), which is the city (frenchPlaceToken);
//   - a French postcode on its own ("Paris, 75009, France", "Toulouse (31000)"), which
//     the US ZIP reading turned into a second country (foreignPostcodeLine);
//   - a hyphenated small town whose last word is a US state or a country ("Meslay-du-
//     Maine", "Porto-Vecchio"), split by the dash-export reading (see ParseIn);
//   - a region or a department beside or instead of the town ("Limoux, Occitanie",
//     "Haute-Savoie, Auvergne-Rhône-Alpes"), in foreignSubdivisionNames;
//   - a town GeoNames gives another country first ("Valence" is Valencia's French name),
//     which the line's own country now names (contestedCityIn).

// arrondissementRE is a city written with its arrondissement: "Lyon 3e", "Paris 8ème",
// "Marseille 13e arrondissement", "Paris 15". Only the three cities that have them.
var arrondissementRE = regexp.MustCompile(`^(paris|lyon|marseille)\s*(?:-\s*)?\d{1,2}\s*(?:e|er|eme|ème|ieme|ième)?(?:\s+arr(?:ondissement|\.)?)?$`)

// arrondissementOfRE is the English form GeoNames-style exports write: "9th
// arrondissement of Paris".
var arrondissementOfRE = regexp.MustCompile(`^\d{1,2}(?:e|er|eme|ème|st|nd|rd|th)\s+arrondissement\s+(?:of|de)\s+(paris|lyon|marseille)$`)

// frenchPlaceToken returns the place a French token names: the city of an
// arrondissement, La Défense for "Paris La Défense". Anything else is returned unchanged.
func frenchPlaceToken(tok string) string {
	if m := arrondissementRE.FindStringSubmatch(tok); m != nil {
		return m[1]
	}
	if m := arrondissementOfRE.FindStringSubmatch(tok); m != nil {
		return m[1]
	}
	switch tok {
	case "paris la défense", "paris la defense", "paris-la défense", "paris-la-défense":
		return "la défense"
	}
	return tok
}

// hyphenFoldedCities and hyphenFoldedSmallPlaces map a hyphenated alias with its hyphens
// written as spaces back to the alias: "aix en provence" → "aix-en-provence". A spaced
// form two different aliases share is dropped rather than guessed.
var (
	hyphenFoldedCities = sync.OnceValue(func() map[string]string {
		return hyphenFoldIndex(func(yield func(string)) {
			for k := range cityDict {
				yield(k)
			}
		})
	})
	hyphenFoldedSmallPlaces = sync.OnceValue(func() map[string]string {
		return hyphenFoldIndex(func(yield func(string)) {
			for k := range smallPlaces() {
				yield(k)
			}
		})
	})
)

func hyphenFoldIndex(keys func(func(string))) map[string]string {
	out := map[string]string{}
	ambiguous := map[string]bool{}
	keys(func(k string) {
		if !strings.Contains(k, "-") {
			return
		}
		f := strings.ReplaceAll(k, "-", " ")
		if prev, ok := out[f]; ok && prev != k {
			ambiguous[f] = true
		}
		out[f] = k
	})
	for f := range ambiguous {
		delete(out, f)
	}
	return out
}

// hyphenFolded returns the hyphenated alias a token spelled with spaces names ("issy les
// moulineaux" → "issy-les-moulineaux"), or the token unchanged when the tables already
// know it as written or know no hyphenated form of it.
func hyphenFolded(tok string) string {
	if !strings.Contains(tok, " ") || knownAsWritten(tok) {
		return tok
	}
	f := strings.ReplaceAll(tok, "-", " ")
	if k, ok := hyphenFoldedCities()[f]; ok {
		return k
	}
	if k, ok := hyphenFoldedSmallPlaces()[f]; ok {
		return k
	}
	return tok
}

// knownAsWritten reports whether a table already reads tok as it is.
func knownAsWritten(tok string) bool {
	if _, ok := nameToCountry[tok]; ok {
		return true
	}
	if _, ok := nameToRegion[tok]; ok {
		return true
	}
	if _, ok := subdivisionToCountry[tok]; ok {
		return true
	}
	if _, ok := cityDict[tok]; ok {
		return true
	}
	return len(smallPlaces()[tok]) > 0
}

// foreignPostcodeLine reports whether a five-digit token in this line is a postcode of
// the country it names rather than a US ZIP: the line names no part of the US, and
// either states another country ("Paris, 75009, France", "Toulouse (31000), Occitanie")
// or comes from a caller whose countries do not include the US.
func foreignPostcodeLine(toks []string, namesUS bool, hint map[string]struct{}) bool {
	if namesUS {
		return false
	}
	for _, tok := range toks {
		tok = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(tok), parenMark))
		if c, ok := nameToCountry[tok]; ok && c != "us" {
			return true
		}
	}
	_, us := hint["us"]
	return len(hint) > 0 && !us
}

// contestedCities maps a contested alias and a country to that country's place of the
// name: ("valence", "fr") → "Valence", where cityDict's first-wins reading of "valence"
// is Valencia. Only a place literally called by the alias counts, and only when the
// country has one of it: "Conceição" is not "Conceição do Mato Dentro", and Argentina's
// several San Martíns name none. Built on first use from the same dataset.
var contestedCities = sync.OnceValue(func() map[[2]string]string {
	out := map[[2]string]string{}
	ambiguous := map[[2]string]bool{}
	sc := bufio.NewScanner(strings.NewReader(citiesTSV))
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for sc.Scan() {
		parts := strings.SplitN(strings.TrimSuffix(sc.Text(), "\r"), "\t", 3)
		if len(parts) < 3 || parts[0] == "" || parts[0][0] == '#' {
			continue
		}
		for _, alias := range strings.Split(parts[2], "|") {
			alias, contested := strings.CutSuffix(alias, contestMark)
			if !contested {
				continue
			}
			key := [2]string{alias, parts[1]}
			if _, seen := out[key]; seen {
				ambiguous[key] = true
				continue
			}
			out[key] = parts[0]
		}
	}
	for key, name := range out {
		if ambiguous[key] || !strings.EqualFold(dropAccents(name), dropAccents(key[0])) {
			delete(out, key)
		}
	}
	return out
})

// contestedCityIn returns the place a contested alias names in country, "" when that
// country has none, or several, of the name.
func contestedCityIn(tok, country string) string {
	return contestedCities()[[2]string{tok, country}]
}

// flagRE is a flag emoji: a pair of regional indicator symbols, whose letters are the
// country's ISO code ("Lyon 🇫🇷").
var flagRE = regexp.MustCompile(`[\x{1F1E6}-\x{1F1FF}]{2}`)

// flagsToCodes writes every flag emoji as a comma token of its ISO code.
func flagsToCodes(s string) string {
	if !flagRE.MatchString(s) {
		return s
	}
	return flagRE.ReplaceAllStringFunc(s, func(f string) string {
		var b strings.Builder
		for _, r := range f {
			b.WriteRune('a' + (r - 0x1F1E6))
		}
		return ", " + b.String() + ","
	})
}

// heldContested is a contested city alias held back until the line states a country,
// with the token that named it.
type heldContested struct {
	ce  cityEntry
	tok string
}

// isSubdivisionName reports whether tok is a US state or Canadian province by name or
// code — a place that is a province before it is a town ("Quebec", "Ontario").
func isSubdivisionName(tok string) bool {
	_, ok := subdivisionToCountry[tok]
	return ok
}
