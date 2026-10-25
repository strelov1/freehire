package location

import (
	"regexp"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// India is the catalogue's largest audience, and its location lines have shapes no other
// market writes. Measured 2026-10-25 over 63,305 Indian postings (naukri, zohorecruit,
// keka, freshteam, whatjobs, workday, oracle, greenhouse and 30 more sources): 97.7% already
// resolved the country, but only 78.3% a city. The gap was the shapes below, not missing
// city names — GeoNames knows nearly every Indian town.

// foldIndianName drops the transliteration marks GeoNames keeps on Indian names
// ("Thāne", "Ghāziābād", "Rājkot"). Nobody in India writes them — not the postings, not
// the searchers — so a facet value carrying them is a value nobody types. Applied to the
// display name of every Indian place, in the city table, the search index and the metro
// table alike, so all three keep agreeing on one name.
func foldIndianName(name string) string {
	if isASCII(name) {
		return name
	}
	var b strings.Builder
	for _, r := range norm.NFD.String(name) {
		if !unicode.Is(unicode.Mn, r) {
			b.WriteRune(r)
		}
	}
	return norm.NFC.String(b.String())
}

func isASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= 0x80 {
			return false
		}
	}
	return true
}

// cityAliasPins point one alias at the RIGHT place where GeoNames hands it to a
// neighbour, without renaming the neighbour (which a cityOverrides entry would do). In
// GeoNames' population order "visakhapatnam" is first listed by Rasapūdipalem, a locality
// of the city, and "secunderabad" by Raniganj, an unrelated town in West Bengal — so both
// cities were filed under the wrong name.
var cityAliasPins = map[string]cityEntry{
	"visakhapatnam": {Name: "Visakhapatnam", Country: "in"},
	"secunderabad":  {Name: "Secunderabad", Country: "in"},
}

// metroLabelOverrides rename a GHS-FUA metro whose generated label is the name of a
// locality rather than of the city that anchors it. Keys are the label after
// foldIndianName.
var metroLabelOverrides = map[string]string{
	"Rasapudipalem (IN)": "Visakhapatnam (IN)", // folded: see foldIndianName
}

// indianMetroRegions name a metropolitan region rather than a city. They state the
// country and the metro and no city: an NCR posting may sit in Delhi, Gurugram or Noida,
// and naming any one of them would be a guess.
var indianMetroRegions = map[string]string{
	"ncr":                            "Delhi (IN)",
	"delhi ncr":                      "Delhi (IN)",
	"delhi-ncr":                      "Delhi (IN)",
	"delhi ncr region":               "Delhi (IN)",
	"national capital region":        "Delhi (IN)",
	"delhi national capital region":  "Delhi (IN)",
	"greater delhi area":             "Delhi (IN)",
	"mumbai metropolitan region":     "Mumbai (IN)",
	"mumbai metropolitan region mmr": "Mumbai (IN)",
}

// indiaWidePhrases state India as a whole. "Anywhere" inside them is not a remote marker
// — a pan-India posting is one an employer may place in any of its Indian offices — so
// the phrase is cut from the line before its work mode is read.
var indiaWidePhrases = []string{
	"anywhere in india", "pan india", "pan-india", "across india", "all over india",
}

// indianAreaSuffixes are the revenue divisions and municipal forms a city name carries in
// Indian ATS exports: "Bangalore North", "Pune City", "Coimbatore South", "Mumbai
// Suburban", "Chennai City Corporation". Cut only when what remains is an INDIAN city of
// the table, so no other market's "X North" moves. "Rural" is deliberately absent:
// Bangalore Rural is a different district from the city.
var indianAreaSuffixes = []string{
	" metropolitan region", " metropolitan area", " city corporation", " municipal corporation",
	" suburban", " urban", " city", " cbd", " north", " south", " east", " west", " district",
}

// stripIndianAreaSuffix returns the Indian city a token names under one of
// indianAreaSuffixes, or the token unchanged.
func stripIndianAreaSuffix(tok string) string {
	if _, ok := cityDict[tok]; ok {
		return tok
	}
	for _, s := range indianAreaSuffixes {
		if rest, ok := strings.CutSuffix(tok, s); ok {
			rest = strings.TrimSpace(rest)
			if isIndianCity(rest) {
				return rest
			}
		}
	}
	return tok
}

// indianCodeLeadRE matches the head of a country-state-city export: "IN-KA-Bangalore",
// "IND-MM-Mumbai", "India-Karnataka-Bangalore", "IN.TN.Chennai.Guindy". The separator is a
// dash or a dot, never a space, so "in house" and "india office" are not read as exports.
var indianCodeLeadRE = regexp.MustCompile(`^(?:in|ind|india)\s*[-.]\s*(.+)$`)

// indianSpacedCodeRE matches the space-joined form ("IN MH Mumbai Eureka", "IND KA
// Bangalore"), which is only read when the second word is an Indian state code.
var indianSpacedCodeRE = regexp.MustCompile(`^(?:in|ind) ([a-z]{2}) (.+)$`)

// indianStateCodes are the vehicle-registration / ISO 3166-2:IN codes Indian exports put
// between the country and the city.
var indianStateCodes = map[string]bool{
	"ka": true, "mh": true, "tn": true, "tg": true, "ts": true, "ap": true, "dl": true,
	"hr": true, "up": true, "wb": true, "kl": true, "gj": true, "rj": true, "mp": true,
	"pb": true, "or": true, "od": true, "ch": true, "ga": true, "br": true, "jh": true,
	"as": true, "uk": true, "hp": true, "ct": true, "cg": true, "jk": true, "py": true,
}

// indianCityInExport returns the Indian city an export token names, and whether it is an
// export at all. A recognised export that names no city ("IN-Goa", "IN-Karnataka") still
// reports true with an empty city: it states the country.
func indianCityInExport(tok string) (city string, ok bool) {
	var rest string
	if m := indianCodeLeadRE.FindStringSubmatch(tok); m != nil {
		rest = m[1]
	} else if m := indianSpacedCodeRE.FindStringSubmatch(tok); m != nil && indianStateCodes[m[1]] {
		rest = m[2]
	} else {
		return "", false
	}
	segs := strings.FieldsFunc(rest, func(r rune) bool { return r == '-' || r == '.' })
	stated := false
	for _, seg := range segs {
		seg = strings.TrimSpace(seg)
		if c := leadingIndianCity(seg); c != "" {
			return c, true
		}
		if code, ok := nameToCountry[seg]; ok && code == "in" {
			stated = true
		}
		// Any two-letter segment is a code in this shape: exports use their own ("MM" for
		// Mumbai) beside the registration codes.
		if len(seg) == 2 || seg == "ind" || seg == "india" {
			continue
		}
		if !stated {
			// The segment after the country is neither a state nor a city: not an export.
			return "", false
		}
	}
	return "", stated
}

// leadingIndianCity returns the Indian city a token opens with — the whole token, or its
// first one or two words before an office name ("Bengaluru Bagmane", "Chennai DLF", "New
// Delhi IBU") — or "" when it opens with none. Only a city the curated country map
// itself places in India counts, so a long-tail GeoNames name ("Ali", "Haveli") never
// claims an office line.
func leadingIndianCity(tok string) string {
	if isIndianCity(tok) {
		return tok
	}
	fields := strings.Fields(tok)
	// "India Mumbai": the country, then the city.
	if len(fields) > 1 && fields[0] == "india" {
		if rest := strings.Join(fields[1:], " "); isIndianCity(rest) {
			return rest
		}
	}
	for n := min(2, len(fields)-1); n >= 1; n-- {
		lead, rest := strings.Join(fields[:n], " "), strings.Join(fields[n:], " ")
		// "Bangalore Rural" is another district, and "Kochi Japan" another country.
		if fields[n] == "rural" {
			return ""
		}
		if code, ok := nameToCountry[rest]; ok && code != "in" {
			return ""
		}
		if code, ok := nameToCountry[lead]; ok && code == "in" && lead != "india" {
			if _, isCity := cityDict[lead]; isCity {
				return lead
			}
		}
	}
	return ""
}

// isIndianCity reports whether tok is an alias the city table places in India.
func isIndianCity(tok string) bool {
	ce, ok := cityDict[tok]
	return ok && ce.Country == "in" && !regionNotCity[tok]
}

// normalizeIndianToken rewrites one comma-token into the form the generic path resolves:
// an export to its city, a city's revenue division to the city, an office line to the
// city it opens with. A token that is none of these is returned unchanged.
func normalizeIndianToken(tok string) string {
	for _, p := range indiaWidePhrases {
		if tok == p {
			return "india"
		}
	}
	if city, ok := indianCityInExport(tok); ok {
		if city == "" {
			return "india"
		}
		return city
	}
	tok = stripIndianAreaSuffix(tok)
	if _, ok := cityDict[tok]; ok {
		return tok
	}
	if _, ok := nameToCountry[tok]; ok {
		return tok
	}
	if c := leadingIndianCity(tok); c != "" {
		return c
	}
	return tok
}

// cutIndiaWidePhrases removes the indiaWidePhrases from a lowercased line before its work
// mode is read, so their "anywhere" does not read as remote.
func cutIndiaWidePhrases(lower string) string {
	for _, p := range indiaWidePhrases {
		lower = strings.ReplaceAll(lower, p, " ")
	}
	return lower
}

// pinCityAliases applies cityAliasPins over the loaded city table.
func pinCityAliases(dict map[string]cityEntry, renamedFrom map[[2]string]string) (map[string]cityEntry, map[[2]string]string) {
	for alias, e := range cityAliasPins {
		dict[alias] = e
	}
	return dict, renamedFrom
}

// usNames are the comma-tokens that name the United States outright.
var usNames = map[string]bool{
	"us": true, "usa": true, "u.s.": true, "u.s.a.": true, "united states": true,
	"united states of america": true,
}

// lineNamesUS reports whether a separator-normalized line names the United States in a
// token of its own. A colliding code beside it ("Warsaw, IN, US") is then the state: a
// line that says United States does not also mean India.
func lineNamesUS(s string) bool {
	for _, tok := range strings.Split(s, ",") {
		if usNames[strings.TrimSpace(tok)] {
			return true
		}
	}
	return false
}
