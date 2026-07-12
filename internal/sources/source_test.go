package sources

import (
	"context"
	"slices"
	"testing"
)

type fakeSource struct{ provider string }

func (f fakeSource) Provider() string { return f.provider }

func (f fakeSource) Fetch(context.Context, CompanyEntry) ([]Job, error) { return nil, nil }

// fakeBoardlessSource is a fakeSource that declares itself boardless, so config
// validation lets its entries omit board.
type fakeBoardlessSource struct{ provider string }

func (f fakeBoardlessSource) Provider() string { return f.provider }

func (f fakeBoardlessSource) Fetch(context.Context, CompanyEntry) ([]Job, error) { return nil, nil }

func (f fakeBoardlessSource) boardless() {}

// fakeAggregatorSource is boardless (no board id) but aggregates many companies,
// so it declares itself an aggregator and must stay in the source facet.
type fakeAggregatorSource struct{ provider string }

func (f fakeAggregatorSource) Provider() string { return f.provider }

func (f fakeAggregatorSource) Fetch(context.Context, CompanyEntry) ([]Job, error) {
	return nil, nil
}

func (f fakeAggregatorSource) boardless()  {}
func (f fakeAggregatorSource) aggregator() {}

func TestRegIndexesByProvider(t *testing.T) {
	r := reg(fakeSource{"greenhouse"}, fakeSource{"lever"})

	if len(r) != 2 {
		t.Fatalf("len(reg) = %d, want 2", len(r))
	}
	if _, ok := r["greenhouse"]; !ok {
		t.Errorf("reg missing provider %q", "greenhouse")
	}
	if got := r["lever"].Provider(); got != "lever" {
		t.Errorf("reg[lever].Provider() = %q, want %q", got, "lever")
	}
}

func TestRegPanicsOnDuplicateProvider(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("reg with duplicate provider should panic")
		}
	}()

	reg(fakeSource{"greenhouse"}, fakeSource{"greenhouse"})
}

func TestFilterableProviders(t *testing.T) {
	got := FilterableProviders()

	// Sorted, and multi-tenant ATS adapters are present.
	for _, want := range []string{"greenhouse", "lever", "ashby", "workday"} {
		if !slices.Contains(got, want) {
			t.Errorf("FilterableProviders() missing %q", want)
		}
	}
	// Single-company boardless adapters are excluded.
	for _, excluded := range []string{"yandex", "ozon", "tbank", "vk", "sber"} {
		if slices.Contains(got, excluded) {
			t.Errorf("FilterableProviders() should exclude boardless %q", excluded)
		}
	}
	if !slices.IsSorted(got) {
		t.Errorf("FilterableProviders() not sorted: %v", got)
	}
}

func TestAggregatorProvidersListsOnlyAggregators(t *testing.T) {
	got := AggregatorProviders(reg(
		fakeSource{"greenhouse"},         // board-based ATS → excluded
		fakeBoardlessSource{"ozon"},      // single-company boardless (not aggregator) → excluded
		fakeAggregatorSource{"jobstash"}, // aggregator → listed
		fakeAggregatorSource{"himalayas"},
	))

	want := []string{"himalayas", "jobstash"}
	if !slices.Equal(got, want) {
		t.Errorf("AggregatorProviders() = %v, want %v", got, want)
	}
}

func TestFilterableProvidersKeepsAggregatorsDropsSingleCompany(t *testing.T) {
	got := filterableProviders(reg(
		fakeSource{"greenhouse"},         // board-based → listed
		fakeBoardlessSource{"ozon"},      // single-company boardless → excluded
		fakeAggregatorSource{"jobstash"}, // aggregator boardless → listed
	))

	if !slices.Contains(got, "greenhouse") {
		t.Errorf("filterableProviders() missing board-based %q", "greenhouse")
	}
	if !slices.Contains(got, "jobstash") {
		t.Errorf("filterableProviders() should list aggregator %q", "jobstash")
	}
	if slices.Contains(got, "ozon") {
		t.Errorf("filterableProviders() should exclude single-company boardless %q", "ozon")
	}
}
