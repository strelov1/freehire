package location

import (
	"reflect"
	"testing"
)

// TestParseCityFacetExpansion covers the GeoNames-backed city dictionary: a resolved
// city emits both its canonical facet value and its country/region, a genuinely
// long-tail city works, GeoNames alternate-name noise is rejected, stoplisted markers
// never become cities, and a curated override spelling wins.
func TestParseCityFacetExpansion(t *testing.T) {
	tests := []struct {
		name     string
		location string
		want     Geo
	}{
		{
			// The motivating case: a city that used to resolve a country but no city.
			name:     "curated city emits facet + country + region",
			location: "Florianópolis",
			want:     Geo{Countries: []string{"br"}, Regions: []string{"latam"}, Cities: []string{"Florianópolis"}, Metros: []string{"Florianópolis (BR)"}},
		},
		{
			name:     "city with embedded country still resolves the facet",
			location: "Florianópolis, Brazil",
			want:     Geo{Countries: []string{"br"}, Regions: []string{"latam"}, Cities: []string{"Florianópolis"}, Metros: []string{"Florianópolis (BR)"}},
		},
		{
			// A long-tail city absent from the hand-curated maps. "Recife" is claimed by
			// exactly one country in the dataset, so it now states br/latam on its own —
			// no country token needed. Nothing is guessed: a name two countries share
			// still states nothing (see the contested cases below).
			name:     "unambiguous long-tail city states its own country",
			location: "Recife",
			want:     Geo{Countries: []string{"br"}, Regions: []string{"latam"}, Cities: []string{"Recife"}, Metros: []string{"Recife (BR)"}},
		},
		{
			// With an explicit country token the geography resolves deterministically too.
			name:     "long-tail city with a country token resolves geography",
			location: "Recife, Brazil",
			want:     Geo{Countries: []string{"br"}, Regions: []string{"latam"}, Cities: []string{"Recife"}, Metros: []string{"Recife (BR)"}},
		},
		{
			// A name two countries share states NO country. This is the guard on the
			// unambiguous-city rule: without it "most populous wins" would file a
			// Birmingham (UK) role under the US, which is worse than filing it nowhere.
			name:     "contested city name states no country",
			location: "Birmingham",
			want:     Geo{Cities: []string{"Birmingham"}},
		},
		{
			// Canada and the US both have a Burlington; the same silence applies. (A
			// contested name the CURATED maps already pin — "San Jose", "Valencia" —
			// still resolves from them: this rule only fills what they left empty.)
			name:     "contested city name states no country, second case",
			location: "Burlington",
			want:     Geo{Cities: []string{"Burlington"}},
		},
		{
			// A country token resolves a contested name — the token is authoritative,
			// the city merely agrees with it.
			name:     "contested city with a country token resolves",
			location: "Birmingham, UK",
			want:     Geo{Countries: []string{"gb"}, Regions: []string{"uk"}, Cities: []string{"Birmingham"}, Metros: []string{"Birmingham (GB)"}},
		},
		{
			// The city is the LAST word, never a contributing one. "Anna" is a town in
			// Russia; the state and country in the same line are what place this job,
			// and the city must not add a second country beside them. Measured against
			// production, this rule is what took the change from 9 wrong countries per
			// 2000 postings down to 3.
			name:     "stated country wins over a long-tail city elsewhere in the line",
			location: "Anna, Illinois, United States",
			want:     Geo{Countries: []string{"us"}, Regions: []string{"north_america"}, Cities: []string{"Anna"}},
		},
		{
			// Same rule with the signal on the other side of the separator: "Crossroads"
			// is an alias of Woonsocket, RI, but London is what this line is about, so
			// only gb lands — and Woonsocket, a US place in a line that states Britain,
			// is no city of it either (alias_place.go).
			name:     "stated country wins over a long-tail city before it",
			location: "Crossroads - London",
			want:     Geo{Countries: []string{"gb"}, Regions: []string{"uk"}, Cities: []string{"London"}, Metros: []string{"London (GB)"}},
		},
		{
			// With no other signal in the line, the city is all there is — and it speaks.
			// This is the case the whole rule exists for: ~39% of geographically
			// unpinned production postings name a real place and nothing else.
			name:     "lone long-tail city speaks when nothing else does",
			location: "Colorado Springs",
			want:     Geo{Countries: []string{"us"}, Regions: []string{"north_america"}, Cities: []string{"Colorado Springs"}, Metros: []string{"Colorado Springs (US)"}},
		},
		{
			// Two individually-unambiguous cities in different countries make the LINE
			// ambiguous. Taking the first would make the answer depend on word order,
			// so they cancel and state nothing. Both orders are asserted because
			// order-dependence is exactly the bug being guarded against.
			name:     "disagreeing long-tail cities state no country",
			location: "Recife, Benidorm",
			want:     Geo{Cities: []string{"Benidorm", "Recife"}},
		},
		{
			name:     "disagreeing long-tail cities state no country, reversed",
			location: "Benidorm, Recife",
			want:     Geo{Cities: []string{"Benidorm", "Recife"}},
		},
		{
			// Agreement is about the country, not the name: two different cities in the
			// same country still speak with one voice.
			name:     "agreeing long-tail cities still state their shared country",
			location: "Colorado Springs, Eden Prairie",
			want:     Geo{Countries: []string{"us"}, Regions: []string{"north_america"}, Cities: []string{"Colorado Springs", "Eden Prairie"}, Metros: []string{"Colorado Springs (US)", "Minneapolis (US)"}},
		},
		{
			// "usa" appears among a Japanese city's GeoNames alternate names; the
			// country-agreement guard must reject that city while keeping the country.
			name:     "GeoNames alternate-name noise is rejected",
			location: "USA",
			want:     Geo{Countries: []string{"us"}, Regions: []string{"north_america"}},
		},
		{
			// A stoplisted open-anywhere marker is never a city; it stays a region signal.
			name:     "stoplisted marker never becomes a city",
			location: "Worldwide",
			want:     Geo{Regions: []string{"global"}, WorkMode: "remote"},
		},
		{
			// The curated override pins the English facet spelling over GeoNames "Köln", and
			// the metro table, keyed by the GeoNames name, still finds the city.
			name:     "curated override spelling wins",
			location: "Köln",
			want:     Geo{Countries: []string{"de"}, Regions: []string{"eu"}, Cities: []string{"Cologne"}, Metros: []string{"Köln (DE)"}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Parse(tt.location)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Parse(%q) = %+v, want %+v", tt.location, got, tt.want)
			}
		})
	}
}
