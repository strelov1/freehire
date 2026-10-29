package location

import (
	"reflect"
	"testing"
)

// TestBCSLines pins the Croatian, Serbian, Bosnian and Montenegrin location lines
// measured over the postings placed in hr/rs/ba/me on 2026-10-29, and the lines the
// Serbian and Croatian boards write (infostud, hzz, mojposao).
func TestBCSLines(t *testing.T) {
	tests := []struct {
		location  string
		countries []string
		cities    []string
	}{
		// The countries in their own languages, both standards, and Serbian Cyrillic.
		{"Srbija", []string{"rs"}, nil},
		{"Србија", []string{"rs"}, nil},
		{"Bosna i Hercegovina", []string{"ba"}, nil},
		{"BiH", []string{"ba"}, nil},
		{"Crna Gora", []string{"me"}, nil},
		{"Cijela Hrvatska", []string{"hr"}, nil},
		// A Serbian board's posting abroad names the country it is in.
		{"Nemačka - Hanover", []string{"de"}, []string{"Hannover"}},
		{"Njemačka", []string{"de"}, nil},
		{"Немачка", []string{"de"}, nil},
		{"Austrija", []string{"at"}, nil},
		{"Švajcarska", []string{"ch"}, nil},
		{"Švicarska", []string{"ch"}, nil},
		{"Severna Makedonija", []string{"mk"}, nil},
		{"Austrija, Njemačka, HR", []string{"at", "de", "hr"}, nil},
		// Workday's ISO-3 export.
		{"SERBIA VIRTUAL, SRB", []string{"rs"}, nil},
		{"HRV-Remote", []string{"hr"}, nil},
		// Kosovo is the country, not the town of Kosovo Polje.
		{"Kosovo", []string{"xk"}, nil},
		{"Pristina, Kosovo", []string{"xk"}, []string{"Pristina"}},
		// The Croatian counties, with and without "županija" and in capitals; the City of
		// Zagreb; Serbia's and Bosnia's provinces and entities.
		{"Splitsko-dalmatinska županija", []string{"hr"}, nil},
		{"SPLITSKO-DALMATINSKA", []string{"hr"}, nil},
		{"Primorsko-goranska zupanija", []string{"hr"}, nil},
		{"GRAD ZAGREB", []string{"hr"}, []string{"Zagreb"}},
		{"Medimurska zupanija", []string{"hr"}, nil},
		{"Međimurje", []string{"hr"}, nil},
		{"Istra, Russia", []string{"ru"}, []string{"Istra"}},
		{"Vojvodina", []string{"rs"}, nil},
		{"Republika Srpska", []string{"ba"}, nil},
		// The locative a line puts a city in.
		{"u Beogradu", []string{"rs"}, []string{"Belgrade"}},
		{"Novom Sadu", []string{"rs"}, []string{"Novi Sad"}},
		{"u Zagrebu", []string{"hr"}, []string{"Zagreb"}},
		// Cyrillic city names.
		{"Нови Сад", []string{"rs"}, []string{"Novi Sad"}},
		{"Панчево", []string{"rs"}, []string{"Pančevo"}},
		{"Београд", []string{"rs"}, []string{"Belgrade"}},
		// The cities whose name GeoNames also gives a hamlet abroad.
		{"Pula", []string{"hr"}, []string{"Pula"}},
		{"Dubrovnik", []string{"hr"}, []string{"Dubrovnik"}},
		{"Leskovac", []string{"rs"}, []string{"Leskovac"}},
		// Novi Sad is not the Michigan town of Novi, and Novi alone states no country.
		{"Novi Sad", []string{"rs"}, []string{"Novi Sad"}},
		{"Novi, Michigan", []string{"us"}, []string{"Novi"}},
		{"Novi", nil, []string{"Novi"}},
		// A Brazilian line naming Brazil reads its state code as the state, not as
		// Bosnia or Serbia.
		{"Luís Eduardo Magalhães, BA, Brasil", []string{"br"}, nil},
		{"Porto Alegre, RS, Brasil", []string{"br"}, []string{"Porto Alegre"}},
	}
	for _, tt := range tests {
		g := Parse(tt.location)
		if !reflect.DeepEqual(g.Countries, tt.countries) || !reflect.DeepEqual(g.Cities, tt.cities) {
			t.Errorf("Parse(%q) = countries %v cities %v, want %v %v", tt.location, g.Countries, g.Cities, tt.countries, tt.cities)
		}
	}
}
