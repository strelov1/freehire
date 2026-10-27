package location

import (
	"reflect"
	"testing"
)

// TestBrazilianAndPortugueseLines pins the location lines of the Brazilian and Portuguese
// crawl (gupy, solides, quickin, catho, empregos, vagas, inhire, pandape, smartrecruiters,
// workday, teamtailor, whatjobs-br/pt), measured 2026-10-27.
func TestBrazilianAndPortugueseLines(t *testing.T) {
	tests := []struct {
		location  string
		countries []string
		cities    []string
	}{
		// A Brazilian state code is the state, not the country or US state it spells:
		// PR was Puerto Rico, SC South Carolina, BA Bosnia, ES Spain, PE Peru.
		{"Curitiba, PR", []string{"br"}, []string{"Curitiba"}},
		{"Curitiba, PR, Brasil", []string{"br"}, []string{"Curitiba"}},
		{"Blumenau, SC", []string{"br"}, []string{"Blumenau"}},
		{"Florianópolis, SC, Brasil", []string{"br"}, []string{"Florianópolis"}},
		{"Salvador, BA", []string{"br"}, []string{"Salvador"}},
		{"Vitória, ES", []string{"br"}, []string{"Vitória"}},
		{"Recife, PE", []string{"br"}, []string{"Recife"}},
		{"Porto Alegre, RS, BR", []string{"br"}, []string{"Porto Alegre"}},
		{"Manaus, AM, Brasil", []string{"br"}, []string{"Manaus"}},
		{"Cuiabá, MT, Brasil", []string{"br"}, []string{"Cuiabá"}},
		{"Dourados, MS", []string{"br"}, []string{"Dourados"}},
		{"São Luís, MA, BR", []string{"br"}, []string{"São Luís"}},
		// The town is Brazilian only in a line the state code places in Brazil.
		{"Colombo, PR", []string{"br"}, []string{"Colombo"}},
		{"Fortaleza, CE", []string{"br"}, []string{"Fortaleza"}},
		{"Natal, RN", []string{"br"}, []string{"Natal"}},
		{"Santo André, SP", []string{"br"}, []string{"Santo André"}},
		{"Boa Vista, RR", []string{"br"}, []string{"Boa Vista"}},
		{"Concordia, SC, Brasil", []string{"br"}, []string{"Concórdia"}},
		{"Laguna, SC", []string{"br"}, []string{"Laguna"}},
		{"Conceição, BA, BR", []string{"br"}, nil},
		// A country and its own ISO code are that country, not a town and a state.
		{"Juan díaz, Panamá, PA", []string{"pa"}, []string{"Juan Díaz"}},
		{"Panamá PA", nil, nil}, // not Brazil, though Goiás has a town called Panamá
		{"Localização não informada, PR (Brasil)", []string{"br"}, nil},
		// The separators the crawl writes: a dash, a slash, a space.
		{"São Paulo - SP", []string{"br"}, []string{"São Paulo"}},
		{"Belo Horizonte/MG", []string{"br"}, []string{"Belo Horizonte"}},
		{"Rio de Janeiro, RJ", []string{"br"}, []string{"Rio de Janeiro"}},
		{"Campinas SP", []string{"br"}, []string{"Campinas"}},
		{"Localização não informada, SP, Brasil", []string{"br"}, nil},
		// The state names state Brazil and no city.
		{"Ijuí, Rio Grande do Sul, BR", []string{"br"}, []string{"Ijuí"}},
		{"Minas Gerais", []string{"br"}, nil},
		{"Todo Brasil", []string{"br"}, nil},
		{"Juazeiro do Norte, Ceará", []string{"br"}, []string{"Juazeiro do Norte"}},

		// A shared code keeps its other reading where the line says so.
		{"Ciudad de México, DF", []string{"mx"}, []string{"Mexico City"}},
		{"San Juan, PR", []string{"pr"}, []string{"San Juan"}},
		{"Columbia, SC", []string{"us"}, []string{"Columbia"}},
		{"Columbia, SC; Curitiba, Brasil", []string{"br", "us"}, []string{"Columbia", "Curitiba"}},
		{"Curitiba, PR; United States", []string{"br", "us"}, []string{"Curitiba"}},
		{"Colombo, Sri Lanka; Colombo, PR", []string{"br", "lk"}, []string{"Colombo"}},
		// An Indian state code between an Indian city and the country is the state:
		// "BR" is Bihar.
		{"Patna, BR, in", []string{"in"}, []string{"Patna"}},
		{"Chennai, TN, IN", []string{"in"}, []string{"Chennai"}},

		// Porto is the Portuguese city, not an alias of Bordeaux.
		{"Porto, PT", []string{"pt"}, []string{"Porto"}},
		{"Porto, Porto District, pt", []string{"pt"}, []string{"Porto"}},
		// A district named after its capital, after a town, is the district.
		{"Matosinhos, Porto, PT", []string{"pt"}, []string{"Matosinhos"}},
		// An unspaced line is a list, and every entry in it a place.
		{"Remote,Spain,Valencia,Lebanon,Barcelona", []string{"es", "lb"}, []string{"Barcelona", "Valencia"}},
		// Each location of a list is read on its own.
		{"Matosinhos, Porto, PT; Porto, PT", []string{"pt"}, []string{"Matosinhos", "Porto"}},
		{"Carnaxide, Lisboa, pt", []string{"pt"}, []string{"Carnaxide"}},
		{"Amadora, Lisbon, pt", []string{"pt"}, []string{"Amadora"}},
		{"Ovar, Aveiro, pt", []string{"pt"}, []string{"Ovar"}},
		{"Guimarães, Braga, pt", []string{"pt"}, []string{"Guimarães"}},
		{"Portimão, Faro, pt", []string{"pt"}, []string{"Portimão"}},
		{"Lisboa, Lisboa, PT", []string{"pt"}, []string{"Lisbon"}},
		{"Lisbon, PT; Porto, PT; Braga, PT", []string{"pt"}, []string{"Braga", "Lisbon", "Porto"}},
		// The islands state Portugal and no city.
		{"Funchal, Madeira, PT", []string{"pt"}, []string{"Funchal"}},
		{"Madeira", []string{"pt"}, nil},
		{"Açores", []string{"pt"}, nil},
	}
	for _, tt := range tests {
		got := Parse(tt.location)
		if !reflect.DeepEqual(got.Countries, tt.countries) || !reflect.DeepEqual(got.Cities, tt.cities) {
			t.Errorf("Parse(%q) = countries %v cities %v, want %v %v",
				tt.location, got.Countries, got.Cities, tt.countries, tt.cities)
		}
	}
}
