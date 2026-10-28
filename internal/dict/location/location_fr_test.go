package location

import (
	"reflect"
	"testing"
)

// TestFrenchLines pins the French location lines measured over the French crawl (wttj,
// flatchr, werecruit, smartrecruiters, teamtailor, workday, eures, recruitee, lever, join,
// whatjobs-fr) on 2026-10-28.
func TestFrenchLines(t *testing.T) {
	tests := []struct {
		location  string
		countries []string
		cities    []string
		metros    []string
	}{
		// A compound town written with spaces for its hyphens.
		{"Issy Les Moulineaux", []string{"fr"}, []string{"Issy-les-Moulineaux"}, []string{"Paris (FR)"}},
		{"Aix en Provence, France", []string{"fr"}, []string{"Aix-en-Provence"}, []string{"Aix-en-Provence (FR)"}},
		{"Saint Herblain, France", []string{"fr"}, []string{"Saint-Herblain"}, []string{"Nantes (FR)"}},
		// The arrondissement is the city.
		{"Lyon 3e", []string{"fr"}, []string{"Lyon"}, []string{"Lyon (FR)"}},
		{"Paris 19e Arrondissement, France", []string{"fr"}, []string{"Paris"}, []string{"Paris (FR)"}},
		{"Marseille 13e", []string{"fr"}, []string{"Marseille"}, []string{"Marseille (FR)"}},
		{"Paris La Défense", []string{"fr"}, []string{"La Defense"}, []string{"Paris (FR)"}},
		// A French postcode is not a US ZIP.
		{"9th arrondissement of Paris, 75009, Paris, France", []string{"fr"}, []string{"Paris"}, []string{"Paris (FR)"}},
		{"Toulouse (31000), Occitanie, France", []string{"fr"}, []string{"Toulouse"}, []string{"Toulouse (FR)"}},
		{"CHAMALIERES, 63400, Auvergne-Rhône-Alpes, France", []string{"fr"}, []string{"Chamalières"}, []string{"Clermont-Ferrand (FR)"}},
		{"Saint-Denis 97400, La Réunion", []string{"re"}, []string{"Saint-Denis"}, []string{"Saint-Denis (RE)"}},
		// A small town's hyphens are not a dash export: no Maine, no Portugal.
		{"53170 Meslay-du-Maine, France", []string{"fr"}, []string{"Meslay-du-Maine"}, nil},
		{"20137 Porto-Vecchio, France", []string{"fr"}, []string{"Porto-Vecchio"}, nil},
		// The regions and departments state France and no city.
		{"PAYS DE LA LOIRE, France", []string{"fr"}, nil, nil},
		{"Haute-Savoie, Auvergne-Rhône-Alpes, France", []string{"fr"}, nil, nil},
		{"Limoux, Occitanie", []string{"fr"}, []string{"Limoux"}, nil},
		{"Provence-Alpes-Côte d'Azur", []string{"fr"}, nil, nil},
		// The overseas departments keep their own codes.
		{"Ducos, Martinique", []string{"mq"}, []string{"Ducos"}, nil},
		{"Basse-Terre, Guadeloupe", []string{"gp"}, []string{"Basse-Terre"}, nil},
		// "Valence" is Valencia's French name; the line's France names Valence.
		{"26000 Valence, France", []string{"fr"}, []string{"Valence"}, []string{"Valence (FR)"}},
		// A flag is its country.
		{"Lyon 🇫🇷", []string{"fr"}, []string{"Lyon"}, []string{"Lyon (FR)"}},
		// A US state after Paris names Paris, Texas — not a second country.
		{"Paris, TX, us", []string{"us"}, []string{"Paris"}, nil},
		{"Paris, Tennessee, United States", []string{"us"}, []string{"Paris"}, nil},
		{"London, ON", []string{"ca"}, []string{"London"}, []string{"London (CA)"}},
		// A town that is a province keeps it: "Quebec" is the province here, and the
		// closing CA is Canada, not California.
		{"Saint-Laurent, Quebec, CA", []string{"ca"}, []string{"Québec", "Saint-Laurent"}, []string{"Montréal (CA)", "Québec (CA)"}},
	}
	for _, tt := range tests {
		g := Parse(tt.location)
		if !reflect.DeepEqual(g.Countries, tt.countries) || !reflect.DeepEqual(g.Cities, tt.cities) || !reflect.DeepEqual(g.Metros, tt.metros) {
			t.Errorf("Parse(%q) = countries %v cities %v metros %v, want %v %v %v", tt.location, g.Countries, g.Cities, g.Metros, tt.countries, tt.cities, tt.metros)
		}
	}
}

// TestFrenchLinesWithTheAdapterCountry pins the bare town an adapter writes beside a
// structured France: the town GeoNames gives another country first still names its
// French place.
func TestFrenchLinesWithTheAdapterCountry(t *testing.T) {
	for loc, want := range map[string][]string{
		"Valence": {"Valence"},
		"Limours": {"Limours"},
		"PEROLS":  {"Pérols"},
		"Massy":   {"Massy"},
		"Belfort": {"Belfort"},
	} {
		if got := ParseIn(loc, []string{"fr"}).Cities; !reflect.DeepEqual(got, want) {
			t.Errorf("ParseIn(%q, fr).Cities = %v, want %v", loc, got, want)
		}
	}
}
