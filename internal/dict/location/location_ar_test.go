package location

import (
	"reflect"
	"testing"
)

// TestArgentineLines pins the Argentine location lines measured over the Argentine crawl
// (pandapelatam, whatjobs-ar, manatal, hiringroom, teamtailor, workday, cornerstone,
// workable, smartrecruiters, oracle) on 2026-10-27.
func TestArgentineLines(t *testing.T) {
	ba := []string{"Buenos Aires"}
	baMetro := []string{"Buenos Aires (AR)"}
	tests := []struct {
		location  string
		countries []string
		cities    []string
		metros    []string
	}{
		// The autonomous city in every spelling a line writes it: the city of Buenos Aires.
		{"CABA", []string{"ar"}, ba, baMetro},
		{"C.A.B.A.", []string{"ar"}, ba, baMetro},
		{"CABA, Argentina", []string{"ar"}, ba, baMetro},
		{"Capital Federal, Capital Federal, Argentina", []string{"ar"}, ba, baMetro},
		{"Ciudad de Buenos Aires", []string{"ar"}, ba, baMetro},
		{"Ciudad Autónoma de Buenos Aires, Argentina", []string{"ar"}, ba, baMetro},
		{"Buenos Aires City, AR", []string{"ar"}, ba, baMetro},
		{"BUENOS AIRES CITY, ARG", []string{"ar"}, ba, baMetro},
		{"CABA, CABA, Argentina", []string{"ar"}, ba, baMetro},
		// A barrio of the city is the city, once the line places it there.
		{"Recoleta, CABA, Argentina", []string{"ar"}, ba, baMetro},
		{"Palermo, Argentina", []string{"ar"}, ba, baMetro},
		{"Palermo, Buenos Aires, Argentina", []string{"ar"}, ba, baMetro},
		{"Retiro, AR", []string{"ar"}, ba, baMetro},
		{"Núñez, Ciudad Autónoma de Buenos Aires, Argentina", []string{"ar"}, ba, baMetro},
		// ...and only then: a barrio's name elsewhere keeps its own reading.
		{"Palermo, Italia", []string{"it"}, []string{"Palermo"}, []string{"Palermo (IT)"}},
		{"San Cristóbal, Santa Fe, Argentina", []string{"ar"}, nil, nil},
		// The conurbation states the country and the metro, and no city.
		{"GBA", []string{"ar"}, nil, baMetro},
		{"Gran Buenos Aires, Argentina", []string{"ar"}, nil, baMetro},
		{"AMBA, Argentina", []string{"ar"}, nil, baMetro}, // was Embi, Kazakhstan
		{"Zona Norte, GBA", []string{"ar"}, nil, baMetro},
		// The towns of Greater Buenos Aires: their own name, in the capital's metro.
		{"Vicente López, Buenos Aires Province, Argentina", []string{"ar"}, []string{"Vicente López"}, baMetro},
		{"Munro, Argentina", []string{"ar"}, []string{"Munro"}, baMetro},
		{"Partido de General San Martín, Argentina", []string{"ar"}, []string{"General San Martín"}, baMetro},
		{"Gral. Pacheco, Buenos Aires, Argentina", []string{"ar"}, []string{"General Pacheco"}, baMetro},
		{"Pilar, Buenos Aires, Argentina", []string{"ar"}, []string{"Pilar"}, baMetro},
		{"Florida, Buenos Aires Province, Argentina", []string{"ar"}, []string{"Florida"}, baMetro}, // was the US state
		{"Martínez, Argentina", []string{"ar"}, []string{"Martínez"}, baMetro},                      // was Martinez, CA
		{"Ciudad Evita, Argentina", []string{"ar"}, []string{"Ciudad Evita"}, baMetro},              // was Cañada de Gómez
		// A name another Argentine place shares needs Buenos Aires itself in the line.
		{"Pilar, Argentina", []string{"ar"}, []string{"Pilar"}, nil},
		{"San Martín, Mendoza, Argentina", []string{"ar"}, nil, nil},
		{"Pilar, Córdoba, Argentina", []string{"ar"}, []string{"Pilar"}, nil}, // Córdoba's Pilar
		// A province named after its capital, after a town, is the province.
		{"Rosario, Santa Fe, Argentina", []string{"ar"}, []string{"Rosario"}, []string{"Rosario (AR)"}},
		{"Villa María, Córdoba, Argentina", []string{"ar"}, []string{"Villa María"}, []string{"Villa María (AR)"}},
		{"San Rafael, Mendoza, Argentina", []string{"ar"}, []string{"San Rafael"}, []string{"San Rafael (AR)"}},
		// The provinces state the country and no city.
		{"Buenos Aires Province", []string{"ar"}, nil, nil},
		{"Provincia de Buenos Aires, Argentina", []string{"ar"}, nil, nil},
		{"Córdoba Province, AR", []string{"ar"}, nil, nil},
		{"Santiago del Estero Province, Argentina", []string{"ar"}, nil, nil},
		{"Chaco", []string{"ar"}, nil, nil},
		{"La Pampa, Argentina", []string{"ar"}, nil, nil},
		// The metros GHS-FUA labels by a suburb are labelled by their city.
		{"Mendoza, Argentina", []string{"ar"}, []string{"Mendoza"}, []string{"Mendoza (AR)"}},
		{"San Juan, Argentina", []string{"ar"}, []string{"San Juan"}, []string{"San Juan (AR)"}},
		// Spain and Mexico keep their Córdoba.
		{"Córdoba, España", []string{"es"}, []string{"Córdoba"}, []string{"Córdoba (ES)"}},
		{"Córdoba, Argentina", []string{"ar"}, []string{"Córdoba"}, []string{"Córdoba (AR)"}},
	}
	for _, tt := range tests {
		g := Parse(tt.location)
		if !reflect.DeepEqual(g.Countries, tt.countries) || !reflect.DeepEqual(g.Cities, tt.cities) || !reflect.DeepEqual(g.Metros, tt.metros) {
			t.Errorf("Parse(%q) = countries %v cities %v metros %v, want %v %v %v", tt.location,
				g.Countries, g.Cities, g.Metros, tt.countries, tt.cities, tt.metros)
		}
	}
}

// TestArkansasIsNotArgentina pins the "AR" a US line writes for Arkansas. The towns are
// below the main city table's floor, so only the small-places table knows them, and an
// unconfirmed AR used to fall through to Argentina.
func TestArkansasIsNotArgentina(t *testing.T) {
	tests := []struct {
		location  string
		countries []string
	}{
		{"Atkins, AR", []string{"us"}},
		{"Mountain Home, AR", []string{"us"}},
		{"Forrest City, AR", []string{"us"}},
		{"Crossett, AR", []string{"us"}},
		// An Argentine place, or one nobody knows, keeps the country reading.
		{"Retiro, AR", []string{"ar"}},
		{"Buenos Aires, AR", []string{"ar"}},
		{"Totuguitas, AR", []string{"ar"}},
		{"San Luis, AR", []string{"ar"}},
		// A code between a town and another country is that country's subdivision: Aragón,
		// Appenzell Ausserrhoden, Arad county.
		{"Zaragoza, AR, España", []string{"es"}},
		{"Herisau, AR, ch", []string{"ch"}},
		{"Arad, AR, ro", []string{"ro"}},
		// A list of country codes is still a list.
		{"MX, AR, BR", []string{"ar", "br", "mx"}},
		{"Remote, AR, CL", []string{"ar", "cl"}},
	}
	for _, tt := range tests {
		if g := Parse(tt.location); !reflect.DeepEqual(g.Countries, tt.countries) {
			t.Errorf("Parse(%q) countries = %v, want %v", tt.location, g.Countries, tt.countries)
		}
	}
}

// TestParseInResolvesBarePlaces pins what an adapter's structured country does for a line
// that names only a place: pandapelatam writes the barrio alone ("Palermo") and states
// the country beside it.
func TestParseInResolvesBarePlaces(t *testing.T) {
	tests := []struct {
		location string
		hint     []string
		cities   []string
		metros   []string
	}{
		{"Palermo", []string{"ar"}, []string{"Buenos Aires"}, []string{"Buenos Aires (AR)"}},
		{"San Nicolás", []string{"ar"}, []string{"Buenos Aires"}, []string{"Buenos Aires (AR)"}},
		{"Vicente López", []string{"ar"}, []string{"Vicente López"}, []string{"Buenos Aires (AR)"}},
		{"Partido de General San Martín", []string{"ar"}, []string{"General San Martín"}, []string{"Buenos Aires (AR)"}},
		{"Pilar", []string{"ar"}, []string{"Pilar"}, nil}, // Córdoba has one too
		{"Rosario", []string{"ar"}, []string{"Rosario"}, []string{"Rosario (AR)"}},
		{"Cordoba", []string{"ar"}, []string{"Córdoba"}, []string{"Córdoba (AR)"}},
		// Without the hint, nothing changes.
		{"Palermo", nil, []string{"Palermo"}, nil},
		// The hint names no country of its own.
		{"Palermo", []string{"it"}, []string{"Palermo"}, []string{"Palermo (IT)"}},
	}
	for _, tt := range tests {
		g := ParseIn(tt.location, tt.hint)
		if len(g.Countries) != 0 || !reflect.DeepEqual(g.Cities, tt.cities) || !reflect.DeepEqual(g.Metros, tt.metros) {
			t.Errorf("ParseIn(%q, %v) = countries %v cities %v metros %v, want none %v %v", tt.location, tt.hint,
				g.Countries, g.Cities, g.Metros, tt.cities, tt.metros)
		}
	}
}
