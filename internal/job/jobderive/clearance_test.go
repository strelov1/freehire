package jobderive

import "testing"

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

func ptr[T any](v T) *T { return &v }
