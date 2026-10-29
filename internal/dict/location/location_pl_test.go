package location

import (
	"reflect"
	"testing"
)

// TestPolishLines pins the Polish location lines measured over the Polish crawl
// (nofluffjobs, whatjobs-pl, erecruiter, traffit, solidjobs, smartrecruiters, teamtailor,
// recruitee, eures) on 2026-10-29.
func TestPolishLines(t *testing.T) {
	tests := []struct {
		location  string
		countries []string
		cities    []string
		metros    []string
	}{
		// eures' street address: the Polish postcode before the town.
		{"Klimczaka 2, 02-797 Warszawa, Polska, Poland", []string{"pl"}, []string{"Warsaw"}, []string{"Warsaw (PL)"}},
		{"Dębowa 44, 40-109 Katowice, Polska, Poland", []string{"pl"}, []string{"Katowice"}, []string{"Katowice (PL)"}},
		{"Przemysłowa 3, 24-300 OPOLE LUBELSKIE, Polska, Poland", []string{"pl"}, []string{"Opole Lubelskie"}, nil},
		// A metropolitan region is a metro and no city.
		{"Trójmiasto, Poland", []string{"pl"}, nil, []string{"Gdańsk (PL)"}},
		{"Tricity, Pomorskie, Poland", []string{"pl"}, nil, []string{"Gdańsk (PL)"}},
		{"Śląsk", []string{"pl"}, nil, []string{"Katowice (PL)"}},
		// A voivodeship alone states the country.
		{"Mazowieckie", []string{"pl"}, nil, nil},
		{"Lesser Poland Voivodeship", []string{"pl"}, nil, nil},
		{"województwo dolnośląskie", []string{"pl"}, nil, nil},
		// Łódź with one diacritic dropped.
		{"Lódz, Poland", []string{"pl"}, []string{"Łódź"}, []string{"Łódź (PL)"}},
		{"Łodź, Poland", []string{"pl"}, []string{"Łódź"}, []string{"Łódź (PL)"}},
		// Already right, and kept: the voivodeship beside its town, the US namesakes.
		{"Lublin, Lublin Voivodeship, pl", []string{"pl"}, []string{"Lublin"}, []string{"Lublin (PL)"}},
		{"Lublin, US", []string{"us"}, []string{"Lublin"}, nil},
		{"Warsaw, IN, us", []string{"us"}, []string{"Warsaw"}, nil},
	}
	for _, tt := range tests {
		g := Parse(tt.location)
		if !reflect.DeepEqual(g.Countries, tt.countries) || !reflect.DeepEqual(g.Cities, tt.cities) || !reflect.DeepEqual(g.Metros, tt.metros) {
			t.Errorf("Parse(%q) = countries %v cities %v metros %v, want %v %v %v",
				tt.location, g.Countries, g.Cities, g.Metros, tt.countries, tt.cities, tt.metros)
		}
	}
}
