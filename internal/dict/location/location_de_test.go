package location

import (
	"slices"
	"testing"
)

// German-market location lines. Every input is a real location field from the German
// ATS crawl (coveto, rexx, dvinci, xing, concludis, onlyfy, kenjo; 2026-10-25) unless
// the case says otherwise.
func TestParseGermanLocations(t *testing.T) {
	tests := []struct {
		name      string
		location  string
		countries []string
		cities    []string
	}{
		// A town below the main city table's 15,000 floor, placed by the Land beside it.
		{"small town with a Land", "Billerbeck, Nordrhein-Westfalen", []string{"de"}, []string{"Billerbeck"}},
		{"small town with a Land and code", "Herrieden, Bayern, DE", []string{"de"}, []string{"Herrieden"}},
		{"small town with the country", "Türkheim, Deutschland", []string{"de"}, []string{"Türkheim"}},
		{"small Austrian town", "Vöcklamarkt, Oberösterreich", []string{"at"}, []string{"Vöcklamarkt"}},

		// A small place never states a country on its own: the same name is a village in
		// more than one country, and this table is evidence of a NAME, not of a place.
		{"small town alone states nothing", "Hasbergen", nil, nil},
		// Nor does a country another CITY implies: Recife places the line in Brazil only by
		// the long-tail fallback, which a small place may not lean on (synthetic).
		{"small town beside an inferring city", "Recife, São João do Triunfo", []string{"br"}, []string{"Recife"}},
		// French "Canton" is a US city, not an area word (synthetic).
		{"Canton MA stays American", "Canton MA", []string{"us"}, nil},

		// Austrian Länder and Swiss cantons.
		{"Austrian Land alone", "Oberösterreich", []string{"at"}, nil},
		{"Tirol", "Tirol", []string{"at"}, nil},
		{"Vorarlberg", "Vorarlberg", []string{"at"}, nil},
		{"Steiermark with a town", "Leoben, Steiermark, AT", []string{"at"}, []string{"Leoben"}},
		{"Swiss canton", "Aargau", []string{"ch"}, nil},

		// "Großraum X" is X's area, "Region X" likewise.
		{"Großraum", "Großraum Hannover, Niedersachsen", []string{"de"}, []string{"Hannover"}},
		{"Großraum with a long city", "Großraum Frankfurt am Main, Hessen", []string{"de"}, []string{"Frankfurt"}},
		{"Region (synthetic)", "Region Stuttgart", []string{"de"}, []string{"Stuttgart"}},

		// A postal code before the town, German, Swiss or Austrian style.
		{"German postal code", "85462 Eitting, Deutschland", []string{"de"}, []string{"Eitting"}},
		{"postal code before a large city (synthetic)", "80331 München", []string{"de"}, []string{"Munich"}},
		{"Swiss postal code with prefix", "CH-3400 Burgdorf, CH", []string{"ch"}, []string{"Burgdorf"}},

		// "X am Main", "X in der Oberpfalz", "X bei Y": the town is X.
		{"am river", "Rottenburg am Neckar, Baden-Württemberg", []string{"de"}, []string{"Rottenburg"}},
		{"in der region", "Weiden in der Oberpfalz, Bayern", []string{"de"}, []string{"Weiden"}},
		{"bei a neighbour", "Ergolding bei Landshut, Germany", []string{"de"}, []string{"Ergolding"}},

		// A district of a large city, written "City-District".
		{"Köln district", "Köln-Niehl, Nordrhein-Westfalen", []string{"de"}, []string{"Cologne"}},
		{"Karlsruhe district", "Karlsruhe-Durlach, Baden-Württemberg", []string{"de"}, []string{"Karlsruhe"}},
		{"hyphenated city before its district", "Baden-Baden-Oos, Baden-Württemberg", []string{"de"}, []string{"Baden-Baden"}},

		// A parenthesised qualifier is never a village of its own name.
		{"river qualifier", "Schönebeck (Elbe), Sachsen-Anhalt", []string{"de"}, []string{"Schönebeck"}},

		{"river after a slash", "Fürstenwalde/Spree, Brandenburg", []string{"de"}, []string{"Fürstenwalde"}},
		{"lake after a slash", "Lunz/See, Niederösterreich, AT", []string{"at"}, nil},

		// One city, one facet value, however the line spells it.
		{"Frankfurt am Main is Frankfurt", "Frankfurt am Main, Hessen", []string{"de"}, []string{"Frankfurt"}},
		{"Koeln is Cologne", "Koeln", []string{"de"}, []string{"Cologne"}},
		{"Zürich is Zurich", "Zürich, CH", []string{"ch"}, []string{"Zurich"}},
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

// A renamed city keeps its metro: the metro table is keyed by the GeoNames name, and a
// display rename that forgot it would quietly drop Frankfurt from its own metro.
func TestRenamedCityKeepsItsMetro(t *testing.T) {
	for _, loc := range []string{"Frankfurt, Germany", "Frankfurt am Main, Hessen", "Köln, Deutschland", "Zürich, Schweiz"} {
		if g := Parse(loc); len(g.Metros) != 1 {
			t.Errorf("Parse(%q).Metros = %v, want one metro", loc, g.Metros)
		}
	}
}
