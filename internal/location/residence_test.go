package location

import (
	"reflect"
	"testing"
)

// TestParseResidence pins the candidate-side rule: a Residence is what Parse yields
// MINUS the two things that are true of a job and false of a person — the work-mode
// hint, and the "global" region.
//
// Residence has no WorkMode field at all, so that half of the rule is enforced by the
// type rather than by a test; what is tested here is that a work-mode marker still
// helps resolve the place it is attached to without leaking a mode or a global region.
func TestParseResidence(t *testing.T) {
	tests := []struct {
		name     string
		location string
		want     Residence
	}{
		{
			// Valencia is ambiguous (Spain and Venezuela), so the parser's city-agreement
			// check suppresses the city facet while still pinning the country from the
			// spelled-out country name. Residence inherits that never-guess behaviour
			// unchanged — it subtracts from Parse, it does not soften it.
			name:     "an ambiguous city still pins the country, without claiming the city",
			location: "Valencia, Spain",
			want:     Residence{Countries: []string{"es"}, Regions: []string{"eu"}},
		},
		{
			name:     "an unambiguous city is carried through",
			location: "Kraków, Poland",
			want:     Residence{Countries: []string{"pl"}, Regions: []string{"eu"}, Cities: []string{"Kraków"}},
		},
		{
			name:     "empty input yields nothing",
			location: "",
			want:     Residence{},
		},
		{
			name:     "unresolvable location yields nothing rather than a guess",
			location: "Nyarugenge District, Nyakabanda Sector",
			want:     Residence{},
		},
		// The "global" region reaches a candidate by TWO independent paths, and the rule
		// has to cut both. Phrasing it as "do not inherit the bare-remote fallback" would
		// have missed the second one.
		{
			// Path 1: the bare-remote fallback in Parse — a remote marker that resolved no
			// place is treated as open-anywhere. True of a job, false of a person.
			name:     "bare remote marker does not make a candidate global",
			location: "Remote (GMT+3)",
			want:     Residence{},
		},
		{
			// Path 2: the dictionary's own worldwide/anywhere -> global entries. This one
			// never touches the fallback at all.
			name:     "an explicit worldwide word does not make a candidate global",
			location: "REMOTE · WORLDWIDE",
			want:     Residence{},
		},
		{
			name:     "a bare anywhere token does not make a candidate global",
			location: "Anywhere",
			want:     Residence{},
		},
		{
			// A real macro-region IS a true claim about where a person is, and survives.
			name:     "a real macro region without a country survives",
			location: "EU / Remote",
			want:     Residence{Regions: []string{"eu"}},
		},
		{
			// The place is where the person is; the remoteness is not geography at all.
			name:     "a place stated beside a remote marker survives",
			location: "Berlin, Germany (Remote)",
			want:     Residence{Countries: []string{"de"}, Regions: []string{"eu"}, Cities: []string{"Berlin"}},
		},
		{
			name:     "a US state resolves the same way it does for a job",
			location: "San Francisco, CA",
			want:     Residence{Countries: []string{"us"}, Regions: []string{"north_america"}, Cities: []string{"San Francisco"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseResidence(tt.location)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ParseResidence(%q) = %+v, want %+v", tt.location, got, tt.want)
			}
		})
	}
}

// TestParseResidenceNeverEmitsGlobal is the exhaustive form of the rule above: whatever
// the input, "global" must never appear. The table test pins the known paths; this one
// guards the paths nobody has thought of yet, including any future dictionary entry that
// maps a new phrase to the global region.
func TestParseResidenceNeverEmitsGlobal(t *testing.T) {
	for name, region := range nameToRegion {
		if region != "global" {
			continue
		}
		if got := ParseResidence(name); len(got.Regions) > 0 {
			t.Errorf("ParseResidence(%q) = regions %v, want none — %q maps to the global region", name, got.Regions, name)
		}
	}
}
