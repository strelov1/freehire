package search

import (
	"net/url"
	"reflect"
	"testing"
)

// perks is a multi-value facet over a closed vocabulary: included values OR (or AND
// with perks_mode=and), excluded values each become their own NOT group.
func TestFilterFromValues_Perks(t *testing.T) {
	tests := []struct {
		name  string
		query string
		want  [][]string
	}{
		{
			name:  "one perk",
			query: "perks=401k_match",
			want:  [][]string{{`perks = "401k_match"`}},
		},
		{
			name:  "two perks OR",
			query: "perks=unlimited_pto,health_insurance",
			want:  [][]string{{`perks = "health_insurance"`, `perks = "unlimited_pto"`}},
		},
		{
			name:  "and mode asks for every perk",
			query: "perks=unlimited_pto&perks=health_insurance&perks_mode=and",
			want:  [][]string{{`perks = "health_insurance"`}, {`perks = "unlimited_pto"`}},
		},
		{
			name:  "exclude",
			query: "perks_exclude=sign_on_bonus",
			want:  [][]string{{`perks != "sign_on_bonus"`}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v, err := url.ParseQuery(tt.query)
			if err != nil {
				t.Fatalf("ParseQuery: %v", err)
			}
			got := normalizeGroups(t, FilterFromValues(v))
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("filter = %v, want %v", got, tt.want)
			}
		})
	}
}
