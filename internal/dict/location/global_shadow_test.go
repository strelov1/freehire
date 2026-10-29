package location

import (
	"slices"
	"testing"
)

// The location lines below are the most common shapes of the 523 open postings that,
// measured 2026-10-29, carried the global region beside a narrower place. Read by hand,
// almost every one is restricted to that place: the open-anywhere word is a requisition
// bucket ("Anywhere, USA"), an ATS prefix ("International - Germany") or a headquarters
// label ("Global Headquarters - Dublin").
func TestParseNarrowerPlaceShadowsGlobal(t *testing.T) {
	tests := []struct {
		location      string
		wantCountries []string
		wantRegions   []string
	}{
		{"Anywhere, USA", []string{"us"}, []string{"north_america"}},
		{"Remote - Anywhere - USA", []string{"us"}, []string{"north_america"}},
		{"PNNL - Anywhere, USA", []string{"us"}, []string{"north_america"}},
		{"International - Germany", []string{"de"}, []string{"eu"}},
		{"International-Saudi Arabia-All", []string{"sa"}, []string{"mena"}},
		{"Global, SP, BR", []string{"br"}, []string{"latam"}},
		{"Worldwide, Croatia", []string{"hr"}, []string{"eu"}},
		{"Anywhere; Europe", nil, []string{"eu"}},
		{"Europe - Anywhere", nil, []string{"eu"}},
		{"Global, EU", nil, []string{"eu"}},
		// The open-anywhere word alone is still global.
		{"Remote - Worldwide", nil, []string{"global"}},
		// Alternatives joined by "or" offer anywhere as one option: the posting is open
		// worldwide, the place is just the other choice.
		{"Remote anywhere or On-site Prague, Czech Republic", []string{"cz"}, []string{"eu", "global"}},
		{"New York City (preferred) or Global Remote", []string{"us"}, []string{"global", "north_america"}},
	}
	for _, tt := range tests {
		t.Run(tt.location, func(t *testing.T) {
			g := Parse(tt.location)
			if !slices.Equal(g.Countries, tt.wantCountries) {
				t.Errorf("countries = %v, want %v", g.Countries, tt.wantCountries)
			}
			if !slices.Equal(g.Regions, tt.wantRegions) {
				t.Errorf("regions = %v, want %v", g.Regions, tt.wantRegions)
			}
		})
	}
}

func TestWithoutShadowedGlobal(t *testing.T) {
	tests := []struct {
		name      string
		countries []string
		regions   []string
		line      string
		want      []string
	}{
		{"global alone stays", nil, []string{"global"}, "Worldwide", []string{"global"}},
		{"a country shadows global", []string{"us"}, []string{"global", "north_america"}, "", []string{"north_america"}},
		{"a region shadows global", nil, []string{"eu", "global"}, "", []string{"eu"}},
		{"stated alternatives keep global", []string{"cz"}, []string{"eu", "global"}, "Anywhere or Prague", []string{"eu", "global"}},
		{"no global is untouched", []string{"de"}, []string{"eu"}, "", []string{"eu"}},
		{"nothing stays nothing", nil, nil, "", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := WithoutShadowedGlobal(tt.countries, tt.regions, tt.line); !slices.Equal(got, tt.want) {
				t.Errorf("WithoutShadowedGlobal = %v, want %v", got, tt.want)
			}
		})
	}
}
