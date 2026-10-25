package location

import (
	"slices"
	"testing"
)

// Dutch and Flemish location lines. Every input is a real location field from the Dutch
// crawl (eures, recruitee; 2026-10-25) unless the case says otherwise.
func TestParseDutchLocations(t *testing.T) {
	tests := []struct {
		name      string
		location  string
		countries []string
		cities    []string
	}{
		// Recruitee writes the country in the posting's language. "Nederland" is also a
		// town in Colorado, which the line used to name instead of the country.
		{"Dutch country name", "Almere, Flevoland, Nederland", []string{"nl"}, []string{"Almere Stad"}},
		{"Dutch country name alone", "Nederland", []string{"nl"}, nil},
		{"Flemish country name", "Gent, Oost-Vlaanderen, België", []string{"be"}, []string{"Gent"}},
		{"French country name", "Liège, Belgique", []string{"be"}, []string{"Liège"}},
		{"Dutch name of another country", "Londen, Verenigd Koninkrijk", []string{"gb"}, []string{"London"}},
		{"Norwegian Nynorsk", "Bergen, Noreg", []string{"no"}, []string{"Bergen"}},
		{"Estonian", "Elva, Tartumaa, Eesti", []string{"ee"}, []string{"Elva"}},
		{"Greek", "Ελλάδα", []string{"gr"}, nil},
		// A province states the country and no city: Limburg is also a German town and a
		// Belgian province, so it states neither.
		{"Dutch province", "Tilburg, Noord-Brabant, Nederland", []string{"nl"}, []string{"Tilburg"}},
		{"Limburg is no city", "Maastricht, Limburg, Nederland", []string{"nl"}, []string{"Maastricht"}},
		{"province alone", "Gelderland", []string{"nl"}, nil},
		{"province named after its capital", "Amersfoort, Utrecht, Nederland", []string{"nl"}, []string{"Amersfoort"}},
		{"a list naming the capital", "Amersfoort or Utrecht, Nederland", []string{"nl"}, []string{"Amersfoort", "Utrecht"}},
		{"the capital itself", "Utrecht, Utrecht, Nederland", []string{"nl"}, []string{"Utrecht"}},
		{"Den Bosch", "Den Bosch, Noord-Brabant, Nederland", []string{"nl"}, []string{"'s-Hertogenbosch"}},
		// The province code Dutch writes after a town to tell it from its namesakes.
		// "NH", "NB" and "UT" are also New Hampshire, New Brunswick and Utah.
		{"province code OV", "HENGELO OV, Netherlands", []string{"nl"}, []string{"Hengelo"}},
		{"province code ZH", "RIJSWIJK ZH, Netherlands", []string{"nl"}, []string{"Rijswijk"}},
		{"province code NB is not New Brunswick", "OOSTERHOUT NB, Netherlands", []string{"nl"}, []string{"Oosterhout"}},
		{"province code NH is not New Hampshire", "HOORN NH, Netherlands", []string{"nl"}, []string{"Hoorn"}},
		{"province code UT is not Utah", "VIANEN UT, Netherlands", []string{"nl"}, []string{"Vianen"}},
		{"Salt Lake City UT stays in Utah (synthetic)", "Salt Lake City UT", []string{"us"}, nil},
		// A postcode before the town, and the area words (synthetic).
		{"Dutch postcode", "1012 AB Amsterdam", []string{"nl"}, []string{"Amsterdam"}},
		{"regio", "Regio Utrecht", []string{"nl"}, []string{"Utrecht"}},
		// Zwolle is also a Louisiana town, so the city alone states no country.
		{"en omgeving", "Zwolle en omgeving", nil, []string{"Zwolle"}},
		{"Randstad", "Randstad", []string{"nl"}, nil},
		{"The Hague in Dutch", "'s-Gravenhage, Zuid-Holland", []string{"nl"}, []string{"The Hague"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			g := Parse(tc.location)
			if !slices.Equal(g.Countries, tc.countries) || !slices.Equal(g.Cities, tc.cities) {
				t.Errorf("Parse(%q) = countries %v cities %v, want countries %v cities %v",
					tc.location, g.Countries, g.Cities, tc.countries, tc.cities)
			}
		})
	}
}

// TestParseRecruiteeRemoteLabels pins the remote label Recruitee writes in the posting's
// language in place of a location, and only as the whole line.
func TestParseRecruiteeRemoteLabels(t *testing.T) {
	for _, loc := range []string{"Werken op afstand", "Homeoffice", "Poste à distance",
		"Trabajo a distancia", "Praca zdalna", "Trabalho remoto"} {
		if g := Parse(loc); g.WorkMode != "remote" {
			t.Errorf("Parse(%q).WorkMode = %q, want remote", loc, g.WorkMode)
		}
	}
	for _, loc := range []string{"Berlin, Homeoffice möglich", "Kein Homeoffice, München"} {
		if g := Parse(loc); g.WorkMode == "remote" {
			t.Errorf("Parse(%q).WorkMode = remote, want the label read only as the whole line", loc)
		}
	}
}

// TestNormalizeCountryExonyms pins the country names an adapter passes through
// NormalizeCountry from a structured country field, written in the posting's language.
func TestNormalizeCountryExonyms(t *testing.T) {
	for name, want := range map[string]string{
		"Nederland": "nl", "België": "be", "Belgie": "be", "Belgique": "be",
		"Norge": "no", "Noreg": "no", "Deutschland": "de", "Österreich": "at",
		"Schweiz": "ch", "Suisse": "ch", "Svizzera": "ch", "Danmark": "dk", "Sverige": "se",
		"España": "es", "Italia": "it", "Polska": "pl", "Eesti": "ee", "Česko": "cz",
		"Verenigd Koninkrijk": "gb", "Duitsland": "de",
	} {
		if got := NormalizeCountry(name); got != want {
			t.Errorf("NormalizeCountry(%q) = %q, want %q", name, got, want)
		}
	}
}
