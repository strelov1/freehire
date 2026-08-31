package search

import (
	"net/url"
	"reflect"
	"testing"
)

// requires_clearance is stored as true-or-absent, never false, so its filter cannot
// be a plain equality on both sides. Asking to EXCLUDE clearance jobs must return
// every posting the system did not mark — which is most of the catalogue and carries
// no such attribute at all — so it negates the positive rather than testing for a
// false that is never written.
func TestFilterFromValues_RequiresClearance(t *testing.T) {
	tests := []struct {
		name  string
		query string
		want  [][]string
	}{
		{
			name:  "true selects only the marked postings",
			query: "requires_clearance=true",
			want:  [][]string{{"requires_clearance = true"}},
		},
		{
			name:  "false negates the positive, catching the unmarked and the unknown",
			query: "requires_clearance=false",
			want:  [][]string{{"NOT requires_clearance = true"}},
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
