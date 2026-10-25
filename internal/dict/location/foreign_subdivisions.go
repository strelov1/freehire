package location

// foreignSubdivisionNames resolves the first-level subdivision NAMES of countries
// other than the US and Canada (whose codes and names live in
// subdivisionToCountry) to their country. Merged into nameToCountry at init, so
// every path that reads a place name — the comma tokens, the dash segments, the
// city-of-country check — sees them without a second lookup.
//
// Found by cmd/report-geo-gaps (2026-10-21): a location line naming only the
// region ("Свердловская область", "Belfast, Northern Ireland", "Canberra region,
// Australian Capital Territory") placed the posting nowhere, while the model named
// the country for 100% of them.
//
// Names only, never codes — a code would collide with subdivisionToCountry (see
// AGENTS.md). Deliberately absent, because the name has a second reading this
// catalogue meets: "Punjab" (India and Pakistan), "Victoria" (the Australian state,
// Victoria BC, Victoria TX), "Berkshire", "Essex", "Kent", "Surrey" and
// "Middlesex" (each also a US or Canadian county or city), "ACT" (an English
// word). The contested territories — Crimea, Sevastopol, the Donetsk, Luhansk,
// Zaporizhzhia and Kherson regions — are left out on purpose: assigning them a
// country is not a dictionary decision.
var foreignSubdivisionNames = map[string]string{
	// Russia — republics, in the official form and the short one a location line
	// splits into ("Республика Татарстан (Татарстан)", "Чувашская Республика - Чувашия").
	"республика адыгея": "ru", "адыгея": "ru",
	"республика алтай":        "ru",
	"республика башкортостан": "ru", "башкортостан": "ru", "башкирия": "ru",
	"республика бурятия": "ru", "бурятия": "ru",
	"республика дагестан": "ru", "дагестан": "ru",
	"республика ингушетия": "ru", "ингушетия": "ru",
	"кабардино-балкарская республика": "ru", "кабардино-балкария": "ru",
	"республика калмыкия": "ru", "калмыкия": "ru",
	"карачаево-черкесская республика": "ru", "карачаево-черкесия": "ru",
	"республика карелия": "ru", "карелия": "ru",
	"республика коми":     "ru",
	"республика марий эл": "ru", "марий эл": "ru",
	"республика мордовия": "ru", "мордовия": "ru",
	"республика саха": "ru", "якутия": "ru",
	"республика северная осетия": "ru", "северная осетия": "ru",
	"республика татарстан": "ru", "татарстан": "ru",
	"республика тыва": "ru", "тыва": "ru",
	"удмуртская республика": "ru", "удмуртия": "ru",
	"республика хакасия": "ru", "хакасия": "ru",
	"чеченская республика": "ru", "чечня": "ru",
	"чувашская республика": "ru", "чувашия": "ru",
	// Russia — krais.
	"алтайский край": "ru", "забайкальский край": "ru", "камчатский край": "ru",
	"краснодарский край": "ru", "красноярский край": "ru", "пермский край": "ru",
	"приморский край": "ru", "ставропольский край": "ru", "хабаровский край": "ru",
	// Russia — oblasts.
	"амурская область": "ru", "архангельская область": "ru", "астраханская область": "ru",
	"белгородская область": "ru", "брянская область": "ru", "владимирская область": "ru",
	"волгоградская область": "ru", "вологодская область": "ru", "воронежская область": "ru",
	"ивановская область": "ru", "иркутская область": "ru", "калининградская область": "ru",
	"калужская область": "ru", "кемеровская область": "ru", "кузбасс": "ru",
	"кировская область": "ru", "костромская область": "ru", "курганская область": "ru",
	"курская область": "ru", "ленинградская область": "ru", "липецкая область": "ru",
	"магаданская область": "ru", "московская область": "ru", "мурманская область": "ru",
	"нижегородская область": "ru", "новгородская область": "ru", "новосибирская область": "ru",
	"омская область": "ru", "оренбургская область": "ru", "орловская область": "ru",
	"пензенская область": "ru", "псковская область": "ru", "ростовская область": "ru",
	"рязанская область": "ru", "самарская область": "ru", "саратовская область": "ru",
	"сахалинская область": "ru", "свердловская область": "ru", "смоленская область": "ru",
	"тамбовская область": "ru", "тверская область": "ru", "томская область": "ru",
	"тульская область": "ru", "тюменская область": "ru", "ульяновская область": "ru",
	"челябинская область": "ru", "ярославская область": "ru",
	// Russia — autonomous okrugs and the autonomous oblast.
	"еврейская автономная область": "ru", "ненецкий автономный округ": "ru",
	"ханты-мансийский автономный округ": "ru", "югра": "ru",
	"чукотский автономный округ": "ru", "ямало-ненецкий автономный округ": "ru",
	// Russia — the Latin spellings international boards use.
	"moscow oblast": "ru", "moscow region": "ru", "leningrad oblast": "ru",
	"sverdlovsk oblast": "ru", "tatarstan": "ru", "bashkortostan": "ru",

	// United Kingdom — the constituent countries beside England (already a country
	// name), and the counties and areas whose name has no US or Canadian twin.
	"scotland": "gb", "wales": "gb", "northern ireland": "gb",
	"central london": "gb", "greater london": "gb", "greater manchester": "gb",
	"west midlands": "gb", "merseyside": "gb", "tyne and wear": "gb",
	"yorkshire": "gb", "west yorkshire": "gb", "south yorkshire": "gb", "north yorkshire": "gb",
	// England's statistical regions (ITL 1), which UK boards name verbatim — Civil Service
	// Jobs writes "Durham, North East England, DH1 3HU". "West Midlands" is above.
	"north east england": "gb", "north west england": "gb", "yorkshire and the humber": "gb",
	"east midlands": "gb", "east of england": "gb", "south east england": "gb", "south west england": "gb",
	"cambridgeshire": "gb", "hertfordshire": "gb", "oxfordshire": "gb",
	"buckinghamshire": "gb", "warwickshire": "gb", "lancashire": "gb",

	// Australia — states and territories (Victoria withheld, see above).
	"new south wales": "au", "nsw": "au", "queensland": "au", "qld": "au",
	"western australia": "au", "south australia": "au", "tasmania": "au",
	"northern territory": "au", "australian capital territory": "au",

	// Germany — Länder, in German and in English. The city-states (Berlin,
	// Hamburg, Bremen) are city names already.
	"bayern": "de", "bavaria": "de",
	"baden-württemberg": "de", "baden-wurttemberg": "de", "baden-wuerttemberg": "de",
	"nordrhein-westfalen": "de", "north rhine-westphalia": "de",
	"hessen": "de", "hesse": "de",
	"niedersachsen": "de", "lower saxony": "de",
	"sachsen": "de", "saxony": "de",
	"sachsen-anhalt": "de", "saxony-anhalt": "de",
	"rheinland-pfalz": "de", "rhineland-palatinate": "de",
	"schleswig-holstein": "de", "thüringen": "de", "thuringia": "de",
	"mecklenburg-vorpommern": "de", "saarland": "de", "brandenburg": "de",

	// Austria — Länder, in German and in English. Wien is a city name already, and
	// "Salzburg" (Land and city) resolves through the city table.
	"burgenland": "at", "kärnten": "at", "carinthia": "at",
	"niederösterreich": "at", "lower austria": "at",
	"oberösterreich": "at", "upper austria": "at",
	"steiermark": "at", "styria": "at", "tirol": "at", "tyrol": "at", "vorarlberg": "at",

	// Switzerland — cantons, in German, French and Italian where a location line uses
	// them. Withheld for a second reading: "Freiburg" (Freiburg im Breisgau), "Neuenburg"
	// (Neuenburg am Rhein), "Jura" (the French département and the mountains), "Uri" (a
	// word in this catalogue's own vocabulary), and three the German crawl showed as
	// German places: "Berne" (a town in Lower Saxony and a Hamburg district), "Zug" (in
	// Baden-Württemberg) and "Tessin" (in Mecklenburg-Vorpommern). The canton names that
	// are also their capital's ("Luzern", "Schaffhausen") are the city in either reading.
	"aargau": "ch", "appenzell": "ch", "appenzell ausserrhoden": "ch",
	"appenzell innerrhoden": "ch", "basel-landschaft": "ch", "basel-land": "ch",
	"baselland": "ch", "basel-stadt": "ch", "graubünden": "ch", "grisons": "ch",
	"glarus": "ch", "nidwalden": "ch", "obwalden": "ch", "schwyz": "ch",
	"solothurn": "ch", "thurgau": "ch", "ticino": "ch",
	"wallis": "ch", "valais": "ch", "waadt": "ch", "vaud": "ch",
	"luzern": "ch", "lucerne": "ch", "schaffhausen": "ch", "fribourg": "ch",
	"neuchâtel": "ch", "genf": "ch", "bern": "ch",

	// India — states (Punjab withheld, see above), and the district form Indian
	// ATSs emit for the Bengaluru metro.
	"karnataka": "in", "maharashtra": "in", "tamil nadu": "in", "telangana": "in",
	"kerala": "in", "gujarat": "in", "rajasthan": "in", "uttar pradesh": "in",
	"west bengal": "in", "haryana": "in", "andhra pradesh": "in", "madhya pradesh": "in",
	"odisha": "in", "bihar": "in", "assam": "in", "jharkhand": "in",
	"chhattisgarh": "in", "uttarakhand": "in", "himachal pradesh": "in", "goa": "in",
	"bangalore urban": "in", "bengaluru urban": "in",
}

// regionNotCity are the subdivision names the city table ALSO lists as an alias of a
// DIFFERENT city: the Land Brandenburg is an alternate name of Brandenburg an der Havel,
// so every "Schönefeld, Brandenburg" posting (hundreds in the German crawl) was named a
// posting in that one city. Such a token states its country and no city. A subdivision
// that shares its name with its own capital ("Bern", "Luzern", "Schaffhausen") is not
// here: the city reading is the same place. TestSubdivisionCityAliasesAreDecided makes
// every new overlap a decision rather than an accident.
var regionNotCity = map[string]bool{
	"brandenburg":         true, // Brandenburg an der Havel
	"goa":                 true, // Mormugao
	"bihar":               true, // Bihār Sharīf
	"воронежская область": true, // Voronezh
}

// init merges the table into nameToCountry. A name already there is a programming
// error — either a duplicate or, worse, a disagreement about the country — so it
// stops the package from loading, which every test in it then reports.
func init() {
	for name, country := range foreignSubdivisionNames {
		if existing, dup := nameToCountry[name]; dup {
			panic("location: foreignSubdivisionNames[" + name + "] is already in nameToCountry as " + existing)
		}
		nameToCountry[name] = country
	}
}
