package feed

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/strelov1/freehire/internal/candidate/jevrank"
)

type mapCache map[string][]byte

func (m mapCache) Get(_ context.Context, k string) ([]byte, bool, error) {
	v, ok := m[k]
	return v, ok, nil
}

func (m mapCache) Set(_ context.Context, k string, v []byte, _ time.Duration) error {
	m[k] = v
	return nil
}

type stubScorer struct {
	scores map[string]float64 // absent slug = failure
	slow   bool
	calls  int
	sent   int
}

func (s *stubScorer) Rank(ctx context.Context, _ string, jobs []jevrank.Job) []jevrank.Result {
	s.calls++
	s.sent += len(jobs)
	if s.slow {
		<-ctx.Done()
		return make([]jevrank.Result, len(jobs))
	}
	out := make([]jevrank.Result, len(jobs))
	for i, j := range jobs {
		sc, ok := s.scores[j.Slug]
		out[i] = jevrank.Result{Slug: j.Slug, Score: sc, OK: ok}
	}
	return out
}

var testJobs = []jevrank.Job{{Slug: "a"}, {Slug: "b"}, {Slug: "c"}, {Slug: "d"}}

func order(t *testing.T, r *Reranker, ver string) ([]string, string) {
	t.Helper()
	return r.Rerank(context.Background(), 1, ver, "cv", testJobs)
}

func join(s []string) string {
	o := ""
	for _, x := range s {
		o += x
	}
	return o
}

func TestRerank_FitReloadAndCVVersion(t *testing.T) {
	sc := &stubScorer{scores: map[string]float64{"a": 0.2, "b": 0.9, "c": 0.2, "d": 0.5}}
	r := NewReranker(sc, mapCache{}, time.Second, time.Hour)
	got, fb := order(t, r, "v1")
	if fb != "" || join(got) != "bdac" { // ties a, c keep skill order
		t.Fatalf("got %v, %q", got, fb)
	}
	if got, _ = order(t, r, "v1"); join(got) != "bdac" || sc.calls != 1 {
		t.Errorf("reload: %v, scorer calls = %d", got, sc.calls)
	}
	order(t, r, "v2")
	if sc.calls != 2 {
		t.Errorf("new CV version must call the scorer again, calls = %d", sc.calls)
	}
}

func TestRerank_NilCacheAlwaysMisses(t *testing.T) {
	sc := &stubScorer{scores: map[string]float64{"a": 1, "b": 1, "c": 1, "d": 1}}
	r := NewReranker(sc, nil, time.Second, time.Hour)
	order(t, r, "v1")
	order(t, r, "v1")
	if sc.calls != 2 {
		t.Errorf("calls = %d", sc.calls)
	}
}

func TestRerank_PartialCachesAndFillsOnReload(t *testing.T) {
	sc := &stubScorer{scores: map[string]float64{"a": 0.1, "b": 0.2, "c": 0.3}}
	r := NewReranker(sc, mapCache{}, time.Second, time.Hour)
	if got, fb := order(t, r, "v1"); fb != FallbackPartial || got != nil {
		t.Fatalf("got %v, %q", got, fb)
	}
	sc.scores["d"] = 0.4
	got, fb := order(t, r, "v1")
	if fb != "" || join(got) != "dcba" || sc.sent != 5 { // 4 first time, only d the second
		t.Errorf("got %v, %q, sent %d", got, fb, sc.sent)
	}
}

func TestRerank_TimeoutAndError(t *testing.T) {
	r := NewReranker(&stubScorer{slow: true}, mapCache{}, 20*time.Millisecond, time.Hour)
	if _, fb := order(t, r, "v1"); fb != FallbackTimeout {
		t.Errorf("fallback = %q", fb)
	}
	r = NewReranker(&stubScorer{}, mapCache{}, time.Second, time.Hour)
	if _, fb := order(t, r, "v1"); fb != FallbackError {
		t.Errorf("fallback = %q", fb)
	}
}

type errCache struct{}

func (errCache) Get(context.Context, string) ([]byte, bool, error) {
	return nil, false, errors.New("down")
}

func (errCache) Set(context.Context, string, []byte, time.Duration) error { return errors.New("down") }

func TestRerank_CacheErrorsCountAsMiss(t *testing.T) {
	sc := &stubScorer{scores: map[string]float64{"a": 0.2, "b": 0.9, "c": 0.1, "d": 0.5}}
	r := NewReranker(sc, errCache{}, time.Second, time.Hour)
	got, fb := order(t, r, "v1")
	if fb != "" || join(got) != "bdac" || sc.sent != 4 {
		t.Errorf("got %v, %q, sent %d", got, fb, sc.sent)
	}
}
