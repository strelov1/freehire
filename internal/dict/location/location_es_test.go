package location

import (
	"reflect"
	"testing"
)

// TestSpanishAndLatinAmericanLines pins the Spanish-language location lines measured over
// the Spanish and Latin-American crawl (infojobs, factorial, teamtailor, occ, pandapelatam,
// getonbrd, torre, hiringroom, whatjobs) on 2026-10-27.
func TestSpanishAndLatinAmericanLines(t *testing.T) {
	tests := []struct {
		location  string
		countries []string
		cities    []string
	}{
		// The communities, islands and Basque provinces state the country and no city.
		{"Comunidad de Madrid", []string{"es"}, nil},
		{"Cataluña", []string{"es"}, nil},
		{"Catalunya", []string{"es"}, nil},
		{"País Vasco", []string{"es"}, nil},
		{"Euskadi", []string{"es"}, nil},
		{"Comunitat Valenciana", []string{"es"}, nil},
		{"Illes Balears", []string{"es"}, nil},
		{"Islas Baleares", []string{"es"}, nil},
		{"Bizkaia", []string{"es"}, nil},
		{"Andalucía", []string{"es"}, nil}, // was a Colombian town
		{"Tenerife", []string{"es"}, nil},  // was Santa Cruz de Tenerife
		{"Los Cristianos, Tenerife, España", []string{"es"}, nil},
		// The bilingual names and the curated cities.
		{"Valencia", []string{"es"}, []string{"Valencia"}},
		{"Zaragoza", []string{"es"}, []string{"Zaragoza"}},
		{"Madrid capital", []string{"es"}, []string{"Madrid"}},
		{"La Coruña", []string{"es"}, []string{"A Coruña"}},
		{"Donostia", []string{"es"}, []string{"Donostia / San Sebastián"}},
		{"Lleida/Lérida", []string{"es"}, []string{"Lleida"}},
		{"Canovelles, Barcelona, Espanya", []string{"es"}, []string{"Canovelles"}},
		// A province named after its capital, after a town, is the province.
		{"Pozuelo de Alarcón, Madrid, España", []string{"es"}, []string{"Pozuelo de Alarcón"}},
		{"Oliva, Valencia, España", []string{"es"}, []string{"Oliva"}},
		{"Madrid, Madrid, España", []string{"es"}, []string{"Madrid"}},
		{"ALICANTE, ESPAÑA/ALICANTE/VALENCIA, España", []string{"es"}, []string{"Alicante"}},
		// Mexico: the states, and the state named after its capital in the last slot.
		{"Mérida, Yucatán", []string{"mx"}, []string{"Mérida"}},
		{"Toluca, Estado de México", []string{"mx"}, []string{"Toluca"}},
		{"Guadalajara, Jalisco", []string{"mx"}, []string{"Guadalajara"}},
		{"Monterrey, Nuevo León", []string{"mx"}, []string{"Monterrey"}},
		{"León, Guanajuato", []string{"mx"}, []string{"León de los Aldama"}},
		{"Celaya, Guanajuato", []string{"mx"}, []string{"Celaya"}},
		{"Juárez, Chihuahua", []string{"mx"}, []string{"Ciudad Juárez"}},
		{"Distrito Federal, Mexico", []string{"mx"}, []string{"Mexico City"}},
		{"Distrito Federal, Brasil", []string{"br"}, nil}, // Brasília's district
		{"Ferrol, La Coruña, España", []string{"es"}, []string{"Ferrol"}},
		{"Baja California", []string{"mx"}, nil}, // was read as California
		{"Remote, Querétaro, Qro., México", []string{"mx"}, []string{"Santiago de Querétaro"}},
		// A country name is not its capital.
		{"Monterrey, Mexico", []string{"mx"}, []string{"Monterrey"}},
		{"David, Panama", []string{"pa"}, []string{"David"}},
		{"Mexico", []string{"mx"}, nil},
		// Colombia, Argentina, Chile, and the Spanish exonyms.
		{"Bogotá, D.C.", []string{"co"}, []string{"Bogotá"}},
		{"Bogotá D.C., Colombia", []string{"co"}, []string{"Bogotá"}},
		{"Envigado, Antioquia, Colombia", []string{"co"}, []string{"Envigado"}},
		{"CABA", []string{"ar"}, nil},
		{"Región Metropolitana", []string{"cl"}, nil},
		{"Panamá", []string{"pa"}, nil},
		{"República Dominicana", []string{"do"}, nil},
		// A contested name whose city is named differently is not that city in another
		// country: León, Spain is not León de los Aldama.
		{"León, ES", []string{"es"}, nil},
		{"Annapolis, MD", []string{"us"}, nil}, // was Simão Dias, Brazil
		{"Canton, Ohio", []string{"us"}, nil},  // was Guangzhou
		{"Salem, MA", []string{"us"}, nil},     // was Jerusalem
		{"Córdoba, Spain", []string{"es"}, []string{"Córdoba"}},
	}
	for _, tt := range tests {
		g := Parse(tt.location)
		if !reflect.DeepEqual(g.Countries, tt.countries) || !reflect.DeepEqual(g.Cities, tt.cities) {
			t.Errorf("Parse(%q) = countries %v cities %v, want %v %v", tt.location, g.Countries, g.Cities, tt.countries, tt.cities)
		}
	}
}
