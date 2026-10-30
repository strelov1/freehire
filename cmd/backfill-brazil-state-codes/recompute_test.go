package main

import (
	"slices"
	"testing"

	"github.com/strelov1/freehire/internal/platform/db"
)

func TestRecomputePlaces(t *testing.T) {
	tests := []struct {
		name          string
		row           db.JobsForPlaceRecheckByIDsRow
		wantCountries []string
		wantCities    []string
	}{
		{
			name:          "RS was Serbia",
			row:           db.JobsForPlaceRecheckByIDsRow{Source: "gupy", Location: "Porto Alegre, RS, Brasil", Countries: []string{"br", "rs"}},
			wantCountries: []string{"br"},
			wantCities:    []string{"Porto Alegre"},
		},
		{
			name:          "SC was South Carolina",
			row:           db.JobsForPlaceRecheckByIDsRow{Source: "vagas", Location: "Florianópolis, SC, Brasil", Countries: []string{"br", "us"}},
			wantCountries: []string{"br"},
			wantCities:    []string{"Florianópolis"},
		},
		{
			name:          "a state with no town",
			row:           db.JobsForPlaceRecheckByIDsRow{Source: "gupy", Location: "BA, Brasil", Countries: []string{"ba", "br"}},
			wantCountries: []string{"br"},
		},
		{
			// solides names no country for a town the dictionary does not know; the adapter's
			// fallback is what placed it, and the recompute must keep it rather than empty it.
			name:          "the adapter's fallback country is kept",
			row:           db.JobsForPlaceRecheckByIDsRow{Source: "solides", Location: "Ipojuca, PE", Countries: []string{"pe"}},
			wantCountries: []string{"br"},
			wantCities:    []string{"Ipojuca"},
		},
		{
			name:          "a genuinely foreign posting of a Brazilian source keeps its country",
			row:           db.JobsForPlaceRecheckByIDsRow{Source: "gupy", Location: "Lima, Lima, PE", Countries: []string{"pe"}},
			wantCountries: []string{"pe"},
			wantCities:    []string{"Lima"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := recomputePlaces(tt.row)
			if !slices.Equal(got.Countries, tt.wantCountries) || !slices.Equal(got.Cities, tt.wantCities) {
				t.Errorf("recomputePlaces(%q) = countries %v cities %v, want %v %v",
					tt.row.Location, got.Countries, got.Cities, tt.wantCountries, tt.wantCities)
			}
		})
	}
}
