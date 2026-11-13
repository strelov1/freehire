package mcpapp

import (
	"context"
	"slices"
	"testing"
)

func TestMarketStatsKeepsTheTopValuesOfEachPublishedFacet(t *testing.T) {
	// A model reads these to say "most remote Go roles are senior backend ones"; the whole
	// distribution (thousands of cities) would bury that in the turn's context.
	counts := map[string]map[string]int64{
		"skills":       {"go": 50, "kubernetes": 40, "php": 5},
		"seniority":    {"senior": 30, "middle": 20},
		"cities":       {"berlin": 9},        // not a published facet: dropped
		"ai_archetype": {"agent_builder": 3}, // not a published facet: dropped
	}
	for i := range 20 {
		counts["skills"][string(rune('a'+i))] = int64(i)
	}

	got := NewProjector(testOrigin).MarketStats(1234, counts)

	if got.Total != 1234 {
		t.Errorf("total = %d, want 1234", got.Total)
	}
	skills := got.Facets["skills"]
	if len(skills) != marketTopValues {
		t.Fatalf("skills kept %d values, want the top %d", len(skills), marketTopValues)
	}
	if skills[0] != (FacetCount{Value: "go", Count: 50}) || skills[1].Value != "kubernetes" {
		t.Errorf("skills = %+v, want highest count first", skills[:2])
	}
	if _, ok := got.Facets["cities"]; ok {
		t.Error("cities is published, though its long tail is noise in a chat turn")
	}
	if _, ok := got.Facets["ai_archetype"]; ok {
		t.Error("an internal facet reached the answer")
	}
}

func TestMarketStatsOrdersTiesByValueSoTheAnswerIsStable(t *testing.T) {
	got := NewProjector(testOrigin).MarketStats(2, map[string]map[string]int64{"work_mode": {"remote": 1, "hybrid": 1}})
	if want := []FacetCount{{"hybrid", 1}, {"remote", 1}}; !slices.Equal(got.Facets["work_mode"], want) {
		t.Errorf("work_mode = %v, want %v", got.Facets["work_mode"], want)
	}
}

func TestSalaryScopeDropsAndNamesWhatItCannotScopeBy(t *testing.T) {
	in := SalaryInput{Category: "Backend", Seniority: "rockstar", Country: "Germany"}

	category, seniority, country := in.Scope()
	if category != "backend" || seniority != "" || country != "" {
		t.Errorf("scope = %q/%q/%q, want backend and the two unknowns dropped", category, seniority, country)
	}
	if got := in.Unsupported(); !slices.Equal(got, []string{"country=Germany", "seniority=rockstar"}) {
		t.Errorf("unsupported = %v, want both unknowns named", got)
	}
}

func TestSalaryScopeLowercasesACountryCode(t *testing.T) {
	// The salary rollup keys countries lowercase, as every geography facet here is stored.
	if _, _, country := (SalaryInput{Country: "DE"}).Scope(); country != "de" {
		t.Errorf("country = %q, want de", country)
	}
}

func TestSimilarJobsIsCappedAndDefaulted(t *testing.T) {
	var asked int
	r := readerThatFinds(t)
	r.similarJobs = func(_ context.Context, _ string, limit int) (SimilarJobsResult, error) {
		asked = limit
		return SimilarJobsResult{}, nil
	}
	session := connect(t, r)

	var got SimilarJobsResult
	callJSON(t, session, toolSimilarJobs, map[string]any{"slug": "x"}, &got)
	if asked != defaultSimilar {
		t.Errorf("no limit asked for %d, want the default %d", asked, defaultSimilar)
	}
	callJSON(t, session, toolSimilarJobs, map[string]any{"slug": "x", "limit": 500}, &got)
	if asked != maxSimilar {
		t.Errorf("limit 500 asked for %d, want the cap %d", asked, maxSimilar)
	}
}
