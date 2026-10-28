package location

import "strings"

// Argentina. Measured 2026-10-27 over ~12k open postings placed in Argentina
// (pandapelatam, whatjobs-ar, manatal, hiringroom, teamtailor, workday, cornerstone,
// workable, smartrecruiters, oracle, torre, epam), a third of the lines named no city, and
// most of those were Buenos Aires written as something the city table does not know:
//
//   - the autonomous city's own names — "CABA", "C.A.B.A.", "Capital Federal",
//     "Ciudad de Buenos Aires", "Buenos Aires City" — which cityOverrides now gives the
//     city;
//   - one of its 48 barrios ("Palermo", "Recoleta", "San Nicolás", "Núñez"), which
//     pandapelatam writes ALONE, with the country only in its structured data;
//   - a town of Greater Buenos Aires ("Vicente López", "Munro", "Gral. Pacheco"), most of
//     them below GeoNames' population floor or under a namesake abroad ("Martínez" was
//     the Californian one, "Florida" the US state, "Ciudad Evita" a Santa Fe town).
//
// A barrio is the city of Buenos Aires; a town of the conurbation is itself, in the
// capital's metro. Both are read only where the line places them in Buenos Aires (see
// tokenLocation.inBuenosAires): "Palermo, Italia" is Sicily's, "San Cristóbal, Santa Fe"
// a department of that province, and "Pilar, Córdoba" Córdoba's Pilar.

// buenosAiresMetro is the metro of the city and its conurbation, as gen-metros labels it.
const buenosAiresMetro = "Buenos Aires (AR)"

// buenosAiresBarrios are the city's barrios, accent-folded, plus the names a line uses for
// its centre ("Microcentro", "Barrio Norte", "Once"). Each names the city of Buenos Aires.
var buenosAiresBarrios = map[string]bool{
	"agronomia": true, "almagro": true, "balvanera": true, "barracas": true, "belgrano": true,
	"boedo": true, "caballito": true, "chacarita": true, "coghlan": true, "colegiales": true,
	"constitucion": true, "flores": true, "floresta": true, "la boca": true, "la paternal": true,
	"liniers": true, "mataderos": true, "monte castro": true, "monserrat": true,
	"montserrat": true, "nueva pompeya": true, "pompeya": true, "nunez": true, "palermo": true,
	"parque avellaneda": true, "parque chacabuco": true, "parque chas": true,
	"parque patricios": true, "puerto madero": true, "recoleta": true, "retiro": true,
	"saavedra": true, "san cristobal": true, "san nicolas": true, "san telmo": true,
	"velez sarsfield": true, "versalles": true, "villa crespo": true, "villa del parque": true,
	"villa devoto": true, "villa general mitre": true, "villa lugano": true, "lugano": true,
	"villa luro": true, "villa ortuzar": true, "villa pueyrredon": true, "villa real": true,
	"villa riachuelo": true, "villa santa rita": true, "villa soldati": true,
	"villa urquiza": true, "microcentro": true, "barrio norte": true, "once": true,
}

// buenosAiresTowns are the towns and partidos of Greater Buenos Aires the city and metro
// tables miss or misread, accent-folded, with the name the cities facet shows. "Victoria",
// "Libertad" and "Fátima" are absent: each is a common word or a better-known place.
var buenosAiresTowns = map[string]string{
	"vicente lopez": "Vicente López", "olivos": "Olivos", "florida": "Florida",
	"munro": "Munro", "villa martelli": "Villa Martelli", "carapachay": "Carapachay",
	"villa adelina": "Villa Adelina", "boulogne": "Boulogne", "martinez": "Martínez",
	"acassuso": "Acassuso", "beccar": "Béccar", "san isidro": "San Isidro",
	"san fernando": "San Fernando", "tigre": "Tigre", "general pacheco": "General Pacheco",
	"el talar": "El Talar", "benavidez": "Benavídez", "nordelta": "Nordelta",
	"garin": "Garín", "escobar": "Escobar", "ingeniero maschwitz": "Ingeniero Maschwitz",
	"pilar": "Pilar", "del viso": "Del Viso", "san miguel": "San Miguel",
	"bella vista": "Bella Vista", "ituzaingo": "Ituzaingó", "ramos mejia": "Ramos Mejía",
	"san justo": "San Justo", "la tablada": "La Tablada",
	"lomas del mirador": "Lomas del Mirador", "gonzalez catan": "González Catán",
	"ciudadela": "Ciudadela", "caseros": "Caseros", "villa ballester": "Villa Ballester",
	"san andres": "San Andrés", "villa lynch": "Villa Lynch",
	"general san martin": "General San Martín", "san martin": "General San Martín",
	"loma hermosa": "Loma Hermosa", "jose leon suarez": "José León Suárez",
	"avellaneda": "Avellaneda", "dock sud": "Dock Sud", "wilde": "Wilde",
	"temperley": "Temperley", "longchamps": "Longchamps", "monte grande": "Monte Grande",
	"canning": "Canning", "bernal": "Bernal", "bernal oeste": "Bernal Oeste",
	"florencio varela": "Florencio Varela", "paso del rey": "Paso del Rey",
	"general rodriguez": "General Rodríguez", "grand bourg": "Grand Bourg",
	"tortuguitas": "Tortuguitas", "malvinas argentinas": "Malvinas Argentinas",
	"villa bosch": "Villa Bosch", "martin coronado": "Martín Coronado",
	"aldo bonzi": "Aldo Bonzi", "tapiales": "Tapiales", "lavallol": "Llavallol",
	"llavallol": "Llavallol", "turdera": "Turdera", "ezpeleta": "Ezpeleta",
	"la matanza": "La Matanza", "tres de febrero": "Tres de Febrero",
	"almirante brown": "Almirante Brown", "esteban echeverria": "Esteban Echeverría",
	"villa tesei": "Villa Tesei", "william c. morris": "William C. Morris",
	"ingeniero budge": "Ingeniero Budge", "gerli": "Gerli", "pineiro": "Piñeiro",
	"villa dominico": "Villa Domínico", "quilmes oeste": "Quilmes Oeste",
	"rafael calzada": "Rafael Calzada", "claypole": "Claypole", "glew": "Glew",
	"ministro rivadavia": "Ministro Rivadavia", "luis guillon": "Luis Guillón",
	"el jaguel": "El Jagüel", "ciudad evita": "Ciudad Evita",
	"villa luzuriaga": "Villa Luzuriaga", "rafael castillo": "Rafael Castillo",
	"virrey del pino": "Virrey del Pino", "villa celina": "Villa Celina",
	"mariano acosta": "Mariano Acosta", "francisco alvarez": "Francisco Álvarez",
	"parque leloir": "Parque Leloir", "manuel alberti": "Manuel Alberti",
	"villa rosa": "Villa Rosa", "la reja": "La Reja", "rincon de milberg": "Rincón de Milberg",
	"troncos del talar": "Troncos del Talar", "ricardo rojas": "Ricardo Rojas",
	"dique lujan": "Dique Luján", "pablo nogues": "Pablo Nogués",
	"ingeniero pablo nogues": "Pablo Nogués", "adolfo sourdeaux": "Adolfo Sourdeaux",
	"villa raffo": "Villa Raffo", "lomas del palomar": "Lomas del Palomar",
	"ciudad jardin lomas del palomar": "Lomas del Palomar", "el palomar": "El Palomar",
	"saenz pena": "Sáenz Peña", "villa sarmiento": "Villa Sarmiento",
	"san antonio de padua": "San Antonio de Padua", "villa madero": "Villa Madero",
	"los cardales": "Los Cardales", "maquinista savio": "Maquinista Savio",
	"matheu": "Matheu", "quilmes": "Quilmes", "lanus": "Lanús",
	"lomas de zamora": "Lomas de Zamora", "moron": "Morón", "hurlingham": "Hurlingham",
	"merlo": "Merlo", "moreno": "Moreno", "jose c. paz": "José C. Paz",
	"don torcuato": "Don Torcuato", "boulogne sur mer": "Boulogne",
}

// buenosAiresMetroRegions name the conurbation itself: they state the country and the
// capital's metro, and no city — a GBA posting may sit in any of its partidos.
var buenosAiresMetroRegions = map[string]bool{
	"gba": true, "gran buenos aires": true, "amba": true, "conurbano": true,
	"conurbano bonaerense": true, "área metropolitana de buenos aires": true,
	"area metropolitana de buenos aires": true,
}

// buenosAiresContext are the tokens that may sit beside a barrio or a town without
// placing it anywhere else: the country, the city, the province, the conurbation and the
// zones a line divides it into. Accent-folded.
var buenosAiresContext = map[string]bool{
	"argentina": true, "ar": true, "arg": true, "republica argentina": true,
	"buenos aires": true, "bs. as.": true, "bs as": true, "bsas": true,
	"buenos aires province": true, "provincia de buenos aires": true,
	"pcia. de buenos aires": true, "caba": true, "c.a.b.a.": true, "c.a.b.a": true,
	"capital federal": true, "ciudad autonoma de buenos aires": true,
	"ciudad de buenos aires": true, "buenos aires city": true,
	"zona norte": true, "zona sur": true, "zona oeste": true,
}

// argentinaNames are the tokens that place a line in Argentina, accent-folded.
// A bare "AR" counts only inside a line every other token of which is a Buenos Aires place
// (lineInBuenosAires), where it cannot be Arkansas.
var argentinaNames = map[string]bool{
	"argentina": true, "ar": true, "arg": true, "republica argentina": true, "buenos aires": true,
	"caba": true, "c.a.b.a.": true, "c.a.b.a": true, "capital federal": true,
	"ciudad autonoma de buenos aires": true, "ciudad de buenos aires": true,
	"buenos aires city": true, "buenos aires province": true, "provincia de buenos aires": true,
	"gba": true, "gran buenos aires": true, "amba": true,
}

// argentineProvinceCapitals are the Argentine provinces named after their capital. In
// the province slot of a line that names Argentina they are the province: "Rosario, Santa
// Fe, Argentina" is a posting in Rosario, "Pilar, Buenos Aires, Argentina" one in Pilar.
// Córdoba is already in spanishProvinceCapitals.
var argentineProvinceCapitals = map[string]bool{
	"buenos aires": true, "santa fe": true, "mendoza": true, "salta": true, "san juan": true,
	"san luis": true, "neuquén": true, "neuquen": true, "tucumán": true, "tucuman": true,
	"catamarca": true, "corrientes": true, "formosa": true, "la rioja": true, "jujuy": true,
	"santiago del estero": true,
}

// argentineToken folds a token for the Argentine tables: accents dropped, a partido's
// "Partido de" cut, and the abbreviations of "General" and "Ingeniero" spelled out
// ("Gral. Pacheco", "Ing. Maschwitz").
func argentineToken(tok string) string {
	tok = dropAccents(tok)
	tok = strings.TrimPrefix(tok, "partido de ")
	for _, r := range [][2]string{{"gral. ", "general "}, {"gral ", "general "}, {"ing. ", "ingeniero "}} {
		if strings.HasPrefix(tok, r[0]) {
			tok = r[1] + tok[len(r[0]):]
		}
	}
	return tok
}

// buenosAiresPlace is the city a barrio or a town of Greater Buenos Aires names, "" when
// tok is neither.
func buenosAiresPlace(tok string) string {
	t := argentineToken(tok)
	if buenosAiresBarrios[t] {
		return "Buenos Aires"
	}
	return buenosAiresTowns[t]
}

// lineInBuenosAires reports whether every token of the separator-normalized location s is
// a Buenos Aires place, a buenosAiresContext token, or a work-mode word, and whether one
// of them names Argentina. The first is what keeps "Pilar, Córdoba" out of the conurbation.
func lineInBuenosAires(s string) (onlyBuenosAires, namesArgentina bool) {
	onlyBuenosAires = true
	for _, tok := range strings.Split(s, ",") {
		tok = stripWorkmodeWords(strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(tok), parenMark)))
		if tok == "" {
			continue
		}
		t := argentineToken(tok)
		if argentinaNames[t] || buenosAiresMetroRegions[tok] {
			namesArgentina = true
		}
		if !buenosAiresContext[t] && !buenosAiresMetroRegions[tok] && buenosAiresPlace(tok) == "" {
			onlyBuenosAires = false
		}
	}
	return onlyBuenosAires, namesArgentina
}

// hasPlaceIn reports whether tok names a place the dictionary puts in country, through any
// of its tables.
func hasPlaceIn(tok, country string) bool {
	// The Argentine provincial capitals count whoever GeoNames gives the name to first:
	// "San Luis, AR" is the Argentine city, not the Arizona town.
	if isRecognizedCityOf(tok, country) ||
		(country == "ar" && (buenosAiresPlace(tok) != "" || argentineProvinceCapitals[tok])) {
		return true
	}
	_, ok := smallPlaceCandidates(tok)[country]
	return ok
}

// subdivisionBeforeCountry reports whether code, a country code written between the town
// prevTok and the country next, is that country's subdivision: "Zaragoza, AR, España" is
// Aragón, "Herisau, AR, CH" Appenzell Ausserrhoden, "Bergamo, BG, Italia" the province of
// Bergamo. It needs the town to be a place of next's country and not of code's, so
// "Berlin, DE, AT" stays a list of two countries.
func subdivisionBeforeCountry(code, prevTok, next string) bool {
	if len(code) != 2 || prevTok == "" || next == "" {
		return false
	}
	if _, iso := countryToRegion[code]; !iso {
		return false
	}
	country := nameToCountry[next]
	if country == "" {
		if _, iso := countryToRegion[next]; iso && len(next) == 2 {
			country = next
		}
	}
	if country == "" || country == code {
		return false
	}
	return hasPlaceIn(prevTok, country) && !hasPlaceIn(prevTok, code)
}

// nextToken is the comma-token after toks[i], trimmed, "" when there is none.
func nextToken(toks []string, i int) string {
	for _, t := range toks[i+1:] {
		if t = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(t), parenMark)); t != "" {
			return t
		}
	}
	return ""
}

// endsInArgentina reports whether a lowercased line closes on Argentina, the country slot
// of the "Town, Province, Country" shape argentineProvinceCapitals are read in.
func endsInArgentina(lower string) bool {
	lower = strings.TrimSpace(lower)
	for _, c := range []string{"argentina", ", ar", ", arg"} {
		if strings.HasSuffix(lower, c) {
			return true
		}
	}
	return false
}
