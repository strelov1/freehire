package location

import (
	"reflect"
	"testing"
)

// TestParseSubdivisionCountryCodeCollision covers the ~16 US/Canada subdivision
// postal codes that are also a curated country's ISO code ("il" Illinois/Israel,
// "la" Louisiana/Laos, "pa" Pennsylvania/Panama, …). The subdivision reading must
// only win when the preceding city is a recognized US/CA place; otherwise the bare
// country code wins, so a foreign city sharing a code with a US state does not get
// mislabeled as that state.
func TestParseSubdivisionCountryCodeCollision(t *testing.T) {
	tests := []struct {
		name     string
		location string
		want     Geo
	}{
		{
			name:     "Tel Aviv, IL is Israel, not Illinois",
			location: "Tel Aviv, IL",
			want:     Geo{Countries: []string{"il"}, Regions: []string{"mena"}, Cities: []string{"Tel Aviv"}, Metros: []string{"Tel Aviv (IL)"}},
		},
		{
			name:     "Vientiane, LA is Laos, not Louisiana",
			location: "Vientiane, LA",
			want:     Geo{Countries: []string{"la"}, Regions: []string{"apac"}, Cities: []string{"Vientiane"}, Metros: []string{"Vientiane (LA)"}},
		},
		{
			name:     "Panama City, PA is Panama, not Pennsylvania",
			location: "Panama City, PA",
			want:     Geo{Countries: []string{"pa"}, Regions: []string{"latam"}, Cities: []string{"Panama City"}, Metros: []string{"Panama City (PA)"}},
		},
		{
			// Regression guard: a genuine US city still wins its colliding state code
			// when the preceding token corroborates it.
			name:     "Chicago, IL stays Illinois, not Israel",
			location: "Chicago, IL",
			want:     Geo{Countries: []string{"us"}, Regions: []string{"north_america"}, Cities: []string{"Chicago"}, Metros: []string{"Chicago (US)"}},
		},
		{
			// Same collision, dash-delimited instead of comma-delimited: the lead
			// segment's context is the resolved TAIL, not a preceding comma-token.
			name:     "IL-Cupertino is California, not Israel",
			location: "IL-Cupertino",
			want:     Geo{Countries: []string{"us"}, Regions: []string{"north_america"}},
		},
		{
			name:     "Carmel, IN stays Indiana, not India",
			location: "Carmel, IN",
			want:     Geo{Countries: []string{"us"}, Regions: []string{"north_america"}, Cities: []string{"Carmel"}, Metros: []string{"Indianapolis (US)"}},
		},
		{
			name:     "Brownsburg, IN stays Indiana, not India",
			location: "Brownsburg, IN",
			want:     Geo{Countries: []string{"us"}, Regions: []string{"north_america"}, Cities: []string{"Brownsburg"}, Metros: []string{"Indianapolis (US)"}},
		},
		{
			name:     "Auburn, IN, US stays Indiana, not India",
			location: "Auburn, IN, US",
			want:     Geo{Countries: []string{"us"}, Regions: []string{"north_america"}, Cities: []string{"Auburn"}},
		},
		{
			name:     "Wilmington, DE stays Delaware, not Germany",
			location: "Wilmington, DE",
			want:     Geo{Countries: []string{"us"}, Regions: []string{"north_america"}, Cities: []string{"Wilmington"}, Metros: []string{"Philadelphia (US)"}},
		},
		{
			name:     "Boise, ID stays Idaho, not Indonesia",
			location: "Boise, ID",
			want:     Geo{Countries: []string{"us"}, Regions: []string{"north_america"}, Cities: []string{"Boise"}, Metros: []string{"Boise (US)"}},
		},
		{
			name:     "Bangalore, IN stays India, not Indiana",
			location: "Bangalore, IN",
			want:     Geo{Countries: []string{"in"}, Regions: []string{"apac"}, Cities: []string{"Bengaluru"}, Metros: []string{"Bengaluru (IN)"}},
		},
		{
			name:     "Mumbai, MH, IN stays India, not Indiana",
			location: "Mumbai, MH, IN",
			want:     Geo{Countries: []string{"in"}, Regions: []string{"apac"}, Cities: []string{"Mumbai"}, Metros: []string{"Mumbai (IN)"}},
		},
		{
			name:     "Berlin, DE stays Germany, not Delaware",
			location: "Berlin, DE",
			want:     Geo{Countries: []string{"de"}, Regions: []string{"eu"}, Cities: []string{"Berlin"}, Metros: []string{"Berlin (DE)"}},
		},
		{
			name:     "Jakarta, ID stays Indonesia, not Idaho",
			location: "Jakarta, ID",
			want:     Geo{Countries: []string{"id"}, Regions: []string{"apac"}, Cities: []string{"Jakarta"}, Metros: []string{"Jakarta (ID)"}},
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

// TestParseStateFirstSubdivisions covers the colliding codes whose US-state reading
// dominates this catalogue (measured 2026-10-21 by cmd/report-geo-gaps: of the open
// postings stored under Morocco for a trailing ", MA", the model read 168 of 227 as
// US; Moldova/MD 131 of 132; Panama/PA 227 of 239). For these, a city the
// dictionary does not place in the colliding COUNTRY is enough to read the state —
// "Cambridge, MA" is Massachusetts even though GeoNames calls Cambridge British.
// The country reading still wins when the city is that country's own.
func TestParseStateFirstSubdivisions(t *testing.T) {
	tests := []struct {
		location string
		want     []string
	}{
		// The state, now.
		{"Cambridge, MA", []string{"us"}},
		{"Birmingham, AL", []string{"us"}},
		{"Montgomery County, PA", []string{"us"}},
		{"Annapolis Junction, MD", []string{"us"}},
		{"Fort Meade, MD", []string{"us"}},
		{"Oak Brook, IL", []string{"us"}},
		{"Lone Tree, MN", []string{"us"}},
		{"Statewide, LA", []string{"us"}},
		{"Slu Saint Louis, MO", []string{"us"}},
		{"Portland, ME", []string{"us"}},
		{"Brighton MA", []string{"us"}},
		// The country, still: the city is that country's own.
		{"Tel Aviv, IL", []string{"il"}},
		{"Haifa, IL", []string{"il"}},
		{"Casablanca, MA", []string{"ma"}},
		{"Rabat, MA", []string{"ma"}},
		{"Tirana, AL", []string{"al"}},
		{"Chisinau, MD", []string{"md"}},
		{"Panama City, PA", []string{"pa"}},
		{"Ulaanbaatar, MN", []string{"mn"}},
		{"Podgorica, ME", []string{"me"}},
		{"Vientiane, LA", []string{"la"}},
		// A bare code has no city to judge by and keeps its old reading.
		{"MA", []string{"ma"}},
		{"IL", []string{"il"}},
		// Codes whose COUNTRY reading dominates are untouched: an unknown city
		// beside them still reads as the country.
		{"Unknownville, DE", []string{"de"}},
		{"Unknownville, IN", []string{"in"}},
		{"Unknownville, CO", []string{"co"}},
	}
	for _, tt := range tests {
		t.Run(tt.location, func(t *testing.T) {
			got := Parse(tt.location).Countries
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Parse(%q).Countries = %v, want %v", tt.location, got, tt.want)
			}
		})
	}
}

// TestStateFirstSubdivisionsAreColliding guards the set against dead entries: a
// code that is not a colliding subdivision never reaches the check that reads it.
func TestStateFirstSubdivisionsAreColliding(t *testing.T) {
	for code := range stateFirstSubdivisions {
		if _, ok := collidingSubdivisions[code]; !ok {
			t.Errorf("stateFirstSubdivisions[%q] is not a colliding subdivision code", code)
		}
	}
}

// TestParseAustralianStateCodes covers the "City STATE" form SEEK writes ("Perth WA",
// "Sydney NSW"): one token with no comma, whose trailing code used to be read against the
// US/Canada table alone, so WA became Washington and NT the Northwest Territories.
func TestParseAustralianStateCodes(t *testing.T) {
	tests := []struct {
		location string
		want     []string
	}{
		// Codes only Australia uses resolve after any place name.
		{"Sydney NSW", []string{"au"}},
		{"Melbourne VIC", []string{"au"}},
		{"Brisbane QLD", []string{"au"}},
		{"Hobart TAS", []string{"au"}},
		{"Bondi Junction NSW", []string{"au"}},
		// Codes another country shares resolve to Australia only beside an Australian city.
		{"Perth WA", []string{"au"}},
		{"Perth, WA", []string{"au"}},
		{"Osborne Park, Perth WA", []string{"au"}},
		{"Adelaide SA", []string{"au"}},
		{"Darwin NT", []string{"au"}},
		{"Darwin, NT", []string{"au"}},
		{"Canberra ACT", []string{"au"}},
		// The other readings stay where no Australian city says otherwise.
		{"Seattle, WA", []string{"us"}},
		{"Spokane WA", []string{"us"}},
		{"Yellowknife, NT", []string{"ca"}},
	}
	for _, tt := range tests {
		t.Run(tt.location, func(t *testing.T) {
			got := Parse(tt.location).Countries
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Parse(%q).Countries = %v, want %v", tt.location, got, tt.want)
			}
		})
	}
}

// TestParseAustralianStateKeepsTheCity guards what cutting the code off is for: the place
// before it still resolves as a city, with its metro, instead of the whole "perth wa" token
// matching nothing in the city dictionary.
func TestParseAustralianStateKeepsTheCity(t *testing.T) {
	want := Geo{Countries: []string{"au"}, Regions: []string{"apac"}, Cities: []string{"Perth"}, Metros: []string{"Perth (AU)"}}
	for _, location := range []string{"Perth WA", "Perth, WA"} {
		if got := Parse(location); !reflect.DeepEqual(got, want) {
			t.Errorf("Parse(%q) = %+v, want %+v", location, got, want)
		}
	}
	// The suburb is a small place, named once Perth has placed the line in Australia.
	want.Cities = []string{"Osborne Park", "Perth"}
	if got := Parse("Osborne Park, Perth WA"); !reflect.DeepEqual(got, want) {
		t.Errorf("Parse(%q) = %+v, want %+v", "Osborne Park, Perth WA", got, want)
	}
}
