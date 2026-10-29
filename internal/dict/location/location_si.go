package location

import "strings"

// Slovenian location lines, measured 2026-10-29 over 2,900 open MojeDelo postings (the
// national board, freehire#3861). MojeDelo writes the town alone and the country beside
// it, so ParseIn already named most towns through the small-places table; what read
// nothing was the rest of the Slovenian vocabulary:
//
//   - a statistical or traditional REGION in place of a town ("Osrednjeslovenska",
//     "Savinjska", "Gorenjska regija", "Jugovzhodna Slovenija" — 136 of the lines);
//   - the country written as an area ("Celotna Slovenija", "po celi Sloveniji") and the
//     countries abroad in Slovenian ("Avstrija", "Nemčija");
//   - the area around a town ("Ljubljana in okolica", "Celje z okolico", "okolica
//     Celja", the last with the town in the genitive);
//   - a Ljubljana district after the city ("Ljubljana Črnuče", "Ljubljana Šiška");
//   - municipality seats and villages GeoNames' cities1000 lacks or names longer
//     ("Hoče", "Kozina", "Boštanj", "Brnik", "Lenart" for Lenart v Slovenskih Goricah);
//   - "Ribnica", whose main-table reading is Rîbnița in Moldova.
//
// And a bare town from a source that states no country named nothing at all: the main
// table holds nine Slovenian places, so "Kamnik" or "Krško" placed no posting. The
// Slovenian TOWNS (the settlements with town status) are therefore read like a main-table
// city — named, and stating Slovenia when nothing else in the line states a country —
// except those another country also has a place of: see slovenianTowns.

// slovenianCountryNames are the countries in Slovenian, where the BCS names
// (bcsCountryNames) spell them differently, and Slovenia written as an area. Absent on
// purpose: "Tujina" (abroad) names no country; "ZDA" (the United States) and the plain
// "Svica" (Switzerland) are also villages in Switzerland and Croatia; "Kras", "Primorska"
// and "Štajerska" are below, withheld.
var slovenianCountryNames = map[string]string{
	"celotna slovenija": "si", "cela slovenija": "si", "vsa slovenija": "si",
	"po celi sloveniji": "si", "po vsej sloveniji": "si", "republika slovenija": "si",
	"avstrija": "at", "nemčija": "de", "nemcija": "de",
	"hrvaška": "hr", "hrvaska": "hr", "madžarska": "hu", "madzarska": "hu",
	"švica": "ch", "francija": "fr", "slovaška": "sk", "slovaska": "sk",
	"romunija": "ro", "bolgarija": "bg", "grčija": "gr", "grcija": "gr",
	"bosna in hercegovina": "ba", "črna gora": "me",
	"združeno kraljestvo": "gb", "zdruzeno kraljestvo": "gb",
	"združene države amerike": "us", "zdruzene drzave amerike": "us",
	"avstralija": "au", "turčija": "tr", "turcija": "tr", "ciper": "cy",
	"portugalska": "pt", "kitajska": "cn", "japonska": "jp",
	"egipt": "eg", "savdska arabija": "sa", "združeni arabski emirati": "ae",
	"zdruzeni arabski emirati": "ae",
}

// slovenianRegions are Slovenia's twelve statistical regions (with their former names)
// and the traditional regions a posting is placed in. Each states Slovenia and no city,
// with and without "regija". Withheld: the bare "Koroška" and "Štajerska", which are also
// the Slovenian names of Austrian Carinthia and Styria (where MojeDelo's postings abroad
// go), "Primorska" (the Croatian littoral too) and "Kras" (a place in Croatia, Austria and
// the Czech Republic). "Koroška regija" is the Slovenian statistical region and is read.
var slovenianRegions = []string{
	"pomurska", "podravska", "savinjska", "zasavska", "posavska", "spodnjeposavska",
	"jugovzhodna slovenija", "osrednjeslovenska", "gorenjska", "primorsko-notranjska",
	"notranjsko-kraška", "goriška", "obalno-kraška",
	"osrednja slovenija", "dolenjska", "notranjska", "prekmurje", "pomurje", "posavje",
	"zasavje", "podravje", "bela krajina", "koroška",
}

// slovenianRegionsOnlyWithRegija are slovenianRegions read only with "regija" beside them.
var slovenianRegionsOnlyWithRegija = map[string]bool{"koroška": true}

// slovenianTowns are the settlements with town status (Statistical Office of Slovenia,
// via Wikipedia's list of cities and towns in Slovenia) below the main table's
// population floor. Each is read as a city that states Slovenia when the line states no
// other country, the way a main-table city is. Left out: every town another country also
// has a populated place of, by GeoNames' full dumps of the neighbours (HR, BA, RS, ME,
// MK, AT, IT, HU, CZ, SK, PL, DE, CH) and cities500 worldwide — Jesenice (Czech and
// Croatian), Ruše (Ruse), Ribnica (Rîbnița), Brežice, Žalec, Tržič, Piran, Laško and
// Železniki. Those still name the town wherever the line states Slovenia, through the
// small-places table. Names are GeoNames' spellings, which the small-places table uses.
var slovenianTowns = []string{
	"Kamnik", "Domžale", "Nova Gorica", "Škofja Loka", "Izola", "Murska Sobota", "Logatec",
	"Postojna", "Vrhnika", "Slovenska Bistrica", "Kočevje", "Grosuplje", "Slovenj Gradec",
	"Mengeš", "Ajdovščina", "Ravne na Koroškem", "Krško", "Litija", "Sežana", "Radovljica",
	"Zagorje ob Savi", "Idrija", "Medvode", "Črnomelj", "Rogaška Slatina",
	"Slovenske Konjice", "Bled", "Šentjur", "Hrastnik", "Sevnica", "Prevalje",
	"Ilirska Bistrica", "Cerknica", "Trebnje", "Žiri", "Šempeter pri Gorici",
	"Lenart v Slovenskih Goricah", "Metlika", "Ljutomer", "Gornja Radgona", "Dravograd",
	"Mežica", "Zreče", "Tolmin", "Šoštanj", "Lendava", "Ormož", "Radeče", "Bovec",
	"Višnja Gora", "Kostanjevica na Krki",
}

// slovenianCityForms are the other spellings a Slovenian line names a city by: the
// genitive after "okolica" ("okolica Celja", "okolica Ljubljane") and an administrative
// unit's name. ljubljanaDistricts adds a Ljubljana district after the city ("Ljubljana
// Črnuče"; "Ljubljana-Šiška" already reads through the dash).
var slovenianCityForms = map[string]string{
	"ljubljane": "Ljubljana", "celja": "Celje", "kranja": "Kranj", "kopra": "Koper",
	"ptuja": "Ptuj", "velenja": "Velenje", "novega mesta": "Novo Mesto",
	"murske sobote": "Murska Sobota", "nove gorice": "Nova Gorica", "kamnika": "Kamnik",
	"domžal": "Domžale", "škofje loke": "Škofja Loka",
	// ESS names the place by its administrative unit, and Šentjur's still carries the
	// town's former name.
	"šentjur pri celju": "Šentjur",
}

// ljubljanaDistricts are the seventeen district communities of the City Municipality of
// Ljubljana and the two halves of Vižmarje-Brod, read only after "Ljubljana": a district
// alone ("Center", "Rudnik", "Polje") names nothing.
var ljubljanaDistricts = []string{
	"bežigrad", "center", "črnuče", "dravlje", "golovec", "jarše", "moste", "polje",
	"posavje", "rožnik", "rudnik", "sostro", "šentvid", "šiška", "trnovo", "vič",
	"vižmarje", "brod", "vižmarje-brod",
}

// slovenianSmallPlaces are Slovenian places the small-places table lacks, keyed by the
// name a line writes: municipalities whose seat GeoNames names otherwise or puts under its
// floor, and the villages MojeDelo's postings name. Like the table, each names a city only
// once the line has stated Slovenia (or the caller's countries do), so a namesake abroad
// costs nothing. A bare municipality name is read as the municipality's seat
// ("Brezovica" is Brezovica pri Ljubljani; the other Brezovicas are villages). A
// municipality written "A - B" splits into its two places at the separator, so the
// halves are listed.
var slovenianSmallPlaces = map[string]string{
	// Municipalities and their seats.
	"bloke":                              "Bloke",
	"bohinj":                             "Bohinj",
	"brezovica":                          "Brezovica pri Ljubljani",
	"dobrepolje":                         "Dobrepolje",
	"duplek":                             "Duplek",
	"gorje":                              "Gorje",
	"hajdina":                            "Hajdina",
	"hoče":                               "Hoče",
	"hrpelje-kozina":                     "Hrpelje",
	"ig":                                 "Ig",
	"jezersko":                           "Jezersko",
	"kanal ob soči":                      "Kanal",
	"kozina":                             "Kozina",
	"kungota":                            "Kungota",
	"lenart":                             "Lenart v Slovenskih Goricah",
	"lenart v slov. goricah":             "Lenart v Slovenskih Goricah",
	"ljubno":                             "Ljubno ob Savinji",
	"loška dolina":                       "Loška Dolina",
	"loški potok":                        "Loški Potok",
	"lukovica":                           "Lukovica pri Domžalah",
	"polhov gradec":                      "Polhov Gradec",
	"šentilj":                            "Šentilj v Slov. Goricah",
	"šentilj v slovenskih goricah":       "Šentilj v Slov. Goricah",
	"šmarješke toplice":                  "Šmarješke Toplice",
	"sveta ana v slovenskih goricah":     "Sveta Ana v Slovenskih Goricah",
	"sveta trojica v slovenskih goricah": "Sveta Trojica v Slovenskih Goricah",
	"sveti andraž v slovenskih goricah":  "Sveti Andraž v Slovenskih Goricah",
	"sveti jurij ob ščavnici":            "Sveti Jurij ob Ščavnici",
	"sveti jurij v slovenskih goricah":   "Sveti Jurij v Slovenskih Goricah",
	"videm-dobrepolje":                   "Dobrepolje",
	"vogrsko":                            "Vogrsko",
	"zgornje hoče":                       "Zgornje Hoče",
	"žirovnica":                          "Žirovnica",
	// Villages the postings name: the airport's village, mills, plants, a resort.
	"batuje":                 "Batuje",
	"begunje na gorenjskem":  "Begunje na Gorenjskem",
	"blanca":                 "Blanca",
	"boštanj":                "Boštanj",
	"brnik":                  "Zgornji Brnik",
	"brnik-aerodrom":         "Zgornji Brnik",
	"črni vrh nad idrijo":    "Črni Vrh",
	"dobruška vas":           "Dobruška Vas",
	"drenov grič":            "Drenov Grič",
	"hotavlje":               "Hotavlje",
	"latkova vas":            "Latkova Vas",
	"otočec":                 "Otočec",
	"petrovče":               "Petrovče",
	"planina pri sevnici":    "Planina pri Sevnici",
	"podskrajnik":            "Podskrajnik",
	"prestranek":             "Prestranek",
	"stranice":               "Stranice",
	"straža pri novem mestu": "Straža",
	"volčja draga":           "Volčja Draga",
	"zgornji brnik":          "Zgornji Brnik",
}

// contestedSmallPlaces names a contested main-table alias in a country whose own place of
// the name sits below the main table's floor: "Ribnica" is Rîbnița in Moldova first, so
// "Ribnica, Slovenija" and MojeDelo's "Ribnica" (whose country is Slovenia) named no city.
// Listed by hand, not read off the small-places table: that table keeps one row per name
// and country, so it cannot tell a town from the several villages of its name ("Salem"
// in the US). "Ribnica" is checked: the municipality seat (3,600 people) beside two
// hamlets of 14 and 123.
var contestedSmallPlaces = map[[2]string]string{
	{"ribnica", "si"}: "Ribnica",
}

// slovenianFold writes a Slovenian name without its diacritics, the spelling a line
// typed without them uses ("Skofja Loka", "Krsko").
var slovenianFold = strings.NewReplacer("č", "c", "š", "s", "ž", "z")

// slovenianSpellings returns a name's lowercase spelling and, when it differs, the one
// without diacritics.
func slovenianSpellings(name string) []string {
	lower := strings.ToLower(name)
	if folded := slovenianFold.Replace(lower); folded != lower {
		return []string{lower, folded}
	}
	return []string{lower}
}

// slovenianCityOverrides are the city aliases this file pins: the towns, the genitive
// forms and the Ljubljana districts. They are merged into cityOverrides by
// withSlovenianCities, which panics on an alias cityOverrides already pins.
func slovenianCityOverrides() map[string]cityEntry {
	out := map[string]cityEntry{}
	for _, town := range slovenianTowns {
		for _, alias := range slovenianSpellings(town) {
			out[alias] = cityEntry{Name: town, Country: "si"}
		}
	}
	for form, name := range slovenianCityForms {
		for _, alias := range slovenianSpellings(form) {
			out[alias] = cityEntry{Name: name, Country: "si"}
		}
	}
	for _, district := range ljubljanaDistricts {
		for _, alias := range slovenianSpellings("ljubljana " + district) {
			out[alias] = cityEntry{Name: "Ljubljana", Country: "si"}
		}
	}
	return out
}

// withSlovenianCities returns overrides with slovenianCityOverrides added.
func withSlovenianCities(overrides map[string]cityEntry) map[string]cityEntry {
	out := make(map[string]cityEntry, len(overrides))
	for alias, e := range overrides {
		out[alias] = e
	}
	for alias, e := range slovenianCityOverrides() {
		if _, dup := out[alias]; dup {
			panic("location: slovenian city alias " + alias + " is already in cityOverrides")
		}
		out[alias] = e
	}
	return out
}

// slovenianSmallPlaceRows are slovenianSmallPlaces as small-places entries, in both
// spellings, for loadSmallPlaces' caller to merge.
func slovenianSmallPlaceRows() map[string]smallPlace {
	out := map[string]smallPlace{}
	for alias, name := range slovenianSmallPlaces {
		for _, a := range slovenianSpellings(alias) {
			out[a] = smallPlace{Name: name, Country: "si"}
		}
	}
	return out
}

// init merges the Slovenian country and region names into nameToCountry, and the region
// names into regionNotCity. A name already in nameToCountry is a programming error, as in
// foreignSubdivisionNames.
func init() {
	add := func(name, country string) {
		if existing, dup := nameToCountry[name]; dup {
			panic("location: slovenian name " + name + " is already in nameToCountry as " + existing)
		}
		nameToCountry[name] = country
	}
	for name, country := range slovenianCountryNames {
		add(name, country)
	}
	for _, region := range slovenianRegions {
		forms := []string{region + " regija", "regija " + region}
		if !slovenianRegionsOnlyWithRegija[region] {
			forms = append(forms, region)
		}
		names := map[string]bool{}
		for _, name := range forms {
			for _, spelling := range slovenianSpellings(name) {
				names[spelling] = true
			}
		}
		for name := range names {
			add(name, "si")
			regionNotCity[name] = true
		}
	}
}
