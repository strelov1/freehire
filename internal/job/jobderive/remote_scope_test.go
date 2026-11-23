package jobderive

import (
	"slices"
	"testing"
)

func TestDerive_WorldwideRemoteWithOfficeLocations(t *testing.T) {
	const offices = "Remote / Boston / Salt Lake City / San Francisco / New York"
	const worldwide = "We accommodate 100% remote work, with teammates living around the globe and paid in their local currency."
	tests := []struct {
		name, title, location, description, workMode string
		structuredCountries, countries, regions      []string
	}{
		{name: "worldwide arrangement overrides office list", location: offices,
			description: "<p>We accommodate <strong>100% remote work</strong>, with teammates living around the globe and paid in their local currency.</p>",
			regions:     []string{"global"}},
		{name: "role explicitly open worldwide", location: offices,
			description: "This role is fully remote and open worldwide.", regions: []string{"global"}},
		{name: "worldwide application scope", location: offices,
			description: "This is a fully remote position open to candidates anywhere in the world.", regions: []string{"global"}},
		{name: "restricted despite generic remote policy", location: offices,
			description: worldwide + " Applicants must be in Europe.",
			countries:   []string{"us"}, regions: []string{"north_america"}},
		{name: "distributed team boilerplate", location: offices,
			description: "We are a fully remote team with colleagues around the globe.",
			countries:   []string{"us"}, regions: []string{"north_america"}},
		{name: "temporary work from anywhere perk", location: offices,
			description: "You can work from anywhere in the world for up to four weeks per year.",
			countries:   []string{"us"}, regions: []string{"north_america"}},
		{name: "explicit home location restriction", location: offices,
			description: worldwide + " You must be based in the United States.",
			countries:   []string{"us"}, regions: []string{"north_america"}},
		{name: "explicit residence requirement", location: "Remote / Berlin / Munich",
			description: worldwide + " Applicants are required to reside in Germany.",
			countries:   []string{"de"}, regions: []string{"eu"}},
		{name: "optional residence does not restrict", location: "Remote / Berlin / Munich",
			description: worldwide + " Applicants are not required to reside in Germany.",
			regions:     []string{"global"}},
		{name: "country only restriction", location: offices,
			description: worldwide + " This position is open to US applicants only.",
			countries:   []string{"us"}, regions: []string{"north_america"}},
		{name: "country in location field", location: "Remote / United States", description: worldwide,
			countries: []string{"us"}, regions: []string{"north_america"}},
		{name: "title restriction", title: "Engineer [Remote-US]", location: offices, description: worldwide,
			countries: []string{"us"}, regions: []string{"north_america"}},
		{name: "onsite required", location: offices,
			description: worldwide + " For this role, you must work from our office in Boston.",
			countries:   []string{"us"}, regions: []string{"north_america"}},
		{name: "remote explicitly denied for the role", location: offices,
			description: worldwide + " This position is 100% on-site.",
			countries:   []string{"us"}, regions: []string{"north_america"}},
		{name: "fully onsite requirement in body", location: offices,
			description: worldwide + " This role is fully on-site.",
			countries:   []string{"us"}, regions: []string{"north_america"}},
		{name: "required days in office", location: offices,
			description: worldwide + " This position requires 3 days in office.",
			countries:   []string{"us"}, regions: []string{"north_america"}},
		{name: "remote only in a named country", location: offices,
			description: worldwide + " Remote in the United States only.",
			countries:   []string{"us"}, regions: []string{"north_america"}},
		{name: "hybrid required", location: offices, workMode: "hybrid", description: worldwide,
			countries: []string{"us"}, regions: []string{"north_america"}},
		{name: "structured country", location: offices, description: worldwide,
			structuredCountries: []string{"ca"}, countries: []string{"ca"}, regions: []string{"north_america"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Derive(Input{Title: tt.title, Location: tt.location, Description: tt.description,
				WorkMode: tt.workMode, Countries: tt.structuredCountries})
			if !slices.Equal(got.Countries, tt.countries) || !slices.Equal(got.Regions, tt.regions) {
				t.Fatalf("geography = %v / %v, want %v / %v", got.Countries, got.Regions, tt.countries, tt.regions)
			}
			if slices.Contains(tt.regions, "global") && (len(got.Cities) != 0 || len(got.Metros) != 0) {
				t.Errorf("worldwide role still has office facets: %v / %v", got.Cities, got.Metros)
			}
		})
	}
}

func TestDerive_ExcludedCountriesAreNotPositiveGeography(t *testing.T) {
	const exclusion = "We are unable to consider applications from candidates based in France, Italy, or Germany"
	cases := []struct {
		location, description string
	}{
		{"Anywhere in the World", "<p><strong>Headquarters:</strong> United States - Remote, EMEA - Remote</p>" +
			"<p><strong>Please note that " + exclusion + " for this role.</strong></p>"},
		{"Remote / Worldwide", "Headquarters: United States - Remote, EMEA - Remote Please note that " +
			exclusion + " for this role. Join a remote-first, global team."},
	}
	for _, tc := range cases {
		t.Run(tc.location, func(t *testing.T) {
			got := Derive(Input{Location: tc.location, Description: tc.description})
			wantRegions := []string{"africa", "eu", "mena", "north_america", "uk"}
			if !slices.Equal(got.Countries, []string{"us"}) || !slices.Equal(got.Regions, wantRegions) {
				t.Errorf("geography = %v / %v, want [us] / %v", got.Countries, got.Regions, wantRegions)
			}
		})
	}
	t.Run("an exclusion alone cannot become a positive facet", func(t *testing.T) {
		got := Derive(Input{Location: "Remote / Worldwide", Description: exclusion + ".", FallbackCountries: []string{"us"}})
		if len(got.Countries) != 0 || len(got.Regions) != 0 {
			t.Errorf("geography = %v / %v, want no positive geography", got.Countries, got.Regions)
		}
	})
	t.Run("a worldwide exception leaves the positive scope unspecified", func(t *testing.T) {
		got := Derive(Input{Location: "Remote / Worldwide", Description: "This role is remote worldwide, except in Germany."})
		if len(got.Countries) != 0 || len(got.Regions) != 0 {
			t.Errorf("geography = %v / %v, want no positive geography", got.Countries, got.Regions)
		}
	})
	t.Run("explicit restriction takes precedence over remote headquarters", func(t *testing.T) {
		got := Derive(Input{Location: "Remote",
			Description: "<p>Headquarters: United States - Remote, EMEA - Remote</p><p>Candidates must meet the requirements. You must be based in Canada.</p>"})
		if !slices.Equal(got.Countries, []string{"ca"}) || !slices.Equal(got.Regions, []string{"north_america"}) {
			t.Errorf("geography = %v / %v, want [ca] / [north_america]", got.Countries, got.Regions)
		}
	})
	t.Run("structured scope cannot restore an explicitly excluded country", func(t *testing.T) {
		got := Derive(Input{Location: "Remote / Worldwide", Countries: []string{"de"}, Regions: []string{"eu", "global"},
			Description: "This role is remote worldwide, except in Germany."})
		if len(got.Countries) != 0 || len(got.Regions) != 0 {
			t.Errorf("geography = %v / %v, want no positive geography", got.Countries, got.Regions)
		}
	})
	t.Run("search continues after rejected clause", func(t *testing.T) {
		got := Derive(Input{Location: "Remote", Description: exclusion + ". We are hiring in Canada and the UK."})
		if !slices.Equal(got.Countries, []string{"ca", "gb"}) || !slices.Equal(got.Regions, []string{"north_america", "uk"}) {
			t.Errorf("geography = %v / %v, want [ca gb] / [north_america uk]", got.Countries, got.Regions)
		}
	})
	t.Run("inferred location list contains both allowed and excluded countries", func(t *testing.T) {
		got := Derive(Input{Location: "Remote / Canada / France", Description: exclusion + "."})
		if !slices.Equal(got.Countries, []string{"ca"}) || !slices.Equal(got.Regions, []string{"north_america"}) {
			t.Errorf("geography = %v / %v, want [ca] / [north_america]", got.Countries, got.Regions)
		}
	})
	t.Run("city exclusion does not exclude its country", func(t *testing.T) {
		got := Derive(Input{Location: "Remote / Germany",
			Description: "We cannot accept applications from candidates based in Berlin, Germany."})
		if !slices.Equal(got.Countries, []string{"de"}) || !slices.Equal(got.Regions, []string{"eu"}) {
			t.Errorf("geography = %v / %v, want [de] / [eu]", got.Countries, got.Regions)
		}
	})
	t.Run("a separately named region survives a country exclusion", func(t *testing.T) {
		got := Derive(Input{Location: "Remote / Europe / France", Description: exclusion + "."})
		if len(got.Countries) != 0 || !slices.Equal(got.Regions, []string{"eu"}) {
			t.Errorf("geography = %v / %v, want [] / [eu]", got.Countries, got.Regions)
		}
	})
	t.Run("a macro-region from a description label survives a country exclusion", func(t *testing.T) {
		got := Derive(Input{Location: "Remote", Description: "Locations: France - Remote, EMEA - Remote\n" +
			"We cannot accept applications from candidates based in France."})
		if len(got.Countries) != 0 || !slices.Equal(got.Regions, []string{"africa", "eu", "mena", "uk"}) {
			t.Errorf("geography = %v / %v, want [] / [africa eu mena uk]", got.Countries, got.Regions)
		}
	})
	t.Run("structured city in the excluded country is removed", func(t *testing.T) {
		got := Derive(Input{Location: "Remote / Worldwide", Countries: []string{"de"}, Cities: []string{"Berlin"},
			Description: "This role is remote worldwide, except in Germany."})
		if len(got.Cities)+len(got.Metros) != 0 {
			t.Errorf("places = %v / %v, want none", got.Cities, got.Metros)
		}
	})
	t.Run("parsed city in the excluded country is removed", func(t *testing.T) {
		got := Derive(Input{Location: "Remote / Berlin, Germany / Toronto, Canada", Description: exclusion + "."})
		if !slices.Equal(got.Cities, []string{"Toronto"}) {
			t.Errorf("cities = %v, want [Toronto]", got.Cities)
		}
	})
	t.Run("optional restriction is still a denial", func(t *testing.T) {
		got := Derive(Input{Location: "Remote / Worldwide",
			Description: "Applications are open worldwide. Candidates are not required to be based in France."})
		if len(got.Countries) != 0 || !slices.Equal(got.Regions, []string{"global"}) {
			t.Errorf("geography = %v / %v, want [] / [global]", got.Countries, got.Regions)
		}
	})
}
