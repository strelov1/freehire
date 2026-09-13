package atsapply

import (
	"testing"

	"github.com/strelov1/freehire/internal/job/jobview"
)

// The auto_apply_available facet (internal/job/jobview's AutoApplyProviders)
// is a manually-synced mirror of this package's own fillProviders ∪
// browserUseProviders — jobview (job, layer 5) cannot import atsapply (api,
// layer 8), so the facet's copy cannot be verified from the jobview package
// itself. This test closes that gap from the side that CAN see both: atsapply
// (layer 8) is free to import jobview (layer 5). Without it, an edit to
// fillProviders or browserUseProviders with no matching edit in
// internal/job/jobview passes every check in both packages and the facet
// silently mis-reports eligibility.
func TestAutoApplyFacetProvidersMatchThisPackagesOwnMaps(t *testing.T) {
	want := make(map[string]bool, len(fillProviders)+len(browserUseProviders))
	for p := range fillProviders {
		want[p] = true
	}
	for p := range browserUseProviders {
		want[p] = true
	}

	got := jobview.AutoApplyProviders
	if len(got) != len(want) {
		t.Fatalf("jobview.AutoApplyProviders = %v, want exactly %v (fillProviders ∪ browserUseProviders)", got, want)
	}
	for p := range want {
		if !got[p] {
			t.Errorf("jobview.AutoApplyProviders missing %q, present in atsapply's own fillProviders/browserUseProviders", p)
		}
	}
}
