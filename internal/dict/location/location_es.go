package location

// spanishProvinceCapitals are the Spanish provinces named after their capital. Spanish
// location lines write "Town, Province, Country" ("Pozuelo de Alarcón, Madrid, España",
// "Oliva, Valencia, España"), so in that slot the name is the province, not a second
// city — the rule provinceAfterCity applies to the Dutch provinceCapitals.
var spanishProvinceCapitals = map[string]bool{
	"a coruña": true, "a coruna": true, "la coruña": true, "la coruna": true, "albacete": true, "alicante": true, "almería": true, "almeria": true,
	"ávila": true, "avila": true, "badajoz": true, "barcelona": true, "burgos": true,
	"cáceres": true, "caceres": true, "cádiz": true, "cadiz": true, "castellón": true,
	"castellon": true, "ciudad real": true, "córdoba": true, "cordoba": true, "cuenca": true,
	"girona": true, "granada": true, "guadalajara": true, "huelva": true, "huesca": true,
	"jaén": true, "jaen": true, "león": true, "leon": true, "lleida": true, "lugo": true,
	"madrid": true, "málaga": true, "malaga": true, "murcia": true, "ourense": true,
	"palencia": true, "pontevedra": true, "salamanca": true, "segovia": true, "sevilla": true,
	"soria": true, "tarragona": true, "teruel": true, "toledo": true, "valencia": true,
	"valladolid": true, "zamora": true, "zaragoza": true, "las palmas": true,
	"santa cruz de tenerife": true,
}

// mexicanStateCapitals are the Mexican states named after their capital. OCC writes
// "Town, State" with the state LAST ("León, Guanajuato", "Celaya, Guanajuato"), so a
// trailing one after a town is the state as well as one between two commas.
var mexicanStateCapitals = map[string]bool{
	"aguascalientes": true, "campeche": true, "chihuahua": true, "colima": true,
	"durango": true, "guanajuato": true, "oaxaca": true, "puebla": true, "querétaro": true,
	"queretaro": true, "san luis potosí": true, "san luis potosi": true, "tlaxcala": true,
	"veracruz": true, "zacatecas": true,
}
