package facetsnapshot

import (
	"slices"
	"testing"

	"github.com/strelov1/freehire/internal/search"
)

func TestAttributesCoversTheFourFacets(t *testing.T) {
	attrs := Attributes()
	want := []string{"countries", "skills", "enrichment.seniority", "work_mode"}
	for _, a := range want {
		if !slices.Contains(attrs, a) {
			t.Errorf("Attributes() missing %q; got %v", a, attrs)
		}
	}
	if len(attrs) != len(want) {
		t.Errorf("Attributes() = %v, want exactly the four covered attrs %v", attrs, want)
	}
}

func TestRowsKeysByPublicParamAndCoversOnlyTheFour(t *testing.T) {
	res := search.FacetResult{
		Facets: map[string]map[string]int64{
			"countries":            {"us": 10, "de": 4},
			"skills":               {"go": 7, "python": 3},
			"enrichment.seniority": {"senior": 5, "junior": 2},
			"work_mode":            {"remote": 8, "onsite": 1},
			// An extra attribute the snapshot does not cover must be dropped.
			"enrichment.category": {"backend": 99},
		},
	}

	rows := Rows(res)

	// Re-key into a map for assertion convenience.
	got := map[string]map[string]int64{}
	for _, r := range rows {
		if got[r.Facet] == nil {
			got[r.Facet] = map[string]int64{}
		}
		got[r.Facet][r.Value] = r.Count
	}

	if _, ok := got["category"]; ok {
		t.Errorf("Rows kept an uncovered facet: %v", got)
	}
	if _, ok := got["enrichment.category"]; ok {
		t.Errorf("Rows kept an uncovered facet by attr name: %v", got)
	}

	// seniority must be keyed by its PUBLIC param, not the index attribute.
	if got["seniority"]["senior"] != 5 {
		t.Errorf("seniority not keyed by public param; got %v", got)
	}
	if _, ok := got["enrichment.seniority"]; ok {
		t.Errorf("Rows kept the raw index attr key: %v", got)
	}

	for _, f := range []string{"countries", "skills", "seniority", "work_mode"} {
		if len(got[f]) == 0 {
			t.Errorf("Rows missing facet %q; got %v", f, got)
		}
	}
	if got["countries"]["us"] != 10 || got["work_mode"]["remote"] != 8 {
		t.Errorf("Rows carried wrong counts; got %v", got)
	}
}
