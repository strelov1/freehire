package search

import (
	"net/url"
	"reflect"
	"testing"
)

// via_recruiter is true-or-absent like requires_clearance: "hide recruiter posts" must
// keep every posting the dictionary did not mark, so false negates the positive.
func TestFilterFromValues_ViaRecruiter(t *testing.T) {
	tests := []struct {
		name  string
		query string
		want  [][]string
	}{
		{"true selects only the marked postings", "via_recruiter=true", [][]string{{"via_recruiter = true"}}},
		{"false negates the positive", "via_recruiter=false", [][]string{{"NOT via_recruiter = true"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v, err := url.ParseQuery(tt.query)
			if err != nil {
				t.Fatalf("ParseQuery: %v", err)
			}
			if got := normalizeGroups(t, FilterFromValues(v)); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("filter = %v, want %v", got, tt.want)
			}
		})
	}
}
