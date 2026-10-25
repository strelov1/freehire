package location

import "strings"

// dutchProvinceCodes are the abbreviations Dutch writes after a town to tell it from a
// namesake in another province ("Hengelo OV", "Rijswijk ZH", "Haren GN", "Elst GLD").
// eures writes them on every such posting. The value says whether the code is Dutch
// alone: "nh", "nb", "ut" and "fl" are also New Hampshire, New Brunswick, Utah and
// Florida, and "fr", "gr", "li", "lb" are ISO country codes, so a shared code needs the
// town before it to be one the dictionary already places in the Netherlands.
var dutchProvinceCodes = map[string]bool{
	"ov": true, "zh": true, "gld": true, "gn": true,
	"nh": false, "nb": false, "ut": false, "fl": false, "dr": false, "fr": false,
	"zl": false, "lb": false, "li": false, "gr": false,
}

// stripDutchProvince cuts a Dutch province code off the end of a multi-word token
// ("hengelo ov" -> "hengelo", "huis ter heide ut" -> "huis ter heide"), the way
// stripAustralianState cuts a state code.
func stripDutchProvince(tok string) (string, bool) {
	fields := strings.Fields(tok)
	if len(fields) < 2 {
		return tok, false
	}
	only, ok := dutchProvinceCodes[fields[len(fields)-1]]
	if !ok {
		return tok, false
	}
	place := strings.Join(fields[:len(fields)-1], " ")
	if !only && !isDutchPlace(place) {
		return tok, false
	}
	return place, true
}

// isDutchPlace reports whether the dictionary places tok in the Netherlands, through the
// city tables or the small-places table.
func isDutchPlace(tok string) bool {
	if isRecognizedCityOf(tok, "nl") {
		return true
	}
	_, ok := smallPlaceCandidates(tok)["nl"]
	return ok
}

// provinceCapitals are the Dutch and Belgian provinces named after their capital. After
// another town they are the province, not a second city: "Amersfoort, Utrecht, Nederland"
// is a posting in Amersfoort, while "Utrecht, Utrecht, Nederland" still names Utrecht
// once, from its first token.
var provinceCapitals = map[string]bool{
	"utrecht": true, "groningen": true, "antwerpen": true, "namen": true, "namur": true,
	"luik": true, "liège": true,
}

// provinceAfterCity reports whether tok is a provinceCapitals name in the province slot of
// a "Town, Province, Country" line: a city came before it, and the line writes it between
// two commas. "Amersfoort or Utrecht, Nederland" names Utrecht as a second worksite — its
// separator before it is a list word, not a comma — and keeps the city.
func provinceAfterCity(tok, lower string, citySet map[string]struct{}) bool {
	return provinceCapitals[tok] && len(citySet) > 0 && strings.Contains(lower, ", "+tok+",")
}

// remoteLabels are the remote label Recruitee writes in the posting's language IN PLACE
// of a location. Matched as the WHOLE line, never as a substring: inside a longer line
// "Homeoffice möglich" offers hybrid work and "kein Homeoffice" refuses it.
var remoteLabels = map[string]bool{
	"werken op afstand": true, "homeoffice": true, "poste à distance": true,
	"trabajo a distancia": true, "praca zdalna": true, "trabalho remoto": true,
	"stillingen kan udføres hjemmefra": true, "attālinātais darbs": true,
}
