package search

import (
	"net/url"
	"reflect"
	"testing"
)

// auto_apply_available is stored as true-or-absent, like requires_clearance and
// ai_interview, so its filter cannot be a plain equality on both sides.
func TestFilterFromValues_AutoApplyAvailable(t *testing.T) {
	tests := []struct {
		name  string
		query string
		want  [][]string
	}{
		{
			name:  "true selects only the marked postings",
			query: "auto_apply_available=true",
			want:  [][]string{{"auto_apply_available = true"}},
		},
		{
			name:  "false negates the positive, catching the unmarked",
			query: "auto_apply_available=false",
			want:  [][]string{{"NOT auto_apply_available = true"}},
		},
		{
			name:  "an absent parameter filters nothing",
			query: "",
			want:  nil,
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

func TestFacetSettings_AutoApplyAvailableIsFilterable(t *testing.T) {
	s := facetSettings()
	if !contains(s.FilterableAttributes, "auto_apply_available") {
		t.Errorf("auto_apply_available must be filterable for the auto-apply facet, got %v", s.FilterableAttributes)
	}
}
