package location

// The dictionaries below are seeded from the high-frequency location strings
// observed in production ATS data. They are meant to grow by observation — add
// the names/cities that show up unresolved, not a full gazetteer up front.

// regionCountries groups ISO 3166-1 alpha-2 country codes under one canonical
// region code from enrich.RegionValues. Each country maps to exactly one region
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
	},
	"apac": {
		"sg", "jp", "au", "nz", "in", "hk", "tw", "kr", "cn", "my", "th", "ph", "vn", "id",
		"bd", "pk", "lk", "np", "kh", "la", "mm", "mn", "bn", "mo",
	},
	"mena": {
		"ae", "sa", "il", "eg", "tr", "qa",
		"kw", "bh", "om", "jo", "lb", "iq", "ir", "ma", "dz", "tn", "ly", "ye", "ps",
	},
	"africa": {
		"za", "ng", "ke",
		"gh", "et", "tz", "ug", "rw", "sn", "ci", "cm", "ao", "mz", "zm", "zw", "mu",
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

// nameToCountry resolves lowercase country names, common ATS shorthands, and a
// few beacon cities to an ISO 3166-1 alpha-2 code. The region falls out of
// countryToRegion, so shorthands like "uk" yield both the country (gb) and its
// region (uk) without a separate entry.
var nameToCountry = map[string]string{
	"united states": "us", "united states of america": "us",
	"usa": "us", "us": "us", "u.s.": "us", "u.s.a.": "us",
	// Unambiguous US city-only strings the dict was missing (LLM-mined gaps).
	"cupertino": "us", "chandler": "us", "schenectady": "us", "greenville": "us",
	"little rock": "us", "oklahoma city": "us",
	"united kingdom": "gb", "uk": "gb", "u.k.": "gb",
	"england": "gb", "britain": "gb", "great britain": "gb", "london": "gb",
	// Unambiguous UK cities (Birmingham/Cambridge omitted — they collide with US metros).
	"manchester": "gb", "edinburgh": "gb", "glasgow": "gb", "bristol": "gb", "liverpool": "gb", "leeds": "gb",
	"germany": "de", "deutschland": "de", "berlin": "de", "munich": "de", "münchen": "de", "hamburg": "de",
	"france": "fr", "paris": "fr",
	"netherlands": "nl", "the netherlands": "nl", "amsterdam": "nl",
	"spain": "es", "madrid": "es", "barcelona": "es",
	"sweden": "se", "sverige": "se", "stockholm": "se",
	"poland": "pl", "warsaw": "pl", "warszawa": "pl",
	"ireland": "ie", "dublin": "ie",
	"portugal": "pt", "lisbon": "pt",
	"italy": "it", "milan": "it", "rome": "it",
	"belgium": "be", "brussels": "be",
	"denmark": "dk", "danmark": "dk", "copenhagen": "dk",
	"finland": "fi", "suomi": "fi", "helsinki": "fi",
	"austria": "at", "vienna": "at",
	"switzerland": "ch", "zurich": "ch",
	"norway": "no", "norge": "no", "ukraine": "ua",
	"canada": "ca", "toronto": "ca", "vancouver": "ca", "montreal": "ca", "montréal": "ca",
	"singapore": "sg",
	"australia": "au", "sydney": "au", "melbourne": "au", "brisbane": "au", "perth": "au", "adelaide": "au",
	"new zealand": "nz", "auckland": "nz", "wellington": "nz",
	"japan": "jp", "tokyo": "jp",
	"india": "in", "pune": "in", "bangalore": "in", "bengaluru": "in", "mumbai": "in", "hyderabad": "in",
	"argentina": "ar", "brazil": "br", "mexico": "mx",
	"israel": "il", "tel aviv": "il",
	"united arab emirates": "ae", "dubai": "ae",
	"south africa": "za",
	// RU / CIS / Central Asia. "georgia" is deliberately absent — it collides with
	// the US state; the country resolves via its capital "tbilisi" only.
	"russia": "ru", "moscow": "ru", "saint petersburg": "ru", "st petersburg": "ru",
	"kyiv": "ua", "kiev": "ua",
	"uzbekistan": "uz", "tashkent": "uz", "toshkent": "uz", "samarkand": "uz",
	"kazakhstan": "kz", "almaty": "kz", "astana": "kz", "nur-sultan": "kz",
	"kyrgyzstan": "kg", "bishkek": "kg",
	"tajikistan": "tj", "dushanbe": "tj",
	"turkmenistan": "tm", "ashgabat": "tm",
	"belarus": "by", "minsk": "by",
	"moldova": "md", "chisinau": "md",
	"armenia": "am", "yerevan": "am",
	"azerbaijan": "az", "baku": "az",
	"tbilisi": "ge",

	// Cyrillic, for the RU-segment ATS sources (sber, mts, alfabank, tbank, vk,
	// huntflow, …) whose location fields are in Russian. Seeded from the
	// high-frequency unresolved strings observed in production; grow by
	// observation. "россия"/"рф" are the country catch-all (the comma tokenizer
	// resolves "Самара, Россия" via the country token even when the city is
	// unknown). The "г "/"город " city-marker prefix is stripped before lookup
	// (stripCityPrefix), so only the bare city name is keyed.
	"россия": "ru", "рф": "ru",
	"москва": "ru", "санкт-петербург": "ru", "спб": "ru", "питер": "ru",
	"екатеринбург": "ru", "новосибирск": "ru", "нижний новгород": "ru",
	"казань": "ru", "самара": "ru", "краснодар": "ru", "ростов-на-дону": "ru",
	"воронеж": "ru", "уфа": "ru", "пермь": "ru", "челябинск": "ru",
	"волгоград": "ru", "красноярск": "ru", "омск": "ru", "тюмень": "ru",
	"саратов": "ru", "тольятти": "ru", "ижевск": "ru", "ульяновск": "ru",
	"барнаул": "ru", "владивосток": "ru", "хабаровск": "ru", "иркутск": "ru",
	"ярославль": "ru", "томск": "ru", "оренбург": "ru", "кемерово": "ru",
	"рязань": "ru", "набережные челны": "ru", "пенза": "ru", "липецк": "ru",
	"тула": "ru", "киров": "ru", "чебоксары": "ru", "калининград": "ru",
	"ставрополь": "ru", "сочи": "ru", "иваново": "ru", "брянск": "ru",
	"белгород": "ru", "сургут": "ru", "владимир": "ru", "архангельск": "ru",
	"калуга": "ru", "смоленск": "ru", "волжский": "ru", "курск": "ru",
	"орёл": "ru", "череповец": "ru", "вологда": "ru", "магнитогорск": "ru",
	"тамбов": "ru", "мурманск": "ru", "тверь": "ru", "новокузнецк": "ru",
	"астрахань": "ru", "великий новгород": "ru", "псков": "ru", "чита": "ru",
	"улан-удэ": "ru", "якутск": "ru", "норильск": "ru", "новороссийск": "ru",
	"таганрог": "ru", "сарапул": "ru", "майкоп": "ru", "подольск": "ru",
	"химки": "ru", "мытищи": "ru", "балашиха": "ru", "курган": "ru",
	"саранск": "ru", "йошкар-ола": "ru", "благовещенск": "ru", "кисловодск": "ru",
	"петропавловск-камчатский": "ru", "комсомольск-на-амуре": "ru",
	"новый уренгой": "ru",

	// CIS / Central Asia / Ukraine in Cyrillic, mirroring their Latin entries.
	"минск": "by", "беларусь": "by",
	"ташкент": "uz", "узбекистан": "uz",
	"алматы": "kz", "астана": "kz", "казахстан": "kz",
	"ереван": "am", "баку": "az", "бишкек": "kg",
	"киев": "ua", "київ": "ua",

	// --- Country names: English + native + ES/PT/DE, seeded from the unresolved
	// production strings. (Names already keyed above are not repeated.)
	"china": "cn", "greece": "gr", "brasil": "br", "philippines": "ph",
	"colombia": "co", "cyprus": "cy", "taiwan": "tw", "malaysia": "my",
	"romania": "ro", "hungary": "hu", "bulgaria": "bg", "thailand": "th",
	"indonesia": "id", "vietnam": "vn", "south korea": "kr", "korea": "kr",
	"turkey": "tr", "türkiye": "tr", "egypt": "eg", "saudi arabia": "sa",
	"lebanon": "lb", "hong kong": "hk", "qatar": "qa", "kuwait": "kw",
	"bahrain": "bh", "oman": "om", "jordan": "jo", "iraq": "iq", "iran": "ir",
	"morocco": "ma", "algeria": "dz", "tunisia": "tn", "pakistan": "pk",
	"bangladesh": "bd", "sri lanka": "lk", "nepal": "np", "peru": "pe",
	"chile": "cl", "uruguay": "uy", "ecuador": "ec", "bolivia": "bo",
	"paraguay": "py", "venezuela": "ve", "costa rica": "cr", "panama": "pa",
	"guatemala": "gt", "dominican republic": "do", "puerto rico": "pr",
	"nigeria": "ng", "kenya": "ke", "ghana": "gh", "ethiopia": "et",
	"czech republic": "cz", "czechia": "cz", "serbia": "rs", "croatia": "hr",
	"slovenia": "si", "slovakia": "sk", "lithuania": "lt", "latvia": "lv",
	"estonia": "ee", "luxembourg": "lu", "iceland": "is", "malta": "mt",
	"monaco": "mc", "north macedonia": "mk", "bosnia and herzegovina": "ba",
	"albania": "al", "montenegro": "me", "kosovo": "xk", "mauritius": "mu",
	// Spanish.
	"españa": "es", "alemania": "de", "méxico": "mx", "méjico": "mx",
	"grecia": "gr", "francia": "fr", "italia": "it", "países bajos": "nl",
	"reino unido": "gb", "estados unidos": "us", "suiza": "ch", "suecia": "se",
	"polonia": "pl", "rumanía": "ro", "hungría": "hu", "turquía": "tr",
	"japón": "jp", "filipinas": "ph", "malasia": "my", "tailandia": "th",
	"corea del sur": "kr", "emiratos árabes unidos": "ae", "arabia saudita": "sa",
	"egipto": "eg", "perú": "pe",
	// Portuguese.
	"espanha": "es", "alemanha": "de", "frança": "fr", "grécia": "gr",
	"países baixos": "nl", "suíça": "ch", "polónia": "pl", "roménia": "ro",
	"hungria": "hu", "turquia": "tr", "japão": "jp", "coreia do sul": "kr",
	"emirados árabes unidos": "ae", "arábia saudita": "sa", "egito": "eg",
	// German.
	"frankreich": "fr", "spanien": "es", "italien": "it", "niederlande": "nl",
	"vereinigtes königreich": "gb", "vereinigte staaten": "us", "schweiz": "ch",
	"schweden": "se", "polen": "pl", "rumänien": "ro", "ungarn": "hu",
	"türkei": "tr", "griechenland": "gr", "philippinen": "ph", "indonesien": "id",
	"südkorea": "kr", "ägypten": "eg", "österreich": "at", "belgien": "be",
	"dänemark": "dk", "finnland": "fi", "irland": "ie", "norwegen": "no",
	"tschechien": "cz", "bulgarien": "bg", "saudi-arabien": "sa",
	"vereinigte arabische emirate": "ae", "bundesweit": "de",

	// --- Beacon cities (high-frequency, unambiguous), region falls out of the code.
	// North America (US).
	"san francisco": "us", "san francisco bay area": "us", "south san francisco": "us",
	"new york city": "us", "nyc": "us", "brooklyn": "us", "manhattan": "us",
	"boston": "us", "chicago": "us", "los angeles": "us", "san jose": "us",
	"austin": "us", "charlotte": "us", "atlanta": "us", "seattle": "us",
	"houston": "us", "palo alto": "us", "denver": "us", "dallas": "us",
	"san antonio": "us", "san diego": "us", "menlo park": "us", "san mateo": "us",
	"indianapolis": "us", "miami": "us", "philadelphia": "us", "phoenix": "us",
	"raleigh": "us", "durham": "us", "detroit": "us", "minneapolis": "us",
	"nashville": "us", "pittsburgh": "us", "salt lake city": "us", "baltimore": "us",
	"sacramento": "us", "irvine": "us", "santa clara": "us", "sunnyvale": "us",
	"mountain view": "us", "redmond": "us", "bellevue": "us", "washington dc": "us",
	"washington, d.c.": "us", "kansas city": "us", "st. louis": "us", "cincinnati": "us",
	// Canada.
	"ottawa": "ca", "calgary": "ca", "edmonton": "ca", "mississauga": "ca",
	// LATAM.
	"são paulo": "br", "sao paulo": "br", "rio de janeiro": "br", "belo horizonte": "br",
	"brasília": "br", "brasilia": "br", "curitiba": "br", "porto alegre": "br",
	"campinas": "br", "florianópolis": "br",
	"mexico city": "mx", "méxico city": "mx", "ciudad de méxico": "mx",
	"guadalajara": "mx", "monterrey": "mx",
	"buenos aires": "ar", "bogotá": "co", "bogota": "co", "medellín": "co",
	"medellin": "co", "lima": "pe", "santiago": "cl", "montevideo": "uy",
	// APAC — China.
	"shanghai": "cn", "beijing": "cn", "shenzhen": "cn", "guangzhou": "cn",
	"suzhou": "cn", "wuxi": "cn", "hangzhou": "cn", "chengdu": "cn",
	"nanjing": "cn", "tianjin": "cn",
	"taipei": "tw", "taichung": "tw", "hsinchu": "tw", "kaohsiung": "tw",
	"seoul": "kr", "pangyo": "kr", "busan": "kr",
	"osaka": "jp", "kyoto": "jp", "yokohama": "jp", "nagoya": "jp", "fukuoka": "jp",
	"bangkok": "th", "kuala lumpur": "my", "jakarta": "id",
	"manila": "ph", "makati": "ph", "taguig": "ph", "cebu": "ph",
	"chennai": "in", "noida": "in", "gurugram": "in", "gurgaon": "in",
	"new delhi": "in", "delhi": "in", "kolkata": "in", "ahmedabad": "in",
	"kochi": "in", "coimbatore": "in", "jaipur": "in", "indore": "in",
	"chandigarh": "in", "thiruvananthapuram": "in",
	"ho chi minh city": "vn", "hanoi": "vn",
	"karachi": "pk", "lahore": "pk", "islamabad": "pk", "dhaka": "bd", "colombo": "lk",
	// Europe.
	"athens": "gr", "thessaloniki": "gr",
	"lisboa": "pt", "porto": "pt",
	"toulouse": "fr", "lyon": "fr", "nantes": "fr", "bordeaux": "fr", "nice": "fr",
	"lille": "fr", "villeurbanne": "fr", "courbevoie": "fr", "levallois-perret": "fr",
	"strasbourg": "fr", "marseille": "fr", "montpellier": "fr", "rennes": "fr", "grenoble": "fr",
	"prague": "cz", "sofia": "bg", "budapest": "hu",
	"bucharest": "ro", "bucurești": "ro", "timișoara": "ro", "timisoara": "ro",
	"cluj-napoca": "ro", "cluj": "ro",
	"kraków": "pl", "krakow": "pl", "wrocław": "pl", "wroclaw": "pl",
	"gdańsk": "pl", "gdansk": "pl", "poznań": "pl", "poznan": "pl",
	"łódź": "pl", "lodz": "pl", "katowice": "pl", "gliwice": "pl",
	"frankfurt": "de", "cologne": "de", "köln": "de", "stuttgart": "de",
	"düsseldorf": "de", "dusseldorf": "de", "leipzig": "de", "dresden": "de",
	"geneva": "ch", "genève": "ch", "geneve": "ch", "basel": "ch", "lausanne": "ch",
	"turin": "it", "naples": "it", "bologna": "it", "florence": "it",
	"valencia": "es", "sevilla": "es", "seville": "es", "málaga": "es",
	"malaga": "es", "bilbao": "es", "zaragoza": "es",
	"antwerp": "be", "ghent": "be",
	"rotterdam": "nl", "the hague": "nl", "den haag": "nl", "eindhoven": "nl", "utrecht": "nl",
	"oslo": "no", "bergen": "no", "gothenburg": "se", "göteborg": "se",
	"malmö": "se", "malmo": "se", "aarhus": "dk", "tampere": "fi", "espoo": "fi",
	"tallinn": "ee", "riga": "lv", "vilnius": "lt", "ljubljana": "si",
	"zagreb": "hr", "belgrade": "rs", "beograd": "rs", "bratislava": "sk",
	"nicosia": "cy", "valletta": "mt", "reykjavik": "is", "reykjavík": "is",
	// MENA.
	"riyadh": "sa", "jeddah": "sa", "dammam": "sa", "beirut": "lb",
	"cairo": "eg", "istanbul": "tr", "ankara": "tr", "izmir": "tr",
	"doha": "qa", "abu dhabi": "ae", "sharjah": "ae", "kuwait city": "kw",
	"manama": "bh", "muscat": "om", "amman": "jo",
	"casablanca": "ma", "rabat": "ma", "tunis": "tn", "algiers": "dz",
	// Africa.
	"cape town": "za", "johannesburg": "za", "durban": "za", "pretoria": "za",
	"nairobi": "ke", "lagos": "ng", "abuja": "ng", "accra": "gh", "addis ababa": "et",
}

// subdivisionToCountry resolves a US state or Canadian province token — a postal
// abbreviation ("tx", "on") or full name ("texas", "ontario") — to its ISO 3166-1
// alpha-2 country code, for the "City, ST ZIP" (US) and "City, Province" (Canada)
// formats that dominate North American ATS data. The region falls out of
// countryToRegion (both us and ca resolve to north_america).
//
// Two-letter codes that collide with a country ISO code whose city the parser
// already keys are deliberately omitted (the country wins, so "Berlin, DE" /
// "Bangalore, IN" / "Amsterdam, NL" stay Germany / India / Netherlands); those
// subdivisions resolve via their full name instead. "ca" is the exception: it
// stays California because "City, CA" is the single most common US location form —
// the rare "Toronto, CA" mislabel is accepted. The country Georgia is unaffected:
// the state carries the code "ga", and the name "georgia" is intentionally absent
// here too (the country resolves via "tbilisi").
var subdivisionToCountry = map[string]string{
	// US states (postal codes). de/in/id omitted — they collide with Germany /
	// India / Indonesia; see delaware/indiana/idaho below.
	"al": "us", "ak": "us", "az": "us", "ar": "us", "ca": "us", "co": "us",
	"ct": "us", "fl": "us", "ga": "us", "hi": "us", "ia": "us", "il": "us",
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

// nameToRegion resolves macro-region names (and explicit open-anywhere markers)
// directly to a region code, for tokens that name an area rather than a country.
var nameToRegion = map[string]string{
	"europe": "eu", "eu": "eu", "europa": "eu",
	"apac": "apac", "asia": "apac", "asia pacific": "apac", "asia-pacific": "apac",
	"north america": "north_america",
	"latam":         "latam", "latin america": "latam", "south america": "latam",
	"mena": "mena", "middle east": "mena",
	"africa": "africa",
	"cis":    "cis", "central asia": "cis",
	// Open-anywhere markers, multilingual. A bare "remote" stays geography-less
	// (work mode only); these are explicit "the whole world" phrasings.
	"anywhere": "global", "worldwide": "global", "global": "global",
	"international": "global", "international remote": "global", "globally": "global",
	"remote anywhere": "global", "world wide": "global", "world-wide": "global",
	"everywhere": "global", "fully distributed": "global",
	"по всему миру": "global", "весь мир": "global",
	"en todo el mundo": "global", "todo el mundo": "global",
	"em todo o mundo": "global", "weltweit": "global",
}
