package sources

import "testing"

// The adapters moved into sources/ats keep the listing markers they carried in this package; a
// family test cannot read the registry (it would import sources), so the check lives here.
func TestMovedATSAdaptersKeepTheirMarkers(t *testing.T) {
	reg := All(nil)
	full, empty := FullBoardListingProviders(reg), EmptyBoardListingProviders(reg)
	for _, p := range []string{"greenhouse", "lever", "ashby", "recruitee"} {
		if !full[p] || !empty[p] {
			t.Errorf("%s: FullBoardListing=%v EmptyBoardListing=%v, want both", p, full[p], empty[p])
		}
	}
}
