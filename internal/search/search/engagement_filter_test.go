package search

import (
	"net/url"
	"reflect"
	"testing"
)

// engagement is a multi-value facet over a closed vocabulary: included values OR,
// excluded values each become their own NOT group — "hide Inside IR35" keeps the
// postings that do not say.
func TestFilterFromValues_Engagement(t *testing.T) {
	tests := []struct {
		name  string
		query string
		want  [][]string
	}{
		{
			name:  "outside only",
			query: "engagement=ir35_outside",
			want:  [][]string{{`engagement = "ir35_outside"`}},
		},
		{
			name:  "either",
			query: "engagement=ir35_outside,ir35_inside",
			want:  [][]string{{`engagement = "ir35_inside"`, `engagement = "ir35_outside"`}},
		},
		{
			name:  "exclude inside",
			query: "engagement_exclude=ir35_inside",
			want:  [][]string{{`engagement != "ir35_inside"`}},
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
