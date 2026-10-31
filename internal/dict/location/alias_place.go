package location

import (
	"bufio"
	"slices"
	"strings"
	"sync"
)

// GeoNames lists a town's old, foreign and colloquial names among its aliases, and the
// city table hands an alias to the most populous place carrying it (loadCityDict). That
// is right for the names people still write — "Calcutta", "Bombay", "Peking" — and wrong
// for a name a big city carried once and a smaller town carries now: "Harrison" was
// Scranton's name until 1851 and is a town in New Jersey today, "Morton" an old name of
// Mandan and a town in Illinois, "Frisco" San Francisco's nickname and a city in Texas.
// Nothing in the dataset tells the two kinds apart — cities1000 carries no flag saying
// which alternate names are historical — but a location line usually does: it states the
// country and, in North America, the state. addAliasCity holds a city read through an
// alias to that statement.

// addAliasCity adds to cities the city a line names through tok, an alias of the
// differently named place ce, given the line's countries and its location segment's
// US/CA subdivision codes — or nothing, when it names none.
//
// A line that states ce's country keeps ce unless the segment also states subdivisions
// of that country and ce lies in none of them; then the town called tok in a stated subdivision
// is the city, or there is none ("Harrison, NJ" is Harrison, not Scranton, PA). A line
// that states other countries names that country's own place of the name, when it has
// exactly one, and never ce ("Bombay, NY" is not Mumbai). A line that states no country
// keeps ce only when the alias is uncontested: a contested one waits for a country.
func addAliasCity(cities map[string]struct{}, ce cityEntry, tok string, stated, subdivisions map[string]struct{}) {
	add := func(name string) {
		if name != "" {
			cities[name] = struct{}{}
		}
	}
	if len(stated) == 0 {
		if !ce.Contested {
			add(ce.Name)
		}
		return
	}
	if _, ok := stated[ce.Country]; ok {
		if subs := subdivisionsOf(ce.Country, subdivisions); len(subs) > 0 && !placeInSubdivision(ce.Name, ce.Country, subs) {
			add(townIn(tok, ce.Country, subs))
			return
		}
		add(ce.Name)
		return
	}
	for c := range stated {
		// "Valence, France" is Valence, where the table's first reading of "valence" is
		// Valencia; "Ribnica, Slovenija" the Slovenian town below the main table's floor,
		// where it is Rîbnița in Moldova (contestedSmallPlaces).
		if name := townIn(tok, c, subdivisionsOf(c, subdivisions)); name != "" {
			add(name)
		} else if name := contestedCityIn(tok, c); name != "" {
			add(name)
		} else {
			add(contestedSmallPlaces[[2]string{tok, c}])
		}
	}
}

// sameCityName reports whether the city table's name for a place is the token that read
// it, rather than an alias of it.
func sameCityName(name, tok string) bool {
	return strings.EqualFold(foldIndianName(name), foldIndianName(tok))
}

// subdivisionsOf returns the stated subdivision codes that belong to country.
func subdivisionsOf(country string, subdivisions map[string]struct{}) []string {
	var out []string
	for code := range subdivisions {
		if subdivisionToCountry[strings.ToLower(code)] == country {
			out = append(out, code)
		}
	}
	return out
}

// placeInSubdivision reports whether the city table's place name lies in one of the
// subdivisions. A place the metro table does not locate at all counts as lying in them:
// the check vetoes only a place known to be elsewhere.
func placeInSubdivision(name, country string, subs []string) bool {
	key := [2]string{strings.ToLower(name), country}
	if from, ok := cityRenamedFrom[key]; ok {
		key[0] = from // the metro table speaks GeoNames names, not the curated display name
	}
	located := placeSubdivisions()[key]
	return len(located) == 0 || slices.ContainsFunc(subs, func(code string) bool { return located[code] })
}

// townIn returns the US/CA place of the city table called tok in one of the
// subdivisions of country; "" when there is none, or when two differently named places
// qualify. Only a place's OWN name counts — an alias here would repeat the reading being
// corrected one state down ("Newton, NC" would be Wilmington, whose old name it is) — but
// spelt the way the line spells it: "St Petersburg, FL" is St. Petersburg.
func townIn(tok, country string, subs []string) string {
	found := ""
	for _, name := range northAmericanTowns()[[2]string{townKey(tok), country}] {
		located := placeSubdivisions()[[2]string{strings.ToLower(name), country}]
		if !slices.ContainsFunc(subs, func(code string) bool { return located[code] }) {
			continue
		}
		if found != "" && found != name {
			return ""
		}
		found = name
	}
	return found
}

// townKey folds the spellings of one town's name that differ only by punctuation or an
// abbreviated first word: "St Petersburg" and "St. Petersburg", "Saint Charles" and "St.
// Charles".
func townKey(name string) string {
	f := strings.Fields(townPunctuation.Replace(strings.ToLower(name)))
	if len(f) > 0 {
		if full, ok := townAbbreviations[f[0]]; ok {
			f[0] = full
		}
	}
	return strings.Join(f, " ")
}

var (
	townPunctuation   = strings.NewReplacer(".", "", "'", "", "’", "")
	townAbbreviations = map[string]string{"st": "saint", "ste": "sainte", "ft": "fort", "mt": "mount"}
)

// placeSubdivisions maps a place of the metro table (lowercased GeoNames name, country) to
// the US/CA subdivisions a place of that name lies in. The metro table lists every place
// the city table can read with its admin1 code (cmd/gen-metros).
var placeSubdivisions = sync.OnceValue(func() map[[2]string]map[string]bool {
	out := map[[2]string]map[string]bool{}
	for _, line := range strings.Split(metrosTSV, "\n") {
		f := strings.Split(line, "\t")
		if len(f) != 4 || f[2] == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key := [2]string{f[0], f[1]}
		if out[key] == nil {
			out[key] = map[string]bool{}
		}
		out[key][f[2]] = true
	}
	return out
})

// northAmericanTowns maps a townKey and a country (us, ca) to the display names of the
// city table's places whose own name it is — every one of them, not first-wins like
// cityDict, since the stated subdivision is what picks among them.
var northAmericanTowns = sync.OnceValue(func() map[[2]string][]string {
	out := map[[2]string][]string{}
	sc := bufio.NewScanner(strings.NewReader(citiesTSV))
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for sc.Scan() {
		parts := strings.SplitN(sc.Text(), "\t", 3)
		if len(parts) < 3 || (parts[1] != "us" && parts[1] != "ca") {
			continue
		}
		key := [2]string{townKey(parts[0]), parts[1]}
		out[key] = append(out[key], parts[0])
	}
	return out
})
