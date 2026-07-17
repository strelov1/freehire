package handler

import (
	"testing"

	"github.com/strelov1/freehire/internal/db"
)

func TestGroupFacetStats_EmptyYieldsTheFourEmptyMaps(t *testing.T) {
	got := groupFacetStats(nil)

	want := []string{"countries", "skills", "seniority", "work_mode"}
	if len(got) != len(want) {
		t.Fatalf("got %d facets, want the four covered facets: %v", len(got), got)
	}
	for _, f := range want {
		m, ok := got[f]
		if !ok {
			t.Errorf("missing facet %q; got %v", f, got)
			continue
		}
		if len(m) != 0 {
			t.Errorf("facet %q should be empty for an empty snapshot; got %v", f, m)
		}
	}
}

func TestGroupFacetStats_GroupsRowsByFacet(t *testing.T) {
	rows := []db.InsightsFacetStat{
		{Facet: "countries", Value: "us", Count: 10},
		{Facet: "countries", Value: "de", Count: 4},
		{Facet: "skills", Value: "go", Count: 7},
		{Facet: "seniority", Value: "senior", Count: 5},
		{Facet: "work_mode", Value: "remote", Count: 8},
	}

	got := groupFacetStats(rows)

	if got["countries"]["us"] != 10 || got["countries"]["de"] != 4 {
		t.Errorf("countries grouped wrong: %v", got["countries"])
	}
	if got["skills"]["go"] != 7 {
		t.Errorf("skills grouped wrong: %v", got["skills"])
	}
	if got["seniority"]["senior"] != 5 {
		t.Errorf("seniority grouped wrong: %v", got["seniority"])
	}
	if got["work_mode"]["remote"] != 8 {
		t.Errorf("work_mode grouped wrong: %v", got["work_mode"])
	}
}
