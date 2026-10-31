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

// clearance_levels ORs its levels in one group like any multi-value facet. Its "none"
// value — no explicit reference to a clearance — is answered from requires_clearance,
// which every indexed document already carries, so it is right before the levels
// reach the index and identical to the legacy requires_clearance=false.
func TestFilterFromValues_ClearanceLevels(t *testing.T) {
	tests := []struct {
		name  string
		query string
		want  [][]string
	}{
		{name: "uppercase none", query: "clearance_levels=NONE", want: [][]string{{"NOT requires_clearance = true"}}},
		{name: "padded none", query: "clearance_levels=+none+", want: [][]string{{"NOT requires_clearance = true"}}},
		{name: "uppercase exclusion", query: "clearance_levels_exclude=NONE", want: [][]string{{"requires_clearance = true"}}},
		{name: "padded exclusion", query: "clearance_levels_exclude=+none+", want: [][]string{{"requires_clearance = true"}}},
		{
			name:  "levels OR within the group",
			query: "clearance_levels=secret&clearance_levels=ts_sci",
			want:  [][]string{{`clearance_levels = "secret"`, `clearance_levels = "ts_sci"`}},
		},
		{
			name:  "none selects the postings not marked as requiring a clearance",
			query: "clearance_levels=none",
			want:  [][]string{{"NOT requires_clearance = true"}},
		},
		{
			name:  "none ORs with a real level",
			query: "clearance_levels=none,public_trust",
			want:  [][]string{{"NOT requires_clearance = true", `clearance_levels = "public_trust"`}},
		},
		{
			name:  "excluding none keeps only the postings that state a clearance",
			query: "clearance_levels_exclude=none",
			want:  [][]string{{"requires_clearance = true"}},
		},
		{
			name:  "excluding a level is an inequality",
			query: "clearance_levels_exclude=top_secret",
			want:  [][]string{{`clearance_levels != "top_secret"`}},
		},
		{
			name:  "the boolean and the levels compose as two AND groups",
			query: "requires_clearance=true&clearance_levels=secret",
			want:  [][]string{{`clearance_levels = "secret"`}, {"requires_clearance = true"}},
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

// The level vocabulary is closed, so a mistyped level is named in meta.ignored_values
// instead of passing for an empty market; the reserved "none" is known.
func TestIgnoredValues_ClearanceLevels(t *testing.T) {
	v, err := url.ParseQuery("clearance_levels=none,secret,topsecret&clearance_levels_exclude=other")
	if err != nil {
		t.Fatalf("ParseQuery: %v", err)
	}
	got := IgnoredValues(v)
	want := map[string][]string{"clearance_levels": {"topsecret"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("IgnoredValues = %v, want %v", got, want)
	}
}
