package location

import (
	"bufio"
	"strings"
	"sync"
)

// brazilianStateCodes are the two-letter codes of Brazil's 26 states and federal district,
// which every Brazilian ATS writes after the town: "Curitiba, PR", "Belo Horizonte/MG",
// "São Paulo - SP". Most of them spell something else too — PR is Puerto Rico, SC South
// Carolina, BA Bosnia, ES Spain, PE Peru and Prince Edward Island, AM Armenia, MT Malta and
// Montana, RS Serbia, MA Morocco and Massachusetts, MS Mississippi, MG Madagascar, PA
// Panama and Pennsylvania, AL Albania and Alabama, RO Romania, SE Sweden, TO Tonga, DF
// Mexico's federal district — and the parser read them that way: measured 2026-10-27, 14%
// of 30,677 open Brazilian postings carried a second, wrong country (4,235 rows), and the
// Fortaleza, Natal and Santo André postings none at all.
//
// So a code is read as the Brazilian state when the line places it in Brazil: it names
// Brazil ("Curitiba, PR, Brasil"), or the town before the code has a Brazilian reading
// (brazilianStateAccepted). A line that also names the United States keeps the US state.
var brazilianStateCodes = map[string]bool{
	"ac": true, "al": true, "ap": true, "am": true, "ba": true, "ce": true, "df": true,
	"es": true, "go": true, "ma": true, "mt": true, "ms": true, "mg": true, "pa": true,
	"pb": true, "pr": true, "pe": true, "pi": true, "rj": true, "rn": true, "rs": true,
	"ro": true, "rr": true, "sc": true, "sp": true, "se": true, "to": true,
}

// brazilNames are the tokens that name Brazil in a location line. "br" counts only as the
// line's LAST token: "Patna, BR, in" writes Bihar.
var brazilNames = map[string]bool{"brasil": true, "brazil": true, "bra": true}

// lineNamesBrazil reports whether a separator-normalized location line names Brazil.
func lineNamesBrazil(s string) bool {
	toks := strings.Split(s, ",")
	last := ""
	for _, tok := range toks {
		tok = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(tok), parenMark))
		if brazilNames[tok] {
			return true
		}
		if tok != "" {
			last = tok
		}
	}
	return last == "br"
}

// brazilianStateAccepted reports whether a Brazilian state code after prevTok is the state:
// the line names Brazil, or prevTok has a Brazilian reading. A town the city table gives
// to another country first still counts when GeoNames also has it in Brazil ("Colombo,
// PR" is the Paraná town, not Sri Lanka's capital).
//
// A token that names the country the code spells is that country's ISO code, not a state:
// "Juan Díaz, Panamá, PA" is in Panama, though Goiás has a town called Panamá.
func brazilianStateAccepted(code, prevTok string, namesBrazil bool) bool {
	if prevTok == "" || nameToCountry[prevTok] == code || regionNotCity[prevTok] {
		return false
	}
	return namesBrazil || isBrazilianPlace(prevTok)
}

// isBrazilianPlace reports whether any of the dictionary's tables has tok in Brazil.
func isBrazilianPlace(tok string) bool {
	if nameToCountry[tok] == "br" {
		return true
	}
	if ce, ok := cityDict[tok]; ok && ce.Country == "br" {
		return true
	}
	if _, ok := brazilianCityAliases()[tok]; ok {
		return true
	}
	_, ok := smallPlaceCandidates(tok)["br"]
	return ok
}

// brazilianCityAliases maps the aliases of every Brazilian row of the city table to that
// row's name, whichever country's namesake the alias resolves to first ("concordia" is
// Concórdia, Santa Catarina, though the city table reads it as a district of El Paso).
// The most populous Brazilian place keeps a shared alias. Loaded on first use.
var brazilianCityAliases = sync.OnceValue(func() map[string]string {
	out := map[string]string{}
	sc := bufio.NewScanner(strings.NewReader(citiesTSV))
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for sc.Scan() {
		parts := strings.SplitN(sc.Text(), "\t", 3)
		if len(parts) < 3 || parts[1] != "br" {
			continue
		}
		for _, alias := range strings.Split(parts[2], "|") {
			if alias = strings.TrimSuffix(alias, contestMark); out[alias] == "" {
				out[alias] = parts[0]
			}
		}
	}
	return out
})

// stripBrazilianState cuts a Brazilian state code off the end of a multi-word token whose
// town is Brazilian ("campinas sp" -> "campinas"), the way stripDutchProvince cuts a Dutch
// province.
func stripBrazilianState(tok string) (string, bool) {
	fields := strings.Fields(tok)
	if len(fields) < 2 || !brazilianStateCodes[fields[len(fields)-1]] {
		return tok, false
	}
	place := strings.Join(fields[:len(fields)-1], " ")
	if !brazilianStateAccepted(fields[len(fields)-1], place, false) {
		return tok, false
	}
	return place, true
}

// indianStateBetween reports whether tok is an Indian state code between an Indian city and
// the country: "Patna, BR, in" (Bihar), "Chennai, TN, IN" (Tamil Nadu). The code is then
// the state and states India, never Brazil or Tunisia.
func indianStateBetween(tok, prevTok string, namesIndia bool) bool {
	return namesIndia && indianStateCodes[tok] && isIndianCity(prevTok)
}

// lineNamesIndia reports whether a separator-normalized location line ends in India.
func lineNamesIndia(s string) bool {
	toks := strings.Split(s, ",")
	for i := len(toks) - 1; i >= 0; i-- {
		if tok := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(toks[i]), parenMark)); tok != "" {
			return tok == "in" || tok == "ind" || tok == "india"
		}
	}
	return false
}

// portugueseDistrictCapitals are the Portuguese districts named after their capital. After
// a town and between two commas they are the district, not a second city: "Matosinhos,
// Porto, PT" is a posting in Matosinhos (see provinceAfterCity).
var portugueseDistrictCapitals = map[string]bool{
	"lisboa": true, "lisbon": true, "porto": true, "braga": true, "aveiro": true,
	"coimbra": true, "leiria": true, "faro": true, "setúbal": true, "setubal": true,
	"viseu": true, "santarém": true, "santarem": true, "évora": true, "evora": true,
	"beja": true, "bragança": true, "braganca": true, "guarda": true, "portalegre": true,
	"castelo branco": true, "vila real": true, "viana do castelo": true,
}

// withdrawCountry removes a country a token stated, with its region unless another stated
// country still holds that region.
func withdrawCountry(c string, countrySet, regionSet map[string]struct{}) {
	delete(countrySet, c)
	r, ok := countryToRegion[c]
	if !ok {
		return
	}
	for other := range countrySet {
		if countryToRegion[other] == r {
			return
		}
	}
	delete(regionSet, r)
}

// sameName reports whether a city's display name spells tok, accents aside: "Concórdia"
// for "concordia", but not "Conceição do Mato Dentro" for "conceição".
func sameName(name, tok string) bool {
	return dropAccents(strings.ToLower(name)) == dropAccents(tok)
}
