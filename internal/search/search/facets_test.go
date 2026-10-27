package search

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"reflect"
	"strconv"
	"sync"
	"testing"

	"github.com/meilisearch/meilisearch-go"
)

func TestDisjunctiveFacetCounts(t *testing.T) {
	type call struct {
		filter any
		facets []string
	}
	var mu sync.Mutex
	var calls []call
	searcher := func(_ context.Context, _ string, filter any, facets []string) (*meilisearch.SearchResponse, error) {
		mu.Lock()
		calls = append(calls, call{filter, facets})
		mu.Unlock()
		if len(facets) == 0 { // total-only query under the full filter
			return &meilisearch.SearchResponse{EstimatedTotalHits: 1234}, nil
		}
		attr := facets[0]
		dist, _ := json.Marshal(map[string]map[string]int64{attr: {"x": 10, "y": 5}})
		return &meilisearch.SearchResponse{FacetDistribution: dist, EstimatedTotalHits: 99}, nil
	}

	reqs := []FacetReq{{Attr: "seniority", Filter: "F_sen"}, {Attr: "category", Filter: "F_cat"}}
	got, err := disjunctiveFacetCounts(context.Background(), "go", reqs, "FULL", searcher)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	// Total comes from the total-only query (the full filter), not any facet subtotal.
	if got.Total != 1234 {
		t.Errorf("Total = %d, want 1234", got.Total)
	}
	// Each facet carries its own disjunctive distribution.
	if got.Facets["seniority"]["x"] != 10 || got.Facets["category"]["y"] != 5 {
		t.Errorf("Facets = %v", got.Facets)
	}
	// One query per facet (facets=[attr], reduced filter) + one total-only (full filter).
	if len(calls) != 3 {
		t.Fatalf("calls = %d, want 3", len(calls))
	}
	var total, sen *call
	for i := range calls {
		switch {
		case len(calls[i].facets) == 0:
			total = &calls[i]
		case calls[i].facets[0] == "seniority":
			sen = &calls[i]
		}
	}
	if total == nil || total.filter != "FULL" {
		t.Errorf("total query filter = %v, want FULL with no facets", total)
	}
	if sen == nil || sen.filter != "F_sen" {
		t.Errorf("seniority query filter = %v, want F_sen", sen)
	}
}

func TestDisjunctiveFacetCounts_EuropeCountIncludesUK(t *testing.T) {
	// The Europe count must be what choosing Europe returns — the documents in
	// Europe OR the UK, counted once — not the bare "eu" bucket of the distribution.
	var mu sync.Mutex
	var widened any
	searcher := func(_ context.Context, _ string, filter any, facets []string) (*meilisearch.SearchResponse, error) {
		if len(facets) > 0 {
			dist, _ := json.Marshal(map[string]map[string]int64{"regions": {"eu": 100, "uk": 30, "latam": 7}})
			return &meilisearch.SearchResponse{FacetDistribution: dist}, nil
		}
		if groups, ok := filter.([][]string); ok && len(groups) == 2 {
			mu.Lock()
			widened = filter
			mu.Unlock()
			return &meilisearch.SearchResponse{EstimatedTotalHits: 120}, nil
		}
		return &meilisearch.SearchResponse{EstimatedTotalHits: 500}, nil
	}

	reqs := []FacetReq{{Attr: "regions", Filter: [][]string{{`work_mode = "remote"`}}}}
	got, err := disjunctiveFacetCounts(context.Background(), "", reqs, nil, searcher)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if got.Facets["regions"]["eu"] != 120 {
		t.Errorf("eu = %d, want 120 (Europe OR UK, overlap counted once)", got.Facets["regions"]["eu"])
	}
	if got.Facets["regions"]["uk"] != 30 || got.Facets["regions"]["latam"] != 7 {
		t.Errorf("other regions changed: %v", got.Facets["regions"])
	}
	want := [][]string{{`work_mode = "remote"`}, {`regions IN ["eu", "uk"]`}}
	if !reflect.DeepEqual(widened, want) {
		t.Errorf("widened count filter = %v, want %v", widened, want)
	}
}

func TestDisjunctiveFacetCounts_ErrorPropagates(t *testing.T) {
	searcher := func(context.Context, string, any, []string) (*meilisearch.SearchResponse, error) {
		return nil, errors.New("boom")
	}
	if _, err := disjunctiveFacetCounts(context.Background(), "", []FacetReq{{Attr: "a", Filter: "f"}}, "full", searcher); err == nil {
		t.Fatal("expected error to propagate")
	}
}

// A cancelled context must re-raise the context sentinel, not the SDK's opaque
// communication error, so a client disconnect is not misreported upstream as a
// fault — mirrors Search's classification (client.go).
func TestDisjunctiveFacetCounts_CancelledContextReRaisesSentinel(t *testing.T) {
	searcher := func(context.Context, string, any, []string) (*meilisearch.SearchResponse, error) {
		return nil, errors.New("meilisearch: could not perform request: context canceled")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := disjunctiveFacetCounts(ctx, "", []FacetReq{{Attr: "a", Filter: "f"}}, "full", searcher)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
}

// A Meili 400 (malformed filter) must map to ErrBadQuery so the handler can render
// it as a client mistake and skip Sentry.
func TestDisjunctiveFacetCounts_BadRequestMapsToErrBadQuery(t *testing.T) {
	searcher := func(context.Context, string, any, []string) (*meilisearch.SearchResponse, error) {
		return nil, &meilisearch.Error{StatusCode: http.StatusBadRequest}
	}

	_, err := disjunctiveFacetCounts(context.Background(), "", []FacetReq{{Attr: "a", Filter: "f"}}, "full", searcher)
	if !errors.Is(err, ErrBadQuery) {
		t.Fatalf("err = %v, want ErrBadQuery", err)
	}
}

// A cancelled context must re-raise the context sentinel — mirrors Search
// (client.go).
func TestFacetCounts_CancelledContextReRaisesSentinel(t *testing.T) {
	c := &Client{facet: fakeSearchIndex{searchFn: func(context.Context, string, *meilisearch.SearchRequest) (*meilisearch.SearchResponse, error) {
		return nil, errors.New("meilisearch: could not perform request: context canceled")
	}}}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := c.FacetCounts(ctx, FacetParams{})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
}

// A Meili 400 (malformed filter) must map to ErrBadQuery so the handler can render
// it as a client mistake and skip Sentry.
func TestFacetCounts_BadRequestMapsToErrBadQuery(t *testing.T) {
	c := &Client{facet: fakeSearchIndex{searchFn: func(context.Context, string, *meilisearch.SearchRequest) (*meilisearch.SearchResponse, error) {
		return nil, &meilisearch.Error{StatusCode: http.StatusBadRequest}
	}}}

	_, err := c.FacetCounts(context.Background(), FacetParams{})
	if !errors.Is(err, ErrBadQuery) {
		t.Fatalf("err = %v, want ErrBadQuery", err)
	}
}

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
	f := facetSettings().Faceting
	if f == nil {
		t.Fatal("facetSettings().Faceting is nil; expected maxValuesPerFacet override")
	}
	if f.MaxValuesPerFacet <= 100 {
		t.Errorf("MaxValuesPerFacet = %d, want > 100", f.MaxValuesPerFacet)
	}
	// The cap must keep the busiest values, not the alphabetically-first ones, or a
	// high-cardinality facet (cities) drops popular values that sort late.
	if got := f.SortFacetValuesBy["*"]; got != meilisearch.SortFacetTypeCount {
		t.Errorf(`SortFacetValuesBy["*"] = %q, want %q`, got, meilisearch.SortFacetTypeCount)
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

// DistinctCount must page (page 1, hitsPerPage 0) and never ask for facets or a
// limit: Meilisearch's limit-0 estimatedTotalHits ignores `distinct`, and
// `distinct` beside `facets` collapses every facet count to 1. totalHits is read.
func TestDistinctCount_PagesWithoutLimitOrFacets(t *testing.T) {
	var got *meilisearch.SearchRequest
	var gotQuery string
	c := &Client{facet: fakeSearchIndex{searchFn: func(_ context.Context, q string, r *meilisearch.SearchRequest) (*meilisearch.SearchResponse, error) {
		got, gotQuery = r, q
		return &meilisearch.SearchResponse{TotalHits: 1540, EstimatedTotalHits: 9999}, nil
	}}}

	n, err := c.DistinctCount(context.Background(), "title:golang", "f", "company_slug")
	if err != nil {
		t.Fatalf("DistinctCount: %v", err)
	}
	if n != 1540 {
		t.Errorf("n = %d, want totalHits 1540", n)
	}
	if got.Distinct != "company_slug" || got.Page != 1 || got.HitsPerPage == nil || *got.HitsPerPage != 0 {
		t.Errorf("request = distinct %q page %d hitsPerPage %v, want company_slug/1/0", got.Distinct, got.Page, got.HitsPerPage)
	}
	if got.Limit != 0 || got.Offset != 0 || len(got.Facets) != 0 {
		t.Errorf("request carries limit %d offset %d facets %v, want none", got.Limit, got.Offset, got.Facets)
	}
	if got.Filter != "f" || gotQuery != "golang" || len(got.AttributesToSearchOn) != 1 || got.AttributesToSearchOn[0] != "title" {
		t.Errorf("filter %v query %q fields %v not carried through", got.Filter, gotQuery, got.AttributesToSearchOn)
	}
}

// An OR query runs federated, where a per-query distinct would count a company
// matched by two alternatives twice — refused as a bad query, never over-counted.
func TestDistinctCount_RefusesORQuery(t *testing.T) {
	c := &Client{facet: fakeSearchIndex{searchFn: func(context.Context, string, *meilisearch.SearchRequest) (*meilisearch.SearchResponse, error) {
		t.Fatal("an OR query must not reach the engine")
		return nil, nil
	}}}
	_, err := c.DistinctCount(context.Background(), "go OR rust", nil, "company_slug")
	if !errors.Is(err, ErrBadQuery) {
		t.Fatalf("err = %v, want ErrBadQuery", err)
	}
}
