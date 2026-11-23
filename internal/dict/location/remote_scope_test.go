package location

import (
	"slices"
	"testing"
)

func TestWorldwideRemoteFromDescription(t *testing.T) {
	const policy = "We accommodate 100% remote work, with teammates living around the globe."
	tests := []struct {
		name, description string
		want              bool
	}{
		{"worldwide remote policy", "<p>" + policy + "</p>", true},
		{"role explicitly open worldwide", "This role is fully remote and open worldwide.", true},
		{"team description only", "We are a fully remote team with colleagues around the globe.", false},
		{"explicit residence requirement", policy + " Applicants are required to reside in Germany.", false},
		{"optional residence", policy + " Applicants are not required to reside in Germany.", true},
		{"country restriction", policy + " You must be based in Canada.", false},
		{"country exception", "This role is remote worldwide, except in Germany.", false},
		{"temporary worldwide perk", "This role is fully remote from anywhere in the world for up to four weeks per year.", false},
		{"work authorization", policy + " You must be authorized to work in the United States.", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := WorldwideRemoteFromDescription(tt.description); got != tt.want {
				t.Errorf("WorldwideRemoteFromDescription() = %t, want %t", got, tt.want)
			}
		})
	}
}

func TestExcludedCountriesFromDescription(t *testing.T) {
	tests := []struct {
		name, description string
		want              []string
	}{
		{"applicant refusal", "<p>We are unable to consider applications from candidates based in France, Italy, or Germany for this role.</p>", []string{"de", "fr", "it"}},
		{"worldwide exception", "This role is remote worldwide, except in Germany.", []string{"de"}},
		{"contracted wording", "We can’t consider candidates located in the UK.", []string{"gb"}},
		{"unrelated earlier denial", "We do not sponsor visas; we cannot accept applications from candidates based in Germany; see the location details.", []string{"de"}},
		{"negated refusal", "It is not true that we cannot accept applications from candidates based in Germany.", nil},
		{"city and country", "We cannot accept applicants based in Berlin, Germany.", nil},
		{"no residence requirement", "Applicants are not required to be based in France.", nil},
		{"ordinary hiring location", "We're hiring in France or Germany.", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ExcludedCountriesFromDescription(tt.description); !slices.Equal(got, tt.want) {
				t.Errorf("ExcludedCountriesFromDescription() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRegionScopeFromDescription_RemoteLocationLabels(t *testing.T) {
	for _, desc := range []string{
		"<p><strong>Headquarters:</strong> United States - Remote, EMEA - Remote</p>",
		"Headquarters: United States - Remote, EMEA - Remote Please note that this role requires English.",
	} {
		countries, regions := RegionScopeFromDescription(desc)
		if !slices.Equal(countries, []string{"us"}) || !slices.Equal(regions, []string{"africa", "eu", "mena", "north_america", "uk"}) {
			t.Errorf("RegionScopeFromDescription(%q) = %v / %v, want US and EMEA", desc, countries, regions)
		}
	}
	countries, regions := RegionScopeFromDescription("<p>Location: United States - Remote</p><p>Location: Canada - Remote</p>")
	if !slices.Equal(countries, []string{"ca", "us"}) || !slices.Equal(regions, []string{"north_america"}) {
		t.Errorf("two location labels = %v / %v, want US and Canada", countries, regions)
	}
	if countries, regions := RegionScopeFromDescription("Headquarters: United States - Remote"); len(countries)+len(regions) != 0 {
		t.Errorf("single headquarters = %v / %v, want no role scope", countries, regions)
	}
}

func TestIsRemoteOfficeList(t *testing.T) {
	for _, tt := range []struct {
		line string
		want bool
	}{
		{"Remote / Boston / New York", true},
		{"Remote / Canada / Boston / New York", false},
		{"Remote / Boston", false},
		{"Berlin / Munich", false},
	} {
		t.Run(tt.line, func(t *testing.T) {
			if got := IsRemoteOfficeList(tt.line, Parse(tt.line).Cities); got != tt.want {
				t.Errorf("IsRemoteOfficeList() = %t, want %t", got, tt.want)
			}
		})
	}
}

func TestWithoutExcludedGeography(t *testing.T) {
	countries := []string{"ca", "de"}
	regions := []string{"eu", "global", "north_america"}
	wantCountries := []string{"ca"}
	for _, tt := range []struct {
		line string
		want []string
	}{
		{"Remote / Canada / Germany", []string{"north_america"}},
		{"Remote / Europe / Canada / Germany", []string{"eu", "north_america"}},
	} {
		t.Run(tt.line, func(t *testing.T) {
			gotCountries, gotRegions := WithoutExcludedGeography(countries, regions, []string{"de"}, tt.line, "")
			if !slices.Equal(gotCountries, wantCountries) || !slices.Equal(gotRegions, tt.want) {
				t.Errorf("WithoutExcludedGeography() = %v / %v, want %v / %v", gotCountries, gotRegions, wantCountries, tt.want)
			}
		})
	}
}

func TestWithoutExcludedPlaces(t *testing.T) {
	cities := []string{"Berlin", "Toronto", "Uncatalogued Place"}
	metros := []string{"Berlin (DE)", "Toronto (CA)"}
	gotCities, gotMetros := WithoutExcludedPlaces(cities, metros, []string{"de", "ca"}, []string{"de"})
	if !slices.Equal(gotCities, []string{"Toronto", "Uncatalogued Place"}) ||
		!slices.Equal(gotMetros, []string{"Toronto (CA)"}) {
		t.Errorf("places = %v / %v, want Toronto and the unresolved city", gotCities, gotMetros)
	}
	gotCities, gotMetros = WithoutExcludedPlaces(cities, metros, nil, nil)
	if !slices.Equal(gotCities, []string{"Berlin", "Toronto", "Uncatalogued Place"}) ||
		!slices.Equal(gotMetros, []string{"Berlin (DE)", "Toronto (CA)"}) {
		t.Errorf("without exclusions, places changed to %v / %v", gotCities, gotMetros)
	}
}
