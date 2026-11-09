package search

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Router metrics, on the default registry like the rest of hire-api's. Labels are
// closed sets (engine, mode, kind, reason), so the series count is bounded.
var (
	facetRequests = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "freehire_search_facet_requests_total", Help: "Routed facet requests by the engine that answered, mode and kind (facets|distinct).",
	}, []string{"engine", "mode", "kind"})
	facetLatency = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name: "freehire_search_facet_engine_seconds", Help: "Latency of one engine call (meili|facetd), failures included, shadow calls included.",
		Buckets: []float64{0.002, 0.005, 0.01, 0.025, 0.05, 0.1, 0.15, 0.25, 0.5, 1, 2.5},
	}, []string{"engine", "kind"})
	facetFallbacks = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "freehire_search_facet_fallbacks_total", Help: "facetd-mode requests answered by Meilisearch, by reason (error|timeout|unsupported|not_ready).",
	}, []string{"reason", "kind"})
	facetShadowDiffs = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "freehire_search_facet_shadow_diffs_total", Help: "Shadow comparisons where the engine answered differently from Meilisearch.",
	}, []string{"kind"})
	facetShadowErrors = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "freehire_search_facet_shadow_errors_total", Help: "Shadow engine calls that failed (not a semantic difference).",
	}, []string{"kind"})
	facetShadowSkipped = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "freehire_search_facet_shadow_skipped_total", Help: "Shadow comparisons skipped because shadowConcurrency calls were already in flight.",
	}, []string{"kind"})
)

// timed runs fn and records its latency under engine.
func timed[T any](engine, kind string, fn func() (T, error)) (T, error) {
	start := time.Now()
	v, err := fn()
	facetLatency.WithLabelValues(engine, kind).Observe(time.Since(start).Seconds())
	return v, err
}

// FacetEngine is a second engine for facet requests without free text (facetd).
// filter is the caller's filter WITHOUT the open-jobs clause: the engine hides
// closed and duplicate jobs itself. An answer's maps belong to the caller.
type FacetEngine interface {
	Facets(ctx context.Context, filter any, facets []string) (FacetEngineAnswer, error)
	DistinctCount(ctx context.Context, filter any, attr string) (int64, error)
}

// FacetEngineAnswer is a FacetEngine's reply to one facet request.
type FacetEngineAnswer struct {
	Total        int64
	Distribution map[string]map[string]int64 // attr -> value -> count, at most maxValuesPerFacet values, highest first
	Stats        map[string]FacetStat
}

var (
	ErrEngineUnsupported = errors.New("search: facet engine cannot answer this request")
	ErrEngineNotReady    = errors.New("search: facet engine not ready")
	// ErrFacetdRejected is facetd's 400 to a /write: a document in the batch is unreadable.
	// Retrying the same batch cannot succeed; facetd itself is healthy.
	ErrFacetdRejected = errors.New("search: facetd rejected the write batch")
)

// FacetMode says which engine answers a facet request that has no query text.
type FacetMode string

const (
	// FacetModeMeili answers from Meilisearch only (the default).
	FacetModeMeili FacetMode = "meili"
	// FacetModeShadow answers from Meilisearch and compares the engine in the background.
	FacetModeShadow FacetMode = "shadow"
	// FacetModeFacetd answers from the engine and falls back to Meilisearch on any error.
	FacetModeFacetd FacetMode = "facetd"
)

// ParseFacetMode reads "meili", "shadow" or "facetd".
func ParseFacetMode(s string) (FacetMode, error) {
	switch m := FacetMode(strings.ToLower(strings.TrimSpace(s))); m {
	case FacetModeMeili, FacetModeShadow, FacetModeFacetd:
		return m, nil
	}
	return "", fmt.Errorf("search: unknown facet mode %q (want meili, shadow or facetd)", s)
}

const (
	defaultEngineTimeout = 150 * time.Millisecond
	shadowConcurrency    = 8
	// failureLogEvery bounds the fallback and shadow-failure warnings to one per interval:
	// a common unsupported filter would otherwise log on every request. The metrics count all.
	failureLogEvery = 10 * time.Second
)

// RoutingCounters is a snapshot of the router's event counts.
type RoutingCounters struct {
	ShadowDiffs   int64
	ShadowErrors  int64 // engine failed in shadow: not a semantic difference, kept apart from ShadowDiffs
	ShadowSkipped int64
	// Fallbacks counts facetd-mode requests answered by Meilisearch, by reason:
	// error, timeout, unsupported, not_ready.
	Fallbacks map[string]int64
}

type facetRouter struct {
	engine  FacetEngine
	mode    func() FacetMode
	timeout time.Duration
	sem     chan struct{}

	shadowDiffs, shadowErrors, shadowSkipped      atomic.Int64
	fbError, fbTimeout, fbUnsupported, fbNotReady atomic.Int64
	lastFailureLog                                atomic.Int64 // unix nanos
}

// logFailure reports whether this failure may be logged: at most one per failureLogEvery.
func (r *facetRouter) logFailure() bool {
	now, last := time.Now().UnixNano(), r.lastFailureLog.Load()
	return now-last >= int64(failureLogEvery) && r.lastFailureLog.CompareAndSwap(last, now)
}

// WithFacetEngine routes facet requests without query text through engine according
// to mode, which is read on every request so it can change at runtime. A nil mode
// means meili.
func WithFacetEngine(engine FacetEngine, mode func() FacetMode) Option {
	return func(c *Client) {
		if engine == nil {
			return
		}
		if c.router == nil {
			c.router = &facetRouter{timeout: defaultEngineTimeout, sem: make(chan struct{}, shadowConcurrency)}
		}
		c.router.engine, c.router.mode = engine, mode
	}
}

// WithFacetEngineTimeout bounds each engine call (default 150ms). Apply after WithFacetEngine.
func WithFacetEngineTimeout(d time.Duration) Option {
	return func(c *Client) {
		if c.router != nil && d > 0 {
			c.router.timeout = d
		}
	}
}

// RoutingCounters reports the router's counts; zero without an engine.
func (c *Client) RoutingCounters() RoutingCounters {
	out := RoutingCounters{Fallbacks: map[string]int64{}}
	if r := c.router; r != nil {
		out.ShadowDiffs, out.ShadowErrors, out.ShadowSkipped = r.shadowDiffs.Load(), r.shadowErrors.Load(), r.shadowSkipped.Load()
		out.Fallbacks["error"], out.Fallbacks["timeout"] = r.fbError.Load(), r.fbTimeout.Load()
		out.Fallbacks["unsupported"], out.Fallbacks["not_ready"] = r.fbUnsupported.Load(), r.fbNotReady.Load()
	}
	return out
}

// countMeiliOnly counts a request that stayed on Meilisearch while an engine is wired,
// so a rollout's share is readable. Text queries are not counted: they never route.
func (c *Client) countMeiliOnly(query, kind string) {
	if c.router != nil && strings.TrimSpace(query) == "" {
		facetRequests.WithLabelValues("meili", string(FacetModeMeili), kind).Inc()
	}
}

// routeMode is the mode for one request: meili unless an engine is set, the mode asks
// for it, and there is no query text. Read once per request, so a flip mid-request
// cannot send it down a mixed path.
func (c *Client) routeMode(query string) FacetMode {
	r := c.router
	if r == nil || r.mode == nil || strings.TrimSpace(query) != "" {
		return FacetModeMeili
	}
	if m := r.mode(); m == FacetModeShadow || m == FacetModeFacetd {
		return m
	}
	return FacetModeMeili
}

// routeRequest answers one raw call per the current mode. clone snapshots a
// Meilisearch answer before the caller mutates it; diff names the first
// difference between the Meilisearch and engine answers ("" when equal).
func routeRequest[T any](ctx context.Context, r *facetRouter, mode FacetMode, kind string, filter any,
	engine func(context.Context) (T, error), meili func() (T, error),
	clone func(T) T, diff func(meili, eng T) string) (T, error) {

	if mode == FacetModeFacetd {
		ectx, cancel := context.WithTimeout(ctx, r.timeout)
		ans, err := timed("facetd", kind, func() (T, error) { return engine(ectx) })
		cancel()
		if err == nil {
			facetRequests.WithLabelValues("facetd", string(mode), kind).Inc()
			return ans, nil
		}
		if ctx.Err() != nil { // the caller left; do not start a Meilisearch call for nobody
			var zero T
			return zero, ctx.Err()
		}
		r.fallback(kind, filter, err)
		facetRequests.WithLabelValues("meili", string(mode), kind).Inc()
		return timed("meili", kind, meili)
	}

	// shadow
	ans, err := timed("meili", kind, meili)
	if err != nil {
		return ans, err
	}
	facetRequests.WithLabelValues("meili", string(mode), kind).Inc()
	select {
	case r.sem <- struct{}{}:
		snap := clone(ans)
		go func() {
			defer func() { <-r.sem }()
			sctx, cancel := context.WithTimeout(context.Background(), r.timeout)
			defer cancel()
			eng, err := timed("facetd", kind, func() (T, error) { return engine(sctx) })
			if err != nil {
				if r.logFailure() {
					slog.Warn("facetd shadow: engine failed (sampled, see freehire_search_facet_shadow_errors_total)", "kind", kind, "filter", filter, "err", err)
				}
				r.shadowErrors.Add(1)
				facetShadowErrors.WithLabelValues(kind).Inc()
				return
			}
			if d := diff(snap, eng); d != "" {
				r.shadowDiffs.Add(1)
				facetShadowDiffs.WithLabelValues(kind).Inc()
				slog.Warn("facetd shadow: answers differ", "kind", kind, "filter", filter, "first_difference", d)
			}
		}()
	default:
		r.shadowSkipped.Add(1)
		facetShadowSkipped.WithLabelValues(kind).Inc()
	}
	return ans, nil
}

func (r *facetRouter) fallback(kind string, filter any, err error) {
	reason, ctr := "error", &r.fbError
	switch {
	case errors.Is(err, ErrEngineUnsupported):
		reason, ctr = "unsupported", &r.fbUnsupported
	case errors.Is(err, ErrEngineNotReady):
		reason, ctr = "not_ready", &r.fbNotReady
	case errors.Is(err, context.DeadlineExceeded):
		reason, ctr = "timeout", &r.fbTimeout
	}
	ctr.Add(1)
	facetFallbacks.WithLabelValues(reason, kind).Inc()
	if r.logFailure() {
		slog.Warn("facetd fallback to meilisearch (sampled, see freehire_search_facet_fallbacks_total)", "kind", kind, "reason", reason, "filter", filter, "err", err)
	}
}

func cloneRaw(a rawFacets) rawFacets {
	out := rawFacets{Total: a.Total, Stats: maps.Clone(a.Stats)}
	if a.Distribution != nil {
		out.Distribution = make(map[string]map[string]int64, len(a.Distribution))
		for k, v := range a.Distribution {
			out.Distribution[k] = maps.Clone(v)
		}
	}
	return out
}

func diffCount(m, e int64) string {
	if m != e {
		return fmt.Sprintf("count: meili=%d engine=%d", m, e)
	}
	return ""
}

// foldCounts sums the counts of keys that fold to the same lowercase string:
// Meilisearch matches strings case-insensitively and reports whichever spelling it saw first.
func foldCounts(m map[string]int64) map[string]int64 {
	out := make(map[string]int64, len(m))
	for k, v := range m {
		out[strings.ToLower(k)] += v
	}
	return out
}

// diffDistribution compares one facet's values, case-folded. At the
// maxValuesPerFacet cap the values tied on Meilisearch's smallest count are
// not compared: which of them made the cut follows Meilisearch's internal tree
// order and cannot be reproduced. Higher counts and the value count still must match.
func diffDistribution(attr string, m, e map[string]int64) string {
	mv, ev := foldCounts(m), foldCounts(e)
	var tie int64 = -1
	if len(m) == maxValuesPerFacet {
		tie = 1<<63 - 1
		for _, c := range mv {
			tie = min(tie, c)
		}
		if len(e) != maxValuesPerFacet {
			return fmt.Sprintf("%s: values: meili=%d engine=%d", attr, len(m), len(e))
		}
	}
	for _, val := range sortedKeys(mv, ev) {
		if tie >= 0 && (mv[val] == tie && ev[val] <= tie || mv[val] == 0 && ev[val] == tie) {
			continue
		}
		if mv[val] != ev[val] {
			return fmt.Sprintf("%s=%s: meili=%d engine=%d", attr, val, mv[val], ev[val])
		}
	}
	return ""
}

// diffRaw names the first difference, comparing in a stable key order. A facet or
// value absent on one side equals a zero count there.
func diffRaw(m, e rawFacets) string {
	if m.Total != e.Total {
		return fmt.Sprintf("total: meili=%d engine=%d", m.Total, e.Total)
	}
	for _, attr := range sortedKeys(m.Distribution, e.Distribution) {
		if d := diffDistribution(attr, m.Distribution[attr], e.Distribution[attr]); d != "" {
			return d
		}
	}
	for _, attr := range sortedKeys(m.Stats, e.Stats) {
		if ms, es := m.Stats[attr], e.Stats[attr]; ms != es {
			return fmt.Sprintf("stats %s: meili=%v engine=%v", attr, ms, es)
		}
	}
	return ""
}

func sortedKeys[V any](a, b map[string]V) []string {
	set := map[string]struct{}{}
	for k := range a {
		set[k] = struct{}{}
	}
	for k := range b {
		set[k] = struct{}{}
	}
	keys := make([]string, 0, len(set))
	for k := range set {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
