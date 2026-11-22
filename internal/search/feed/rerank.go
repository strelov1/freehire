package feed

import (
	"context"
	"errors"
	"sort"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"

	"github.com/strelov1/freehire/internal/candidate/jevrank"
	"github.com/strelov1/freehire/internal/platform/cache"
)

// Fallback reasons Rerank and the handler report; "" means the page is in fit order.
const (
	FallbackError   = "error"
	FallbackTimeout = "timeout"
	FallbackPartial = "partial"
)

const (
	// DefaultBudget bounds the wait for Jev on one page (AD-11).
	DefaultBudget = 2 * time.Second
	// DefaultTTL is how long a score stays cached.
	DefaultTTL = 7 * 24 * time.Hour
)

var (
	rerankDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "freehire_me_feed_rerank_duration_seconds",
		Help:    "Feed page re-rank latency by outcome (fit, error, timeout, partial).",
		Buckets: prometheus.DefBuckets,
	}, []string{"outcome"})
	rerankJevCalls = promauto.NewCounter(prometheus.CounterOpts{
		Name: "freehire_me_feed_rerank_jev_calls_total", Help: "Jobs sent to Jev by the feed re-rank.",
	})
	rerankCache = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "freehire_me_feed_rerank_cache_total", Help: "Feed re-rank score cache lookups by result (hit, miss).",
	}, []string{"result"})
	rerankFallbacks = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "freehire_me_feed_rerank_fallbacks_total", Help: "Pages that kept the skill order, by reason.",
	}, []string{"reason"})
)

// CountFallback records a page kept in skill order for a reason decided outside Rerank.
func CountFallback(reason string) { rerankFallbacks.WithLabelValues(reason).Inc() }

// Scorer is jevrank.Ranker's Rank.
type Scorer interface {
	Rank(ctx context.Context, cvText string, jobs []jevrank.Job) []jevrank.Result
}

// Reranker reorders a page by the Jev fit score, caching scores per user, CV version and
// job. A nil *Reranker is disabled; a nil cache is always a miss.
type Reranker struct {
	scorer Scorer
	cache  cache.Cache
	budget time.Duration
	ttl    time.Duration
}

func NewReranker(s Scorer, c cache.Cache, budget, ttl time.Duration) *Reranker {
	return &Reranker{scorer: s, cache: c, budget: budget, ttl: ttl}
}

func fitKey(userID int64, cvVersion, slug string) string {
	return "feedfit:v1:" + strconv.FormatInt(userID, 10) + ":" + cvVersion + ":" + slug
}

// Rerank returns the slugs of jobs (given in skill order) best fit first, ties in skill
// order. fallback is "" when every job had a score; otherwise order is nil and the caller
// keeps the skill order. Scores obtained before a failure are still cached.
func (r *Reranker) Rerank(ctx context.Context, userID int64, cvVersion, cvText string, jobs []jevrank.Job) (order []string, fallback string) {
	start := time.Now()
	defer func() {
		outcome := fallback
		if outcome == "" {
			outcome = "fit"
		} else {
			CountFallback(fallback)
		}
		rerankDuration.WithLabelValues(outcome).Observe(time.Since(start).Seconds())
	}()

	scores := make(map[string]float64, len(jobs))
	var misses []jevrank.Job
	for _, j := range jobs {
		if s, ok := r.cached(ctx, fitKey(userID, cvVersion, j.Slug)); ok {
			rerankCache.WithLabelValues("hit").Inc()
			scores[j.Slug] = s
			continue
		}
		rerankCache.WithLabelValues("miss").Inc()
		misses = append(misses, j)
	}

	if len(misses) > 0 {
		bctx, cancel := context.WithTimeout(ctx, r.budget)
		defer cancel()
		rerankJevCalls.Add(float64(len(misses)))
		got := 0
		for _, res := range r.scorer.Rank(bctx, cvText, misses) {
			if !res.OK {
				continue
			}
			got++
			scores[res.Slug] = res.Score
			r.store(ctx, fitKey(userID, cvVersion, res.Slug), res.Score)
		}
		switch {
		case got == len(misses):
		case errors.Is(bctx.Err(), context.DeadlineExceeded):
			return nil, FallbackTimeout
		case got == 0:
			return nil, FallbackError
		default:
			return nil, FallbackPartial
		}
	}

	order = make([]string, len(jobs))
	for i, j := range jobs {
		order[i] = j.Slug
	}
	sort.SliceStable(order, func(a, b int) bool { return scores[order[a]] > scores[order[b]] })
	return order, ""
}

func (r *Reranker) cached(ctx context.Context, key string) (float64, bool) {
	if r.cache == nil {
		return 0, false
	}
	b, ok, err := r.cache.Get(ctx, key)
	if err != nil || !ok {
		return 0, false
	}
	s, err := strconv.ParseFloat(string(b), 64)
	return s, err == nil
}

// store ignores errors: a failed write only costs a later Jev call.
func (r *Reranker) store(ctx context.Context, key string, score float64) {
	if r.cache != nil {
		_ = r.cache.Set(ctx, key, []byte(strconv.FormatFloat(score, 'g', -1, 64)), r.ttl)
	}
}
