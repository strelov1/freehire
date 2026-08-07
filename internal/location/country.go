package location

import "strings"

// alpha3ToAlpha2 maps the ISO 3166-1 alpha-3 code of every country this package already
// places on the region map (countryToRegion) to its alpha-2 equivalent, so a source that
// states its country as an alpha-3 code (Ashby) normalizes the same as one using alpha-2
// (Lever, Workday). It deliberately covers only the curated 133-country set rather than
// the full ISO-3166 list: a code outside it "resolves to nothing" everywhere else in this
// package, and NormalizeCountry keeps that same contract.
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

// NormalizeCountry maps an ATS-supplied ISO 3166-1 country code — alpha-2 or alpha-3, in
// any case — to freehire's canonical lowercase alpha-2 code. It returns "" for a code
// outside the curated country set Parse itself resolves (countryToRegion): the same
// never-guess contract, so a country this package cannot place on the region map is left
// for the location/description dictionaries to fill instead of being recorded ungrouped.
func NormalizeCountry(code string) string {
	c := strings.ToLower(strings.TrimSpace(code))
	if _, ok := countryToRegion[c]; ok {
		return c
	}
	if a2, ok := alpha3ToAlpha2[c]; ok {
		return a2
	}
	return ""
}
