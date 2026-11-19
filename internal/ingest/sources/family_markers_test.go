package sources

import (
	"slices"
	"testing"
)

// The adapters moved into family packages keep the registry standing they had in this package.
// A family test cannot read the registry (it would import sources), so the checks live here.
func TestMovedATSAdaptersKeepTheirMarkers(t *testing.T) {
	reg := All(nil)
	full, empty := FullBoardListingProviders(reg), EmptyBoardListingProviders(reg)
	for _, p := range []string{"greenhouse", "lever", "ashby", "recruitee"} {
		if !full[p] || !empty[p] {
			t.Errorf("%s: FullBoardListing=%v EmptyBoardListing=%v, want both", p, full[p], empty[p])
		}
	}
}

func TestMovedAggregatorsKeepTheirRegistryStanding(t *testing.T) {
	reg := All(nil)
	filterable, aggregators := FilterableProviders(), AggregatorProviders(reg)
	for _, p := range []string{"adzuna", "eures", "jobnet", "jobtech", "nofluffjobs"} {
		if _, ok := reg[p]; !ok {
			t.Errorf("All() does not register %s", p)
		}
		if !slices.Contains(filterable, p) {
			t.Errorf("FilterableProviders() lacks %s", p)
		}
		if !slices.Contains(aggregators, p) {
			t.Errorf("AggregatorProviders() lacks %s", p)
		}
	}
	if !slices.Contains(BoardKeyedProviders(reg), "eures") || slices.Contains(FullCatalogProviders(reg), "eures") {
		t.Error("eures: want board-keyed (board = country) and not fullCatalog")
	}
	if !slices.Contains(FullCatalogProviders(reg), "jobnet") {
		t.Error("jobnet: want fullCatalog")
	}
	if !slices.Contains(UnseenSweepExemptProviders(reg), "jobtech") {
		t.Error("jobtech: want exempt from the unseen sweep")
	}
}
