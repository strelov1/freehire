package jobderive

import (
	"slices"
	"testing"
)

// TestDerive_RequiresClearance covers the tri-state the column stores. Only two of
// the three states are ever produced: true when the description states a requirement,
// nil when it does not. A false is never written, because the dictionary cannot
// distinguish a posting that promises no clearance from one that is simply silent.
func TestDerive_RequiresClearance(t *testing.T) {
	tests := []struct {
		name string
		desc string
		want *bool
	}{
		{
			name: "stated requirement marks the job",
			desc: "You must hold or be eligible for SC clearance before starting.",
			want: ptr(true),
		},
		{
			name: "a silent description leaves it unknown",
			desc: "We are hiring a backend engineer to work on our Go services.",
			want: nil,
		},
		{
			name: "a denied requirement leaves it unknown, not false",
			desc: "No security clearance is required for this role.",
			want: nil,
		},
		{
			name: "an empty description leaves it unknown",
			desc: "",
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Derive(Input{
				Title:       "Backend Engineer",
				Company:     "Acme",
				Source:      "greenhouse",
				ExternalID:  "1",
				Description: tt.desc,
			}).RequiresClearance
			switch {
			case tt.want == nil && got != nil:
				t.Fatalf("RequiresClearance = %v, want nil", *got)
			case tt.want != nil && got == nil:
				t.Fatalf("RequiresClearance = nil, want %v", *tt.want)
			case tt.want != nil && *got != *tt.want:
				t.Fatalf("RequiresClearance = %v, want %v", *got, *tt.want)
			}
		})
	}
}

// TestDerive_ClearanceLevels pins the invariant the search filter leans on: the levels
// are non-empty exactly when RequiresClearance is set, because "no explicit reference"
// is the absence of levels and must never cover a posting the boolean marks.
func TestDerive_ClearanceLevels(t *testing.T) {
	tests := []struct {
		name string
		desc string
		want []string
	}{
		{"a named rung", "An active Secret clearance is required to start.", []string{"secret"}},
		{"a requirement with no rung is other", "An active security clearance is required.", []string{"other"}},
		{"a silent description has none", "We are hiring a backend engineer.", nil},
		{"a denied requirement has none", "No security clearance is required for this role.", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := Derive(Input{
				Title: "Backend Engineer", Company: "Acme", Source: "greenhouse", ExternalID: "1",
				Description: tt.desc,
			})
			if !slices.Equal(d.ClearanceLevels, tt.want) {
				t.Errorf("ClearanceLevels = %v, want %v", d.ClearanceLevels, tt.want)
			}
			if (len(d.ClearanceLevels) > 0) != (d.RequiresClearance != nil) {
				t.Errorf("levels %v disagree with RequiresClearance %v", d.ClearanceLevels, d.RequiresClearance)
			}
		})
	}
}

func ptr[T any](v T) *T { return &v }
