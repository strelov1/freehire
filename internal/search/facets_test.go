package search

import (
	"encoding/json"
	"strconv"
	"testing"

	"github.com/meilisearch/meilisearch-go"
)

func TestBuildFacetResult(t *testing.T) {
	t.Run("assembles total, facets and stats", func(t *testing.T) {
		resp := &meilisearch.SearchResponse{
			EstimatedTotalHits: 1234,
			FacetDistribution:  json.RawMessage(`{"regions":{"eu":800}}`),
			FacetStats:         json.RawMessage(`{"enrichment.salary_min":{"min":0,"max":400000}}`),
		}
		got, err := buildFacetResult(resp)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.Total != 1234 {
			t.Errorf("Total = %d, want 1234", got.Total)
		}
		if got.Facets["regions"]["eu"] != 800 {
			t.Errorf("Facets = %v", got.Facets)
		}
		if got.Stats["enrichment.salary_min"].Max != 400000 {
			t.Errorf("Stats = %v", got.Stats)
		}
	})

	t.Run("empty facets yield nil maps but keep total", func(t *testing.T) {
		resp := &meilisearch.SearchResponse{EstimatedTotalHits: 7}
		got, err := buildFacetResult(resp)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.Total != 7 || got.Facets != nil || got.Stats != nil {
			t.Errorf("got %+v, want total=7 nil maps", got)
		}
	})

	t.Run("invalid distribution errors", func(t *testing.T) {
		resp := &meilisearch.SearchResponse{FacetDistribution: json.RawMessage(`{bad`)}
		if _, err := buildFacetResult(resp); err == nil {
			t.Error("expected error for invalid distribution")
		}
	})
}

func TestIndexSettings_FacetingRaisesValueCap(t *testing.T) {
	// The Meili default of 100 truncates high-cardinality facets (skills,
	// countries) in the distribution; the analytics page needs the full set.
	f := indexSettings().Faceting
	if f == nil {
		t.Fatal("indexSettings().Faceting is nil; expected maxValuesPerFacet override")
	}
	if f.MaxValuesPerFacet <= 100 {
		t.Errorf("MaxValuesPerFacet = %d, want > 100", f.MaxValuesPerFacet)
	}
}

func TestDecodeFacetDistribution(t *testing.T) {
	t.Run("nil raw yields nil map", func(t *testing.T) {
		got, err := decodeFacetDistribution(nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != nil {
			t.Errorf("got %v, want nil", got)
		}
	})

	t.Run("parses attr to value to count", func(t *testing.T) {
		raw := json.RawMessage(`{"regions":{"eu":800,"us":300},"work_mode":{"remote":1200}}`)
		got, err := decodeFacetDistribution(raw)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got["regions"]["eu"] != 800 || got["regions"]["us"] != 300 {
			t.Errorf("regions = %v", got["regions"])
		}
		if got["work_mode"]["remote"] != 1200 {
			t.Errorf("work_mode = %v", got["work_mode"])
		}
	})

	t.Run("does not truncate high-cardinality facets", func(t *testing.T) {
		// 250 distinct skill values must all survive decode — the engine's
		// maxValuesPerFacet caps what is sent, but the decode layer never drops.
		skills := make(map[string]int64, 250)
		for i := 0; i < 250; i++ {
			skills["skill-"+strconv.Itoa(i)] = int64(i)
		}
		raw, _ := json.Marshal(map[string]map[string]int64{"skills": skills})
		got, err := decodeFacetDistribution(raw)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got["skills"]) != 250 {
			t.Errorf("got %d skill values, want 250", len(got["skills"]))
		}
	})

	t.Run("invalid json errors", func(t *testing.T) {
		if _, err := decodeFacetDistribution(json.RawMessage(`{bad`)); err == nil {
			t.Error("expected error for invalid json")
		}
	})
}

func TestDecodeFacetStats(t *testing.T) {
	t.Run("nil raw yields nil map", func(t *testing.T) {
		got, err := decodeFacetStats(nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != nil {
			t.Errorf("got %v, want nil", got)
		}
	})

	t.Run("parses min and max", func(t *testing.T) {
		raw := json.RawMessage(`{"enrichment.salary_min":{"min":0,"max":400000}}`)
		got, err := decodeFacetStats(raw)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		s := got["enrichment.salary_min"]
		if s.Min != 0 || s.Max != 400000 {
			t.Errorf("stat = %+v, want {0, 400000}", s)
		}
	})
}
