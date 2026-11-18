package location

// The dictionaries below are seeded from high-frequency location strings in ATS
// data and grow by observation: add names that show up unresolved, not a full
// gazetteer.

// regionCountries groups ISO 3166-1 alpha-2 country codes under one canonical
// region code from vocab.RegionValues. Each country maps to exactly one region
// (the coarse facet a user filters on); countryToRegion is the inverted lookup.
// "eu" is used in the broad geographic sense of Europe (not only EU members).
var regionCountries = map[string][]string{
	"eu": {
		"de", "fr", "nl", "es", "se", "pl", "ie", "pt", "it", "be", "dk",
		"fi", "at", "cz", "ro", "gr", "hu", "bg", "hr", "sk", "si", "lt",
		"lv", "ee", "lu", "ch", "no", "ua", "is",
		// Rest of geographic Europe (the Balkans, micro-states, Cyprus/Malta).
		"rs", "ba", "mk", "al", "me", "xk", "cy", "mt", "li", "mc", "ad", "sm",
	},
	"uk":            {"gb"},
	"north_america": {"us", "ca"},
	"latam": {
		"ar", "br", "mx", "cl", "co", "pe", "uy",
		"ec", "bo", "py", "ve", "cr", "pa", "gt", "do", "hn", "sv", "ni", "pr",
		// France's Caribbean and South American departments.
		"mq", "gp", "gf",
	},
	"apac": {
		"sg", "jp", "au", "nz", "in", "hk", "tw", "kr", "cn", "my", "th", "ph", "vn", "id",
		"bd", "pk", "lk", "np", "kh", "la", "mm", "mn", "bn", "mo",
		// French Polynesia. New Caledonia ("nc") and Mayotte ("yt") are absent: their codes
		// are North Carolina's and Yukon's, and a code with a region becomes a country a
		// bare "NC" would resolve to.
		"pf",
	},
	"mena": {
		"ae", "sa", "il", "eg", "tr", "qa",
		"kw", "bh", "om", "jo", "lb", "iq", "ir", "ma", "dz", "tn", "ly", "ye", "ps",
	},
	"africa": {
		"za", "ng", "ke",
		"gh", "et", "tz", "ug", "rw", "sn", "ci", "cm", "ao", "mz", "zm", "zw", "mu",
		// Réunion.
		"re",
	},
	// CIS — the whole post-Soviet space (the RU-segment geography of the Telegram
	// sources): Russia, Belarus, Moldova, the Caucasus, and the five Central Asian
	// republics. Russia is not its own region; ua stays eu.
	"cis": {"ru", "by", "md", "am", "az", "ge", "uz", "kz", "kg", "tj", "tm"},
}

// countryToRegion is the inverted regionCountries: ISO code -> region code.
var countryToRegion = invertRegionCountries()

func invertRegionCountries() map[string]string {
	out := make(map[string]string)
	for region, codes := range regionCountries {
		for _, code := range codes {
			out[code] = region
		}
	}
	return out
}

// subdivisionToCountry resolves a US state or Canadian province token — a postal
// abbreviation ("tx", "on") or full name ("texas", "ontario") — to its ISO 3166-1
// alpha-2 country code, for the "City, ST ZIP" (US) and "City, Province" (Canada)
// formats that dominate North American ATS data. The region falls out of
// countryToRegion (both us and ca resolve to north_america).
//
// Two-letter codes that collide with a country ISO code are listed anyway:
// computeCollidingSubdivisions picks them up into collidingSubdivisions, and
// resolveSubdivision's subdivisionAccepted gate lets the US/CA reading win only when
// the neighbouring city token names a recognized US/CA place ("Carmel, IN" stays
// Indiana, "Bangalore, IN" stays India). "nl" is the one deliberate omission:
// Newfoundland resolves via its full name so "Amsterdam, NL" stays the Netherlands.
// "ca" stays California unconditionally because "City, CA" is the most common US
// form; the rare "Toronto, CA" mislabel is accepted. The name "georgia" is absent
// (the country resolves via "tbilisi"; the state carries "ga").
var subdivisionToCountry = map[string]string{
	// US states (postal codes). de/in/id collide with Germany / India /
	// Indonesia; collidingSubdivisions gates them behind subdivisionAccepted.
	"al": "us", "ak": "us", "az": "us", "ar": "us", "ca": "us", "co": "us",
	"ct": "us", "de": "us", "fl": "us", "ga": "us", "hi": "us", "ia": "us",
	"id": "us", "il": "us", "in": "us",
	"ks": "us", "ky": "us", "la": "us", "ma": "us", "md": "us", "me": "us",
	"mi": "us", "mn": "us", "mo": "us", "ms": "us", "mt": "us", "nc": "us",
	"nd": "us", "ne": "us", "nh": "us", "nj": "us", "nm": "us", "nv": "us",
	"ny": "us", "oh": "us", "ok": "us", "or": "us", "pa": "us", "ri": "us",
	"sc": "us", "sd": "us", "tn": "us", "tx": "us", "ut": "us", "va": "us",
	"vt": "us", "wa": "us", "wi": "us", "wv": "us", "wy": "us", "dc": "us",
	// US states (full names). "georgia" omitted (collides with the country).
	"alabama": "us", "alaska": "us", "arizona": "us", "arkansas": "us",
	"california": "us", "colorado": "us", "connecticut": "us", "delaware": "us",
	"florida": "us", "hawaii": "us", "idaho": "us", "illinois": "us",
	"indiana": "us", "iowa": "us", "kansas": "us", "kentucky": "us",
	"louisiana": "us", "maine": "us", "maryland": "us", "massachusetts": "us",
	"michigan": "us", "minnesota": "us", "mississippi": "us", "missouri": "us",
	"montana": "us", "nebraska": "us", "nevada": "us", "new hampshire": "us",
	"new jersey": "us", "new mexico": "us", "new york": "us",
	"north carolina": "us", "north dakota": "us", "ohio": "us", "oklahoma": "us",
	"oregon": "us", "pennsylvania": "us", "rhode island": "us",
	"south carolina": "us", "south dakota": "us", "tennessee": "us",
	"texas": "us", "utah": "us", "vermont": "us", "virginia": "us",
	"washington": "us", "west virginia": "us", "wisconsin": "us",
	"wyoming": "us", "district of columbia": "us",
	// Canadian provinces (postal codes). "nl" omitted — it collides with the
	// Netherlands; Newfoundland resolves via its full name below.
	"on": "ca", "bc": "ca", "qc": "ca", "ab": "ca", "mb": "ca", "sk": "ca",
	"ns": "ca", "nb": "ca", "pe": "ca", "nt": "ca", "yt": "ca", "nu": "ca",
	// Canadian provinces (full names).
	"ontario": "ca", "quebec": "ca", "british columbia": "ca", "alberta": "ca",
	"manitoba": "ca", "saskatchewan": "ca", "nova scotia": "ca",
	"new brunswick": "ca", "newfoundland and labrador": "ca",
	"prince edward island": "ca", "northwest territories": "ca",
	"yukon": "ca", "nunavut": "ca",
}

// subdivisionCode is the postal code of each subdivision subdivisionToCountry places,
// read by the metro lookup to tell Roseville, CA from Roseville, MI.
var subdivisionCode = map[string]string{
	"al": "AL", "ak": "AK", "az": "AZ", "ar": "AR", "ca": "CA", "co": "CO", "ct": "CT",
	"de": "DE", "fl": "FL", "ga": "GA", "hi": "HI", "ia": "IA", "id": "ID", "il": "IL",
	"in": "IN", "ks": "KS", "ky": "KY", "la": "LA", "ma": "MA", "md": "MD", "me": "ME",
	"mi": "MI", "mn": "MN", "mo": "MO", "ms": "MS", "mt": "MT", "nc": "NC", "nd": "ND",
	"ne": "NE", "nh": "NH", "nj": "NJ", "nm": "NM", "nv": "NV", "ny": "NY", "oh": "OH",
	"ok": "OK", "or": "OR", "pa": "PA", "ri": "RI", "sc": "SC", "sd": "SD", "tn": "TN",
	"tx": "TX", "ut": "UT", "va": "VA", "vt": "VT", "wa": "WA", "wi": "WI", "wv": "WV",
	"wy": "WY", "dc": "DC", "alabama": "AL", "alaska": "AK", "arizona": "AZ",
	"arkansas": "AR", "california": "CA", "colorado": "CO", "connecticut": "CT",
	"delaware": "DE", "florida": "FL", "hawaii": "HI", "idaho": "ID", "illinois": "IL",
	"indiana": "IN", "iowa": "IA", "kansas": "KS", "kentucky": "KY", "louisiana": "LA",
	"maine": "ME", "maryland": "MD", "massachusetts": "MA", "michigan": "MI",
	"minnesota": "MN", "mississippi": "MS", "missouri": "MO", "montana": "MT",
	"nebraska": "NE", "nevada": "NV", "new hampshire": "NH", "new jersey": "NJ",
	"new mexico": "NM", "new york": "NY", "north carolina": "NC", "north dakota": "ND",
	"ohio": "OH", "oklahoma": "OK", "oregon": "OR", "pennsylvania": "PA",
	"rhode island": "RI", "south carolina": "SC", "south dakota": "SD", "tennessee": "TN",
	"texas": "TX", "utah": "UT", "vermont": "VT", "virginia": "VA", "washington": "WA",
	"west virginia": "WV", "wisconsin": "WI", "wyoming": "WY",
	"district of columbia": "DC", "on": "ON", "bc": "BC", "qc": "QC", "ab": "AB",
	"mb": "MB", "sk": "SK", "ns": "NS", "nb": "NB", "pe": "PE", "nt": "NT", "yt": "YT",
	"nu": "NU", "ontario": "ON", "quebec": "QC", "british columbia": "BC",
	"alberta": "AB", "manitoba": "MB", "saskatchewan": "SK", "nova scotia": "NS",
	"new brunswick": "NB", "newfoundland and labrador": "NL",
	"prince edward island": "PE", "northwest territories": "NT", "yukon": "YT",
	"nunavut": "NU",
}

// collidingSubdivisions is the subset of subdivisionToCountry's two-letter codes that
// also spell a curated country's alpha-2 code ("il" Illinois/Israel, "la"
// Louisiana/Laos, "pa" Pennsylvania/Panama, and about a dozen more).
// resolveSubdivision accepts the US/CA reading only when the neighbouring city token
// names a recognized US/CA place (subdivisionAccepted in location.go); otherwise the
// country code wins. Computed so it cannot drift from the source tables; "ca" is
// excluded (see subdivisionToCountry).
var collidingSubdivisions = computeCollidingSubdivisions()

// stateFirstSubdivisions are the colliding codes whose US-STATE reading dominates the
// catalogue, so the default flips: after a city the dictionary does not place in the
// colliding country, the code is the state. Otherwise every US town the curated maps
// and GeoNames do not call American ("Cambridge" and "Birmingham" are British there,
// "Annapolis Junction" is nowhere) fell through to the country code.
//
// Membership rests on a measurement (cmd/report-geo-gaps over postings stored under
// the country reading, against the countries LLM enrichment recorded): PA (Panama),
// MA, MD, IL, MN, AL, MO, LA, ME read as US almost always. Deliberately absent
// because the COUNTRY wins there: DE, IN, ID, AR, CO, TN, MT. A code joins this set
// on such a measurement, never on intuition.
var stateFirstSubdivisions = map[string]struct{}{
	"pa": {}, "ma": {}, "md": {}, "il": {}, "mn": {}, "al": {}, "mo": {}, "la": {}, "me": {},
}

func computeCollidingSubdivisions() map[string]struct{} {
	out := map[string]struct{}{}
	for code := range subdivisionToCountry {
		if len(code) != 2 || code == "ca" {
			continue
		}
		if _, isCountryCode := countryToRegion[code]; isCountryCode {
			out[code] = struct{}{}
		}
	}
	return out
}
