package jobview

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/strelov1/freehire/internal/job/job"
	"github.com/strelov1/freehire/internal/job/jobderive"
)

// TestAutoApplyProviders_ExactExpectedSet is a local snapshot check, not a
// cross-package one: it cannot see atsapply's real fillProviders/
// browserUseProviders (jobview, layer 5, cannot import atsapply, in api,
// layer 8), so it only catches an accidental edit to this literal, not
// atsapply's own maps drifting away from it unnoticed.
// TestAutoApplyFacetProvidersMatchThisPackagesOwnMaps in
// internal/api/atsapply is what closes that real gap, from the side that
// can see both.
func TestAutoApplyProviders_ExactExpectedSet(t *testing.T) {
	want := map[string]bool{
		"greenhouse": true,
		"lever":      true,
		"ashby":      true,
		"workable":   true,
	}
	if len(AutoApplyProviders) != len(want) {
		t.Fatalf("AutoApplyProviders = %v, want exactly %v", AutoApplyProviders, want)
	}
	for provider := range want {
		if !AutoApplyProviders[provider] {
			t.Errorf("AutoApplyProviders missing expected provider %q", provider)
		}
	}
}

// The facet is served only when the posting's source is one of the four
// eligible ATS providers. A non-eligible source carries no key at all, so a
// consumer cannot mistake "not one of the providers we can drive" for "this
// posting was checked and found ineligible" — matching the true-or-absent
// contract every derived boolean facet in this codebase follows.
func TestFromDomain_AutoApplyAvailableFacet(t *testing.T) {
	tests := []struct {
		name    string
		source  string
		want    bool
		wantKey bool
	}{
		{name: "greenhouse is eligible", source: "greenhouse", want: true, wantKey: true},
		{name: "lever is eligible", source: "lever", want: true, wantKey: true},
		{name: "ashby is eligible", source: "ashby", want: true, wantKey: true},
		{name: "workable is eligible", source: "workable", want: true, wantKey: true},
		{name: "recruitee is not eligible", source: "recruitee", want: false, wantKey: false},
		{name: "an arbitrary non-ATS source is not eligible", source: "djinni", want: false, wantKey: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			j, err := job.New(job.Draft{Input: jobderive.Input{
				Source:      tt.source,
				ExternalID:  "acme:1",
				Title:       "Backend Engineer",
				Company:     "Acme",
				Description: "We use Go and Kubernetes.",
			}})
			if err != nil {
				t.Fatalf("New: %v", err)
			}
			v, err := FromDomain(j, job.Extras{})
			if err != nil {
				t.Fatalf("FromDomain: %v", err)
			}
			if v.AutoApplyAvailable != tt.want {
				t.Errorf("AutoApplyAvailable = %v, want %v", v.AutoApplyAvailable, tt.want)
			}

			b, err := json.Marshal(v)
			if err != nil {
				t.Fatalf("Marshal: %v", err)
			}
			if gotKey := strings.Contains(string(b), `"auto_apply_available"`); gotKey != tt.wantKey {
				t.Errorf("auto_apply_available key present = %v, want %v", gotKey, tt.wantKey)
			}
		})
	}
}
