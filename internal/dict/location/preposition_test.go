package location

import (
	"reflect"
	"testing"
)

// TestPlaceAfterPreposition pins that a country or region named after a preposition is
// read: "Remote within Canada" is Canada, "Remote in APAC" is APAC. The preposition used to
// stay glued to the name, so the token resolved to nothing and the country was lost — an
// API customer filtering countries=ca never saw "…, or Remote within Canada or United
// States". Only a name the dictionary knows is read; what precedes the preposition must be
// a lead-in ("anywhere", "based", "work from anywhere"), never a town of its own.
func TestPlaceAfterPreposition(t *testing.T) {
	tests := []struct {
		location  string
		countries []string
		regions   []string
	}{
		{"Remote within Canada", []string{"ca"}, []string{"north_america"}},
		{"within Canada", []string{"ca"}, []string{"north_america"}},
		{"Remote (within Canada)", []string{"ca"}, []string{"north_america"}},
		{"Remote in Canada or US", []string{"ca", "us"}, []string{"north_america"}},
		{"Remote within Canada or United States", []string{"ca", "us"}, []string{"north_america"}},
		{"San Francisco, CA, New York, NY, Portland, OR, or Remote within Canada or United States", []string{"ca", "us"}, []string{"north_america"}},
		{"Remote in the US", []string{"us"}, []string{"north_america"}},
		{"Remote within the United States", []string{"us"}, []string{"north_america"}},
		{"Based in Germany", []string{"de"}, []string{"eu"}},
		{"Remote from Spain", []string{"es"}, []string{"eu"}},
		{"Remote, Anywhere in the US", []string{"us"}, []string{"north_america"}},
		{"Work from anywhere in Canada", []string{"ca"}, []string{"north_america"}},
		{"Remote in India", []string{"in"}, []string{"apac"}},
		{"Anywhere in Europe", nil, []string{"eu"}},
		{"Remote within Europe", nil, []string{"eu"}},
		{"Remote in APAC", nil, []string{"apac"}},
		{"Remote across LATAM", nil, []string{"latam"}},
		// Guards: a known place stays as it was, an unknown name proves nothing, and a
		// word before the preposition that is not a lead-in keeps the token whole.
		{"Remote - Canada", []string{"ca"}, []string{"north_america"}},
		{"Canada or United States", []string{"ca", "us"}, []string{"north_america"}},
		{"Anywhere in the world", nil, nil},
		{"Remote in Zzyzx", nil, nil},
		// EMEA is no region of the vocabulary.
		{"Remote in EMEA", nil, nil},
		{"Engineering in Canada", nil, nil},
		// A city of the main table reads like any other ("Recife" alone is Brazil); a state
		// name stays with resolveSubdivision and names no town.
		{"Remote in Recife", []string{"br"}, []string{"latam"}},
		{"Anywhere in Oregon", []string{"us"}, []string{"north_america"}},
		// A bare "IN" opening the line is also Indiana's code: "IN Lebanon" and "IN Angola"
		// are Indiana towns, so a name the US has a town of is not read after it. A work
		// mode before the preposition settles it.
		{"IN Lebanon", nil, nil},
		{"IN Angola - 127206", nil, nil},
		{"Remote in Lebanon", []string{"lb"}, []string{"mena"}},
		{"IN Hyderabad", []string{"in"}, []string{"apac"}},
	}
	for _, tt := range tests {
		g := Parse(tt.location)
		if !reflect.DeepEqual(g.Countries, tt.countries) || !reflect.DeepEqual(g.Regions, tt.regions) {
			t.Errorf("Parse(%q) = %v %v, want %v %v", tt.location, g.Countries, g.Regions, tt.countries, tt.regions)
		}
	}
}

// TestPlaceAfterPrepositionNamesNoWrongCity pins the two ways cutting a preposition could
// name a city the line does not: an alternate GeoNames name ("market" is an alias of
// Marquette) and an Indiana town after a bare "IN" that shares a foreign city's name.
func TestPlaceAfterPrepositionNamesNoWrongCity(t *testing.T) {
	for loc, want := range map[string][]string{
		"Remote in Market - Atlanta, Georgia - Atlanta, GA 30303": {"Atlanta"},
		"IN Munster - 127103":  nil,
		"Hybrid in Boston, MA": {"Boston"},
		// The article is part of this name: not Singapore's Woodlands.
		"Remote in The Woodlands, TX": {"The Woodlands"},
	} {
		if got := Parse(loc).Cities; !reflect.DeepEqual(got, want) {
			t.Errorf("Parse(%q).Cities = %v, want %v", loc, got, want)
		}
	}
}
