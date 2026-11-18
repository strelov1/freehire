package search

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/meilisearch/meilisearch-go"
)

type fakeEngine struct {
	mu      sync.Mutex
	filters []any
	calls   atomic.Int64
	facets  func(ctx context.Context, filter any, facets []string) (FacetEngineAnswer, error)
	distinc func() (int64, error)
}

func (f *fakeEngine) Facets(ctx context.Context, filter any, facets []string) (FacetEngineAnswer, error) {
	f.calls.Add(1)
	f.mu.Lock()
	f.filters = append(f.filters, filter)
	f.mu.Unlock()
	return f.facets(ctx, filter, facets)
}

func (f *fakeEngine) DistinctCount(context.Context, any, string) (int64, error) {
	f.calls.Add(1)
	return f.distinc()
}

func answer(total int64) func(context.Context, any, []string) (FacetEngineAnswer, error) {
	return func(context.Context, any, []string) (FacetEngineAnswer, error) {
		return FacetEngineAnswer{Total: total, Distribution: map[string]map[string]int64{"source": {"a": total}}}, nil
	}
}

// meiliClient answers every search with total 7 and counts calls.
func meiliClient(calls *atomic.Int64, mode *atomic.Value, eng FacetEngine, opts ...Option) *Client {
	c := &Client{facet: fakeSearchIndex{searchFn: func(context.Context, string, *meilisearch.SearchRequest) (*meilisearch.SearchResponse, error) {
		calls.Add(1)
		return &meilisearch.SearchResponse{EstimatedTotalHits: 7, TotalHits: 5}, nil
	}}}
	WithFacetEngine(eng, func() FacetMode { return mode.Load().(FacetMode) })(c)
	for _, o := range opts {
		o(c)
	}
	return c
}

func modeOf(m FacetMode) *atomic.Value {
	v := &atomic.Value{}
	v.Store(m)
	return v
}

func TestParseFacetMode(t *testing.T) {
	for _, s := range []string{"meili", "shadow", "facetd"} {
		if m, err := ParseFacetMode(s); err != nil || string(m) != s {
			t.Errorf("%s: %v %v", s, m, err)
		}
	}
	if _, err := ParseFacetMode("x"); err == nil {
		t.Error("want error")
	}
}

func TestRouteMeiliModeNeverCallsEngine(t *testing.T) {
	var mc atomic.Int64
	eng := &fakeEngine{facets: answer(1)}
	c := meiliClient(&mc, modeOf(FacetModeMeili), eng)
	got, err := c.FacetCounts(context.Background(), FacetParams{Facets: []string{"source"}})
	if err != nil || got.Total != 7 || eng.calls.Load() != 0 || mc.Load() != 1 {
		t.Fatalf("got %+v err %v engine %d meili %d", got, err, eng.calls.Load(), mc.Load())
	}
}

func TestRouteNoEngineIsMeili(t *testing.T) {
	var mc atomic.Int64
	c := meiliClient(&mc, modeOf(FacetModeFacetd), nil)
	if got, err := c.FacetCounts(context.Background(), FacetParams{}); err != nil || got.Total != 7 {
		t.Fatalf("got %+v err %v", got, err)
	}
}

func TestRouteFacetdAnswersFromEngineWithoutOpenJobsClause(t *testing.T) {
	var mc atomic.Int64
	eng := &fakeEngine{facets: answer(42), distinc: func() (int64, error) { return 9, nil }}
	c := meiliClient(&mc, modeOf(FacetModeFacetd), eng)
	filter := [][]string{{`work_mode = "remote"`}}
	got, err := c.FacetCounts(context.Background(), FacetParams{Filter: filter, Facets: []string{"source"}})
	if err != nil || got.Total != 42 || got.Facets["source"]["a"] != 42 || mc.Load() != 0 {
		t.Fatalf("got %+v err %v meili %d", got, err, mc.Load())
	}
	if !reflect.DeepEqual(eng.filters[0], filter) {
		t.Errorf("engine filter = %v, want caller's filter without the open-jobs clause", eng.filters[0])
	}
	if n, err := c.DistinctCount(context.Background(), "", filter, "company_slug"); err != nil || n != 9 || mc.Load() != 0 {
		t.Fatalf("distinct %d %v meili %d", n, err, mc.Load())
	}
}

func TestRouteFacetdFallsBack(t *testing.T) {
	cases := map[string]struct {
		err    error
		slow   bool
		reason string
	}{
		"error":       {err: errors.New("boom"), reason: "error"},
		"unsupported": {err: ErrEngineUnsupported, reason: "unsupported"},
		"not_ready":   {err: ErrEngineNotReady, reason: "not_ready"},
		"timeout":     {slow: true, reason: "timeout"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			var mc atomic.Int64
			eng := &fakeEngine{facets: func(ctx context.Context, _ any, _ []string) (FacetEngineAnswer, error) {
				if tc.slow {
					<-ctx.Done()
					return FacetEngineAnswer{}, ctx.Err()
				}
				return FacetEngineAnswer{}, tc.err
			}, distinc: func() (int64, error) { return 0, tc.err }}
			c := meiliClient(&mc, modeOf(FacetModeFacetd), eng, WithFacetEngineTimeout(10*time.Millisecond))
			got, err := c.FacetCounts(context.Background(), FacetParams{})
			if err != nil || got.Total != 7 || mc.Load() != 1 {
				t.Fatalf("got %+v err %v meili %d", got, err, mc.Load())
			}
			if c.RoutingCounters().Fallbacks[tc.reason] != 1 {
				t.Errorf("counters = %+v", c.RoutingCounters())
			}
			if !tc.slow {
				if n, err := c.DistinctCount(context.Background(), "", nil, "company_slug"); err != nil || n != 5 {
					t.Errorf("distinct fallback %d %v", n, err)
				}
			}
		})
	}
}

func TestRouteFacetdCancelledCallerSkipsMeili(t *testing.T) {
	var mc atomic.Int64
	eng := &fakeEngine{facets: func(ctx context.Context, _ any, _ []string) (FacetEngineAnswer, error) {
		return FacetEngineAnswer{}, ctx.Err()
	}}
	c := meiliClient(&mc, modeOf(FacetModeFacetd), eng)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := c.FacetCounts(ctx, FacetParams{}); !errors.Is(err, context.Canceled) || mc.Load() != 0 {
		t.Fatalf("err %v meili %d", err, mc.Load())
	}
}

func TestRouteQueryTextAlwaysMeili(t *testing.T) {
	var mc atomic.Int64
	eng := &fakeEngine{facets: answer(1)}
	c := meiliClient(&mc, modeOf(FacetModeFacetd), eng)
	if got, err := c.FacetCounts(context.Background(), FacetParams{Query: "golang"}); err != nil || got.Total != 7 || eng.calls.Load() != 0 {
		t.Fatalf("got %+v err %v", got, err)
	}
}

func TestRouteModeFlipTakesEffectNextRequest(t *testing.T) {
	var mc atomic.Int64
	mode := modeOf(FacetModeMeili)
	eng := &fakeEngine{facets: answer(42)}
	c := meiliClient(&mc, mode, eng)
	if got, _ := c.FacetCounts(context.Background(), FacetParams{}); got.Total != 7 {
		t.Fatalf("meili total %d", got.Total)
	}
	mode.Store(FacetModeFacetd)
	if got, _ := c.FacetCounts(context.Background(), FacetParams{}); got.Total != 42 {
		t.Fatalf("facetd total %d", got.Total)
	}
}

func waitFor(t *testing.T, ok func() bool) {
	t.Helper()
	for i := 0; i < 200 && !ok(); i++ {
		time.Sleep(5 * time.Millisecond)
	}
	if !ok() {
		t.Fatal("condition not reached")
	}
}

func TestRouteShadowReturnsMeiliAndCountsDifferences(t *testing.T) {
	var mc atomic.Int64
	eng := &fakeEngine{facets: func(context.Context, any, []string) (FacetEngineAnswer, error) {
		return FacetEngineAnswer{Total: 7, Distribution: map[string]map[string]int64{"source": {"a": 1}}}, nil
	}}
	c := meiliClient(&mc, modeOf(FacetModeShadow), eng)
	// Meili answers total 7 with no distribution; the engine adds source=a:1.
	got, err := c.FacetCounts(context.Background(), FacetParams{Facets: []string{"source"}})
	if err != nil || got.Total != 7 || got.Facets["source"] != nil {
		t.Fatalf("got %+v err %v: shadow must return Meilisearch's answer", got, err)
	}
	waitFor(t, func() bool { return c.RoutingCounters().ShadowDiffs == 1 })

	eng.facets = func(context.Context, any, []string) (FacetEngineAnswer, error) {
		return FacetEngineAnswer{Total: 7}, nil
	}
	if _, err := c.FacetCounts(context.Background(), FacetParams{}); err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool { return eng.calls.Load() == 2 })
	time.Sleep(20 * time.Millisecond)
	if d := c.RoutingCounters().ShadowDiffs; d != 1 {
		t.Fatalf("diffs = %d, want 1 (agreeing answers add none)", d)
	}
}

func TestRouteShadowEngineErrorIsNotADifference(t *testing.T) {
	var mc atomic.Int64
	eng := &fakeEngine{facets: func(context.Context, any, []string) (FacetEngineAnswer, error) {
		return FacetEngineAnswer{}, ErrEngineNotReady
	}}
	c := meiliClient(&mc, modeOf(FacetModeShadow), eng)
	if _, err := c.FacetCounts(context.Background(), FacetParams{}); err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool { return c.RoutingCounters().ShadowErrors == 1 })
	if d := c.RoutingCounters().ShadowDiffs; d != 0 {
		t.Fatalf("diffs = %d, want 0: an engine failure must not count against the parity budget", d)
	}
}

func TestRouteShadowSkipsWhenFull(t *testing.T) {
	var mc atomic.Int64
	release := make(chan struct{})
	eng := &fakeEngine{facets: func(ctx context.Context, _ any, _ []string) (FacetEngineAnswer, error) {
		select {
		case <-release:
		case <-ctx.Done():
		}
		return FacetEngineAnswer{Total: 7}, nil
	}}
	c := meiliClient(&mc, modeOf(FacetModeShadow), eng, WithFacetEngineTimeout(5*time.Second))
	for i := 0; i < shadowConcurrency+3; i++ {
		if _, err := c.FacetCounts(context.Background(), FacetParams{}); err != nil {
			t.Fatal(err)
		}
	}
	if s := c.RoutingCounters().ShadowSkipped; s != 3 {
		t.Errorf("skipped = %d, want 3", s)
	}
	close(release)
}

func TestRouteFacetdRegionWideningAndDisjunctive(t *testing.T) {
	var mc atomic.Int64
	eng := &fakeEngine{facets: func(_ context.Context, filter any, facets []string) (FacetEngineAnswer, error) {
		if len(facets) > 0 {
			return FacetEngineAnswer{Distribution: map[string]map[string]int64{facets[0]: {"eu": 100, "uk": 30}}}, nil
		}
		if g, ok := filter.([][]string); ok && len(g) == 1 && g[0][0] == `regions IN ["eu", "uk"]` {
			return FacetEngineAnswer{Total: 120}, nil
		}
		return FacetEngineAnswer{Total: 500}, nil
	}}
	c := meiliClient(&mc, modeOf(FacetModeFacetd), eng)
	got, err := c.DisjunctiveFacetCounts(context.Background(), "", []FacetReq{{Attr: "regions"}}, nil)
	if err != nil || got.Total != 500 || got.Facets["regions"]["eu"] != 120 || got.Facets["regions"]["uk"] != 30 || mc.Load() != 0 {
		t.Fatalf("got %+v err %v meili %d", got, err, mc.Load())
	}
}

func TestFailureLogIsSampled(t *testing.T) {
	r := &facetRouter{}
	if !r.logFailure() || r.logFailure() {
		t.Fatal("want the first failure logged and the next one within the interval suppressed")
	}
}
