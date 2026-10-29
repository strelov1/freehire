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
	// Brazil's states, in the long form a location line writes beside or instead of the
	// two-letter code (location_br.go). Absent, because the name has a second reading:
	// "Paraná" (an Argentine city), "Santa Catarina" (a Mexican city), "Amazonas" (a
	// Venezuelan and a Peruvian region), "Acre" (an Israeli city), "Distrito Federal"
	// (Mexico City's), and São Paulo and Rio de Janeiro, which the city table already places.
	"minas gerais": "br", "rio grande do sul": "br", "rio grande do norte": "br",
	"mato grosso": "br", "mato grosso do sul": "br", "espírito santo": "br",
	"espirito santo": "br", "pernambuco": "br", "ceará": "br", "ceara": "br", "bahia": "br",
	"goiás": "br", "goias": "br", "maranhão": "br", "maranhao": "br", "paraíba": "br",
	"paraiba": "br", "piauí": "br", "piaui": "br", "alagoas": "br", "sergipe": "br",
	"tocantins": "br", "rondônia": "br", "rondonia": "br", "roraima": "br", "amapá": "br",
	"amapa": "br", "pará": "br",
	// Portugal's autonomous regions.
	"madeira": "pt", "região autónoma da madeira": "pt", "açores": "pt", "acores": "pt",
	"azores": "pt", "região autónoma dos açores": "pt",
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
	// The Netherlands — the provinces a location line names after its town ("Tilburg,
	// Noord-Brabant, Nederland"), and the Randstad conurbation. Utrecht and Groningen are
	// absent: each shares its name with its own capital, which the city tables already
	// place. Limburg is absent too: it is a Dutch AND a Belgian province (regionNotCity).
	"noord-holland": "nl", "zuid-holland": "nl", "noord-brabant": "nl", "gelderland": "nl",
	"overijssel": "nl", "flevoland": "nl", "drenthe": "nl", "friesland": "nl",
	"fryslân": "nl", "zeeland": "nl", "randstad": "nl",
	// Belgium — the Flemish and Walloon provinces and regions, in Dutch and French.
	// Antwerpen, Namen/Namur and Luik/Liège share their names with their capitals.
	"oost-vlaanderen": "be", "west-vlaanderen": "be", "vlaams-brabant": "be",
	"waals-brabant": "be", "brabant wallon": "be", "flandre-orientale": "be",
	"flandre-occidentale": "be", "vlaanderen": "be", "wallonië": "be", "wallonie": "be",
	"henegouwen": "be", "hainaut": "be",
	// Spain — the autonomous communities in Spanish, Catalan, Basque and English, the
	// Basque provinces, and the islands a location line names in place of a town ("Madrid,
	// Comunidad de Madrid", "Palma, Illes Balears", "Tenerife"). The provinces named after
	// their capital are the city in either reading (spanishProvinceCapitals). La Rioja is
	// absent: it is also an Argentine province and city.
	"comunidad de madrid": "es", "cataluña": "es", "catalunya": "es", "catalonia": "es",
	"país vasco": "es", "pais vasco": "es", "euskadi": "es", "basque country": "es",
	"comunidad valenciana": "es", "comunitat valenciana": "es",
	"andalucía": "es", "andalucia": "es", "andalusia": "es",
	"aragón": "es", "aragon": "es", "principado de asturias": "es", "asturias": "es",
	"islas baleares": "es", "illes balears": "es", "baleares": "es", "balearic islands": "es",
	"canarias": "es", "islas canarias": "es", "canary islands": "es", "cantabria": "es",
	"castilla y león": "es", "castilla y leon": "es", "castilla-la mancha": "es",
	"castilla la mancha": "es", "extremadura": "es", "galicia": "es",
	"región de murcia": "es", "region de murcia": "es",
	"comunidad foral de navarra": "es", "navarra": "es", "nafarroa": "es",
	"bizkaia": "es", "vizcaya": "es", "gipuzkoa": "es", "guipúzcoa": "es", "guipuzcoa": "es",
	"álava": "es", "alava": "es", "araba": "es",
	"mallorca": "es", "menorca": "es", "tenerife": "es", "gran canaria": "es",
	"lanzarote": "es", "fuerteventura": "es",
	// Mexico — the states OCC writes after the town ("Mérida, Yucatán", "Toluca, Estado
	// de México"). The states named after their capital are the city in either reading
	// (mexicanStateCapitals). Withheld for a US reading the catalogue meets: "Durango"
	// (Colorado), "Hidalgo" (Texas), "Sonora" (California).
	"aguascalientes": "mx", "baja california": "mx", "baja california sur": "mx",
	"campeche": "mx", "chiapas": "mx", "chihuahua": "mx", "coahuila": "mx", "coahuila de zaragoza": "mx",
	"colima": "mx", "estado de méxico": "mx", "estado de mexico": "mx", "edomex": "mx",
	"guanajuato": "mx", "guerrero": "mx", "jalisco": "mx", "michoacán": "mx",
	"michoacan": "mx", "michoacán de ocampo": "mx", "morelos": "mx", "nayarit": "mx",
	"nuevo león": "mx", "nuevo leon": "mx", "oaxaca": "mx", "puebla": "mx",
	"querétaro": "mx", "queretaro": "mx", "quintana roo": "mx", "san luis potosí": "mx",
	"san luis potosi": "mx", "sinaloa": "mx", "tabasco": "mx", "tamaulipas": "mx",
	"tlaxcala": "mx", "veracruz": "mx", "yucatán": "mx", "yucatan": "mx", "zacatecas": "mx",
	// Colombia — the departments a line names beside or instead of Bogotá's "D.C.".
	// Withheld: "Santander" (the Spanish city), "Bolívar", "Córdoba", "Magdalena", "Cesar".
	"antioquia": "co", "valle del cauca": "co", "cundinamarca": "co", "atlántico": "co",
	"atlantico": "co", "risaralda": "co", "boyacá": "co", "boyaca": "co", "tolima": "co",
	"huila": "co", "quindío": "co", "quindio": "co", "norte de santander": "co",
	// Argentina — the capital city under its own names (cityOverrides gives it the city),
	// and the provinces whose name has no second reading this catalogue meets. Withheld:
	// "Misiones" (a Paraguayan department), "Río Negro" (a Uruguayan one), "Santa Cruz" and
	// "Tierra del Fuego" (Bolivia, Chile), "La Rioja" and "Córdoba" (Spain), and "Formosa",
	// "Corrientes" and "Entre Ríos" (towns elsewhere: Entre Rios is in Paraná and Bahia).
	// The conurbation (GBA, AMBA) is a metro region, read in location_ar.go.
	"caba": "ar", "c.a.b.a.": "ar", "c.a.b.a": "ar", "capital federal": "ar",
	"ciudad autónoma de buenos aires": "ar", "ciudad autonoma de buenos aires": "ar",
	"ciudad de buenos aires": "ar", "buenos aires city": "ar",
	"provincia de buenos aires": "ar", "buenos aires province": "ar",
	"pcia. de buenos aires": "ar", "chaco": "ar", "chubut": "ar", "jujuy": "ar",
	"catamarca": "ar", "santiago del estero": "ar",
	"santa fe province": "ar", "provincia de santa fe": "ar", "mendoza province": "ar",
	"provincia de mendoza": "ar", "santiago del estero province": "ar",
	"entre ríos province": "ar", "entre rios province": "ar", "chaco province": "ar",
	"chubut province": "ar", "neuquén province": "ar", "neuquen province": "ar",
	"provincia de neuquén": "ar", "salta province": "ar", "provincia de salta": "ar",
	"tucumán province": "ar", "tucuman province": "ar", "provincia de tucumán": "ar",
	"jujuy province": "ar", "misiones province": "ar", "corrientes province": "ar",
	"san juan province": "ar", "san luis province": "ar", "río negro province": "ar",
	"rio negro province": "ar", "la pampa province": "ar", "catamarca province": "ar",
	"formosa province": "ar", "santa cruz province": "ar", "tierra del fuego province": "ar",
	// Poland — the sixteen voivodeships, in Polish (adjective and "województwo" forms) and in
	// English, as a location line names them beside or instead of a town ("Duchnice,
	// Mazowieckie, Poland", "Gołków, Masovian Voivodeship, pl"). Absent: "Śląsk" and
	// "Silesia" (a metro, see polishMetroRegions, and Czech Silesia), "Pomerania" (German
	// Vorpommern), and the bare city names Lublin, Łódź and Opole, which the city table places.
	"mazowieckie": "pl", "małopolskie": "pl", "malopolskie": "pl", "małopolska": "pl",
	"malopolska": "pl", "dolnośląskie": "pl", "dolnoslaskie": "pl", "wielkopolskie": "pl",
	"wielkopolska": "pl", "pomorskie": "pl", "zachodniopomorskie": "pl", "śląskie": "pl",
	"slaskie": "pl", "łódzkie": "pl", "lodzkie": "pl", "lubelskie": "pl", "podkarpackie": "pl",
	"świętokrzyskie": "pl", "swietokrzyskie": "pl", "warmińsko-mazurskie": "pl",
	"warminsko-mazurskie": "pl", "kujawsko-pomorskie": "pl", "opolskie": "pl",
	"lubuskie": "pl", "podlaskie": "pl",
	"województwo mazowieckie": "pl", "województwo małopolskie": "pl",
	"województwo dolnośląskie": "pl", "województwo wielkopolskie": "pl",
	"województwo pomorskie": "pl", "województwo zachodniopomorskie": "pl",
	"województwo śląskie": "pl", "województwo łódzkie": "pl", "województwo lubelskie": "pl",
	"województwo podkarpackie": "pl", "województwo świętokrzyskie": "pl",
	"województwo warmińsko-mazurskie": "pl", "województwo kujawsko-pomorskie": "pl",
	"województwo opolskie": "pl", "województwo lubuskie": "pl", "województwo podlaskie": "pl",
	"masovian voivodeship": "pl", "lesser poland voivodeship": "pl", "lesser poland": "pl",
	"lower silesian voivodeship": "pl", "lower silesia": "pl", "greater poland voivodeship": "pl",
	"greater poland": "pl", "pomeranian voivodeship": "pl", "west pomeranian voivodeship": "pl",
	"silesian voivodeship": "pl", "łódź voivodeship": "pl", "lodz voivodeship": "pl",
	"lublin voivodeship": "pl", "subcarpathian voivodeship": "pl",
	"podkarpackie voivodeship": "pl", "świętokrzyskie voivodeship": "pl",
	"holy cross voivodeship": "pl", "warmian-masurian voivodeship": "pl",
	"kuyavian-pomeranian voivodeship": "pl", "opole voivodeship": "pl",
	"lubusz voivodeship": "pl", "podlaskie voivodeship": "pl",
	// France — the regions (current and pre-2016) and the departments a location line
	// names beside or instead of a town ("Limoux, Occitanie", "Haute-Savoie, Auvergne-Rhône-Alpes,
	// France", "PAYS DE LA LOIRE, France"). Withheld: Paris (the city), Vienne (Austria's capital in
	// French, and a town in Isère), Jura (a Swiss canton too), Manche (the Channel), Nord (a
	// compass word), Ardennes and Meuse (Belgian too). The overseas departments and collectivities keep their own ISO codes; New
	// Caledonia and Mayotte are absent, their codes being North Carolina's and Yukon's.
	"ain": "fr", "aisne": "fr", "allier": "fr", "alpes de haute provence": "fr",
	"alpes maritimes": "fr", "alpes-de-haute-provence": "fr", "alpes-maritimes": "fr",
	"alsace": "fr", "aquitaine": "fr", "ardeche": "fr", "ardèche": "fr",
	"ariege": "fr", "ariège": "fr", "aube": "fr", "aude": "fr", "auvergne": "fr",
	"auvergne rhone alpes": "fr", "auvergne rhône alpes": "fr", "auvergne-rhone-alpes": "fr",
	"auvergne-rhône-alpes": "fr", "aveyron": "fr", "bas rhin": "fr", "bas-rhin": "fr",
	"basse normandie": "fr", "basse-normandie": "fr", "bouches du rhone": "fr",
	"bouches du rhône": "fr", "bouches-du-rhone": "fr", "bouches-du-rhône": "fr",
	"bourgogne": "fr", "bourgogne franche comte": "fr", "bourgogne franche comté": "fr",
	"bourgogne-franche-comte": "fr", "bourgogne-franche-comté": "fr", "bretagne": "fr",
	"calvados": "fr", "cantal": "fr", "centre val de loire": "fr", "centre-val de loire": "fr",
	"centre-val-de-loire": "fr", "champagne ardenne": "fr", "champagne-ardenne": "fr",
	"charente": "fr", "charente maritime": "fr", "charente-maritime": "fr", "cher": "fr",
	"correze": "fr", "corrèze": "fr", "corse": "fr", "corse du sud": "fr", "corse-du-sud": "fr",
	"cote d'or": "fr", "cote-d'or": "fr", "cote-d’or": "fr", "cotes d'armor": "fr",
	"cotes-d'armor": "fr", "cotes-d’armor": "fr", "creuse": "fr", "côte d'or": "fr",
	"côte-d'or": "fr", "côte-d’or": "fr", "côtes d'armor": "fr", "côtes-d'armor": "fr",
	"côtes-d’armor": "fr", "deux sevres": "fr", "deux sèvres": "fr", "deux-sevres": "fr",
	"deux-sèvres": "fr", "dordogne": "fr", "doubs": "fr", "drome": "fr", "drôme": "fr",
	"essonne": "fr", "eure": "fr", "eure et loir": "fr", "eure-et-loir": "fr", "finistere": "fr",
	"finistère": "fr", "franche comte": "fr", "franche comté": "fr", "franche-comte": "fr",
	"franche-comté": "fr", "gard": "fr", "gers": "fr", "gironde": "fr", "grand est": "fr",
	"grand-est": "fr", "haut rhin": "fr", "haut-rhin": "fr", "haute corse": "fr",
	"haute garonne": "fr", "haute loire": "fr", "haute marne": "fr", "haute normandie": "fr",
	"haute saone": "fr", "haute savoie": "fr", "haute saône": "fr", "haute vienne": "fr",
	"haute-corse": "fr", "haute-garonne": "fr", "haute-loire": "fr", "haute-marne": "fr",
	"haute-normandie": "fr", "haute-saone": "fr", "haute-savoie": "fr", "haute-saône": "fr",
	"haute-vienne": "fr", "hautes alpes": "fr", "hautes pyrenees": "fr", "hautes pyrénées": "fr",
	"hautes-alpes": "fr", "hautes-pyrenees": "fr", "hautes-pyrénées": "fr",
	"hauts de france": "fr", "hauts de seine": "fr", "hauts-de-france": "fr",
	"hauts-de-seine": "fr", "herault": "fr", "hérault": "fr", "ile de france": "fr",
	"ile-de-france": "fr", "ille et vilaine": "fr", "ille-et-vilaine": "fr", "indre": "fr",
	"indre et loire": "fr", "indre-et-loire": "fr", "isere": "fr", "isère": "fr", "landes": "fr",
	"languedoc roussillon": "fr", "languedoc-roussillon": "fr", "limousin": "fr",
	"loir et cher": "fr", "loir-et-cher": "fr", "loire": "fr", "loire atlantique": "fr",
	"loire-atlantique": "fr", "loiret": "fr", "lot": "fr", "lot et garonne": "fr",
	"lot-et-garonne": "fr", "lozere": "fr", "lozère": "fr", "maine et loire": "fr",
	"maine-et-loire": "fr", "marne": "fr", "mayenne": "fr", "meurthe et moselle": "fr",
	"meurthe-et-moselle": "fr", "midi pyrenees": "fr", "midi pyrénées": "fr",
	"midi-pyrenees": "fr", "midi-pyrénées": "fr", "morbihan": "fr", "moselle": "fr",
	"nievre": "fr", "nièvre": "fr", "nord pas de calais": "fr", "nord-pas-de-calais": "fr",
	"normandie": "fr", "nouvelle aquitaine": "fr", "nouvelle-aquitaine": "fr", "occitanie": "fr",
	"oise": "fr", "orne": "fr", "paca": "fr", "pas de calais": "fr", "pas-de-calais": "fr",
	"pays de la loire": "fr", "pays-de-la-loire": "fr", "picardie": "fr",
	"poitou charentes": "fr", "poitou-charentes": "fr", "provence alpes cote d'azur": "fr",
	"provence alpes côte d'azur": "fr", "provence-alpes-cote d'azur": "fr",
	"provence-alpes-cote d’azur": "fr", "provence-alpes-cote-d'azur": "fr",
	"provence-alpes-cote-d’azur": "fr", "provence-alpes-côte d'azur": "fr",
	"provence-alpes-côte d’azur": "fr", "provence-alpes-côte-d'azur": "fr",
	"provence-alpes-côte-d’azur": "fr", "puy de dome": "fr", "puy de dôme": "fr",
	"puy-de-dome": "fr", "puy-de-dôme": "fr", "pyrenees atlantiques": "fr",
	"pyrenees orientales": "fr", "pyrenees-atlantiques": "fr", "pyrenees-orientales": "fr",
	"pyrénées atlantiques": "fr", "pyrénées orientales": "fr", "pyrénées-atlantiques": "fr",
	"pyrénées-orientales": "fr", "region sud": "fr", "rhone": "fr", "rhone alpes": "fr",
	"rhone-alpes": "fr", "rhône": "fr", "rhône alpes": "fr", "rhône-alpes": "fr",
	"région sud": "fr", "saone et loire": "fr", "saone-et-loire": "fr", "sarthe": "fr",
	"savoie": "fr", "saône et loire": "fr", "saône-et-loire": "fr", "seine et marne": "fr",
	"seine maritime": "fr", "seine saint denis": "fr", "seine-et-marne": "fr",
	"seine-maritime": "fr", "seine-saint-denis": "fr", "somme": "fr", "tarn": "fr",
	"tarn et garonne": "fr", "tarn-et-garonne": "fr", "territoire de belfort": "fr",
	"val d'oise": "fr", "val de marne": "fr", "val-d'oise": "fr", "val-de-marne": "fr",
	"val-d’oise": "fr", "var": "fr", "vaucluse": "fr", "vendee": "fr", "vendée": "fr",
	"vosges": "fr", "yonne": "fr", "yvelines": "fr", "île de france": "fr",
	"île-de-france": "fr", "martinique": "mq", "guadeloupe": "gp", "guyane": "gf",
	"guyane française": "gf", "guyane francaise": "gf", "la réunion": "re", "la reunion": "re",
	"réunion": "re", "île de la réunion": "re", "ile de la reunion": "re", "polynésie française": "pf", "polynesie francaise": "pf",

	// Chile.
	"región metropolitana": "cl", "region metropolitana": "cl",
	"región metropolitana de santiago": "cl",
}

// regionNotCity are the subdivision names the city table ALSO lists as an alias of a
// DIFFERENT city: the Land Brandenburg is an alternate name of Brandenburg an der Havel,
// so every "Schönefeld, Brandenburg" posting (hundreds in the German crawl) was named a
// posting in that one city. Such a token states its country and no city. A subdivision
// that shares its name with its own capital ("Bern", "Luzern", "Schaffhausen") is not
// here: the city reading is the same place. TestSubdivisionCityAliasesAreDecided makes
// every new overlap a decision rather than an accident.
var regionNotCity = map[string]bool{
	// French departments the city table reads as a town: Mayenne is also a town that is
	// not the department's prefecture, "Cher" is an alias of Chieri, "Lot" of Lota.
	"mayenne": true, "cher": true, "lot": true,
	"brandenburg":         true, // Brandenburg an der Havel
	"goa":                 true, // Mormugao
	"bihar":               true, // Bihār Sharīf
	"воронежская область": true, // Voronezh
	// A Dutch and a Belgian province, and Limburg an der Lahn in Hesse: it states no
	// city, and — being two countries' province — no country either.
	"limburg": true,
	// The Spanish islands and communities whose name GeoNames also gives a town: Tenerife
	// is an alias of Santa Cruz de Tenerife, Andalucía and Asturias are Colombian and
	// Philippine towns.
	"tenerife": true, "gran canaria": true, "mallorca": true, "andalucía": true,
	"andalucia": true, "asturias": true, "galicia": true, "navarra": true,
	// Mexican states the city table reads as a different town ("Jalisco" is Xalisco,
	// "Chiapas" Chiapa de Corzo, "Morelos" Ecatepec de Morelos).
	"jalisco": true, "chiapas": true, "morelos": true, "nuevo león": true, "nuevo leon": true,
	"michoacán": true, "michoacan": true, "coahuila": true, "guerrero": true, "nayarit": true,
	"quintana roo": true, "sinaloa": true, "tabasco": true, "tamaulipas": true,
	"yucatán": true, "yucatan": true, "baja california": true, "baja california sur": true,
	"estado de méxico": true, "estado de mexico": true, "edomex": true,
	// Colombian departments, Bogotá's "D.C." (an alias of Washington) and Buenos Aires'
	// "CABA" (an alias of Chamba).
	"antioquia": true, "valle del cauca": true, "cundinamarca": true, "atlántico": true,
	"atlantico": true, "risaralda": true, "boyacá": true, "boyaca": true, "tolima": true,
	"huila": true, "quindío": true, "quindio": true, "norte de santander": true,
	"d.c.": true,
	// Entre Ríos, which is not a country name here (see above), is GeoNames' alias of
	// Ribeirão Preto, so it names no city either.
	"entre rios": true, "entre ríos": true,
	// The Brazilian states and Portuguese islands the city table reads as a town ("Ceará"
	// is an alias of Fortaleza, "Madeira" of Tabira, "Rio Grande do Sul" a prefix of Rio
	// Grande).
	"minas gerais": true, "rio grande do sul": true, "rio grande do norte": true,
	"mato grosso": true, "mato grosso do sul": true, "espírito santo": true,
	"espirito santo": true, "pernambuco": true, "ceará": true, "ceara": true, "bahia": true,
	"goiás": true, "goias": true, "maranhão": true, "maranhao": true, "paraíba": true,
	"paraiba": true, "piauí": true, "piaui": true, "alagoas": true, "sergipe": true,
	"tocantins": true, "rondônia": true, "rondonia": true, "roraima": true, "amapá": true,
	"amapa": true, "pará": true, "madeira": true, "açores": true, "acores": true, "azores": true,
	// Country names GeoNames also gives the capital: "Monterrey, Mexico" named Mexico City
	// as a second city, "David, Panama" Panama City.
	"mexico": true, "méxico": true, "méjico": true, "panama": true, "panamá": true,
	"guatemala": true, "paraguay": true, "puerto rico": true, "nicaragua": true,
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
