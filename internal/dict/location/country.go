package location

import "strings"

// alpha3ToAlpha2 maps the ISO 3166-1 alpha-3 code of every country on the region map
// (countryToRegion) to its alpha-2 equivalent, so a source stating alpha-3 (Ashby)
// normalizes like one using alpha-2 (Lever, Workday). It covers only that curated set,
// not all of ISO-3166: a code outside it resolves to nothing, as elsewhere here.
var alpha3ToAlpha2 = map[string]string{
	"deu": "de", "fra": "fr", "nld": "nl", "esp": "es", "swe": "se", "pol": "pl",
	"irl": "ie", "prt": "pt", "ita": "it", "bel": "be", "dnk": "dk", "fin": "fi",
	"aut": "at", "cze": "cz", "rou": "ro", "grc": "gr", "hun": "hu", "bgr": "bg",
	"hrv": "hr", "svk": "sk", "svn": "si", "ltu": "lt", "lva": "lv", "est": "ee",
	"lux": "lu", "che": "ch", "nor": "no", "ukr": "ua", "isl": "is",
	"srb": "rs", "bih": "ba", "mkd": "mk", "alb": "al", "mne": "me", "xkx": "xk",
	"cyp": "cy", "mlt": "mt", "lie": "li", "mco": "mc", "and": "ad", "smr": "sm",
	"gbr": "gb",
	"usa": "us", "can": "ca",
	"arg": "ar", "bra": "br", "mex": "mx", "chl": "cl", "col": "co", "per": "pe",
	"ury": "uy", "ecu": "ec", "bol": "bo", "pry": "py", "ven": "ve", "cri": "cr",
	"pan": "pa", "gtm": "gt", "dom": "do", "hnd": "hn", "slv": "sv", "nic": "ni",
	"pri": "pr",
	"sgp": "sg", "jpn": "jp", "aus": "au", "nzl": "nz", "ind": "in", "hkg": "hk",
	"twn": "tw", "kor": "kr", "chn": "cn", "mys": "my", "tha": "th", "phl": "ph",
	"vnm": "vn", "idn": "id", "bgd": "bd", "pak": "pk", "lka": "lk", "npl": "np",
	"khm": "kh", "lao": "la", "mmr": "mm", "mng": "mn", "brn": "bn", "mac": "mo",
	"are": "ae", "sau": "sa", "isr": "il", "egy": "eg", "tur": "tr", "qat": "qa",
	"kwt": "kw", "bhr": "bh", "omn": "om", "jor": "jo", "lbn": "lb", "irq": "iq",
	"irn": "ir", "mar": "ma", "dza": "dz", "tun": "tn", "lby": "ly", "yem": "ye",
	"pse": "ps",
	"zaf": "za", "nga": "ng", "ken": "ke", "gha": "gh", "eth": "et", "tza": "tz",
	"uga": "ug", "rwa": "rw", "sen": "sn", "civ": "ci", "cmr": "cm", "ago": "ao",
	"moz": "mz", "zmb": "zm", "zwe": "zw", "mus": "mu",
	"rus": "ru", "blr": "by", "mda": "md", "arm": "am", "aze": "az", "geo": "ge",
	"uzb": "uz", "kaz": "kz", "kgz": "kg", "tjk": "tj", "tkm": "tm",
}

// NormalizeCountry maps an ATS-supplied country value to freehire's canonical lowercase
// alpha-2 code. The field's format varies by ATS and even by employer: alpha-2 (Lever,
// Workday), alpha-3 ("USA", Ashby) or a plain name ("United States", Ashby). The name
// case reuses nameToCountry, the lookup Parse uses for a location string's country
// token. It returns "" for a value outside the curated set (countryToRegion), leaving
// that country for the location/description dictionaries to fill.
func NormalizeCountry(code string) string {
	c := strings.ToLower(strings.TrimSpace(code))
	if _, ok := countryToRegion[c]; ok {
		return c
	}
	if a2, ok := alpha3ToAlpha2[c]; ok {
		return a2
	}
	if a2, ok := nameToCountry[c]; ok {
		return a2
	}
	return ""
}
