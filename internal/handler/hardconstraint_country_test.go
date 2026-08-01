package handler

import "testing"

// TestCandidateCountry pins the precedence between what the candidate ASSERTED about
// where they are and what was DERIVED for them from their CV.
//
// The fallback exists because two hard constraints — "this job does not sponsor visas and
// is pinned to a country you are not in" and "this on-site job is in another country" —
// read this one field, and it was empty for the large majority of profiles, so both were
// silently inert. Filling it from the CV revives them without ever overriding the user.
func TestCandidateCountry(t *testing.T) {
	tests := []struct {
		name     string
		asserted string
		derived  []string
		want     string
	}{
		{
			name:     "an asserted base country always wins",
			asserted: "de",
			derived:  []string{"pl"},
			want:     "de",
		},
		{
			name:     "a single derived country fills an unstated base",
			asserted: "",
			derived:  []string{"pl"},
			want:     "pl",
		},
		{
			// The dictionary's never-guess rule reaches all the way here. An ambiguous
			// derivation is not a fact about the candidate, and picking one of the two
			// would be manufacturing evidence they never supplied.
			name:     "more than one derived country yields nothing rather than a guess",
			asserted: "",
			derived:  []string{"pl", "de"},
			want:     "",
		},
		{
			name:     "neither source yields nothing, exactly as before",
			asserted: "",
			derived:  nil,
			want:     "",
		},
		{
			// A CV that stated a place the dictionary could not resolve arrives as an
			// empty (non-nil) slice. It is still no answer.
			name:     "a stated but unresolved location yields nothing",
			asserted: "",
			derived:  []string{},
			want:     "",
		},
		{
			// The user's own statement wins even when the derivation is ambiguous.
			name:     "an asserted country wins over an ambiguous derivation",
			asserted: "br",
			derived:  []string{"pl", "de"},
			want:     "br",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := candidateCountry(tt.asserted, tt.derived); got != tt.want {
				t.Errorf("candidateCountry(%q, %v) = %q, want %q", tt.asserted, tt.derived, got, tt.want)
			}
		})
	}
}
