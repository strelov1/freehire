package location

import (
	"reflect"
	"testing"
)

// TestSlovenianLines pins the Slovenian location lines measured over 2,900 open MojeDelo
// postings on 2026-10-29, read the way a source with no structured country reads them.
func TestSlovenianLines(t *testing.T) {
	tests := []struct {
		location  string
		countries []string
		cities    []string
	}{
		// The statistical and traditional regions state Slovenia and no city.
		{"Osrednjeslovenska", []string{"si"}, nil},
		{"Osrednjeslovenska regija", []string{"si"}, nil},
		{"Savinjska", []string{"si"}, nil},
		{"Jugovzhodna Slovenija", []string{"si"}, nil},
		{"Obalno-kraška", []string{"si"}, nil},
		{"Obalno-kraska", []string{"si"}, nil},
		{"Goriška regija", []string{"si"}, nil},
		{"Dolenjska", []string{"si"}, nil},
		{"Posavje", []string{"si"}, nil},
		{"Koroška regija", []string{"si"}, nil},
		// Withheld: Austrian Carinthia and Styria are "Koroška" and "Štajerska" too.
		{"Koroška", nil, nil},
		{"Štajerska", nil, nil},
		// Slovenia as an area, and the countries abroad in Slovenian.
		{"Celotna Slovenija", []string{"si"}, nil},
		{"Slovenija", []string{"si"}, nil},
		{"Avstrija", []string{"at"}, nil},
		{"Nemčija", []string{"de"}, nil},
		{"Hrvaška", []string{"hr"}, nil},
		// A town with town status names the city and states Slovenia, with and without
		// its diacritics.
		{"Kamnik", []string{"si"}, []string{"Kamnik"}},
		{"Škofja Loka", []string{"si"}, []string{"Škofja Loka"}},
		{"Skofja Loka", []string{"si"}, []string{"Škofja Loka"}},
		{"Krško", []string{"si"}, []string{"Krško"}},
		{"Krsko", []string{"si"}, []string{"Krško"}},
		{"Murska Sobota", []string{"si"}, []string{"Murska Sobota"}},
		{"Slovenske Konjice", []string{"si"}, []string{"Slovenske Konjice"}},
		// A town another country also has a place of states nothing alone, and names
		// the town once the line says Slovenia.
		{"Jesenice", nil, nil},
		{"Jesenice, Slovenija", []string{"si"}, []string{"Jesenice"}},
		{"Brežice", nil, nil},
		{"Brežice, Slovenia", []string{"si"}, []string{"Brežice"}},
		// Ribnica is Rîbnița first; a line that says Slovenia names the Slovenian town.
		{"Ribnica, Slovenija", []string{"si"}, []string{"Ribnica"}},
		// The area around a town.
		{"Ljubljana in okolica", []string{"si"}, []string{"Ljubljana"}},
		{"Celje z okolico", []string{"si"}, []string{"Celje"}},
		{"Nova Gorica z bližnjo okolico", []string{"si"}, []string{"Nova Gorica"}},
		{"okolica Celja", []string{"si"}, []string{"Celje"}},
		{"Okolica Ljubljane", []string{"si"}, []string{"Ljubljana"}},
		// A Ljubljana district.
		{"Ljubljana Črnuče", []string{"si"}, []string{"Ljubljana"}},
		{"Ljubljana Siska", []string{"si"}, []string{"Ljubljana"}},
		// A village or municipality seat below GeoNames' floor names a city only beside
		// Slovenia.
		{"Kozina", nil, nil},
		{"Kozina, Slovenija", []string{"si"}, []string{"Kozina"}},
		{"Hoče - Slivnica, Slovenija", []string{"si"}, []string{"Hoče"}},
		{"Brnik, Slovenia", []string{"si"}, []string{"Zgornji Brnik"}},
	}
	for _, tt := range tests {
		g := Parse(tt.location)
		if !reflect.DeepEqual(g.Countries, tt.countries) || !reflect.DeepEqual(g.Cities, tt.cities) {
			t.Errorf("Parse(%q) = countries %v cities %v, want %v %v", tt.location, g.Countries, g.Cities, tt.countries, tt.cities)
		}
	}
}

// TestSlovenianLinesWithKnownCountry pins what MojeDelo's lines name: the town alone, with
// Slovenia as the adapter's structured country.
func TestSlovenianLinesWithKnownCountry(t *testing.T) {
	tests := []struct {
		location string
		cities   []string
	}{
		{"Grosuplje", []string{"Grosuplje"}},
		{"Jesenice", []string{"Jesenice"}},
		{"Ribnica", []string{"Ribnica"}},
		{"Boštanj", []string{"Boštanj"}},
		{"Lenart", []string{"Lenart v Slovenskih Goricah"}},
		{"Zgornji Brnik", []string{"Zgornji Brnik"}},
		{"Videm-Dobrepolje", []string{"Dobrepolje"}},
		{"Dobruška vas", []string{"Dobruška Vas"}},
		{"Savinjska", nil},
		// Abroad, the line's own country wins over the adapter's.
		{"Avstrija", nil},
	}
	for _, tt := range tests {
		g := ParseIn(tt.location, []string{"si"})
		if !reflect.DeepEqual(g.Cities, tt.cities) {
			t.Errorf("ParseIn(%q, si) cities = %v, want %v", tt.location, g.Cities, tt.cities)
		}
	}
}

// TestSlovenianNamesDoNotCollide holds the Slovenian tables to the names they may take:
// a town pinned here must not be a country or region name, and a small place must not be
// one the generated table already lists for Slovenia (it would only duplicate it, or, under
// another display name, make the name ambiguous and name nothing).
func TestSlovenianNamesDoNotCollide(t *testing.T) {
	for alias := range slovenianCityOverrides() {
		if c, ok := nameToCountry[alias]; ok {
			t.Errorf("slovenian city alias %q is also a country or region name (%s)", alias, c)
		}
	}
	generated := loadSmallPlaces(smallPlacesTSV)
	for alias := range slovenianSmallPlaceRows() {
		for _, p := range generated[alias] {
			if p.Country == "si" {
				t.Errorf("slovenian small place %q is already in smallplaces.tsv as %q", alias, p.Name)
			}
		}
		if _, ok := cityDict[alias]; ok {
			t.Errorf("slovenian small place %q is shadowed by the city table", alias)
		}
	}
}
