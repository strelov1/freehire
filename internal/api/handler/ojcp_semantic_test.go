package handler

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"testing"

	"github.com/strelov1/freehire/internal/api/ojcp"
	"github.com/strelov1/freehire/internal/search/search"
)

// hybridRecordingSearcher records every request and can refuse the hybrid ones, the way the
// engine refuses a vector for an embedder its live settings do not declare.
type hybridRecordingSearcher struct {
	calls        []search.SearchParams
	refuseHybrid bool
}

func (r *hybridRecordingSearcher) Search(_ context.Context, p search.SearchParams) (search.SearchResult, error) {
	r.calls = append(r.calls, p)
	if r.refuseHybrid && p.SemanticVector != nil {
		return search.SearchResult{}, fmt.Errorf("search: query: %w: unknown embedder desc", search.ErrBadQuery)
	}
	if p.SemanticVector != nil {
		// What search.Client answers a hybrid with: a total that is only a lower bound.
		return search.SearchResult{Total: 5, TotalIsLowerBound: true}, nil
	}
	return search.SearchResult{}, nil
}

// fakeQueryEmbedder answers a fixed vector or a fixed error and counts its calls.
type fakeQueryEmbedder struct {
	vec   []float32
	err   error
	calls int
	got   string
}

func (f *fakeQueryEmbedder) EmbedQuery(_ context.Context, text string) ([]float32, error) {
	f.calls++
	f.got = text
	return f.vec, f.err
}

func semanticHandlers(s searcher, e queryEmbedder) *ojcpHandlers {
	h := newOJCPHandlers(s, fakeOJCPStore{}, "https://freehire.me", nil)
	h.semantic = semanticSearch{embedder: e, threshold: 0.7}
	return h
}

func semanticInput(query string, semantic *bool) ojcp.SearchInput {
	return ojcp.SearchInput{
		Query:    query,
		Location: &ojcp.SearchLocation{Country: "DE"},
		Semantic: semantic,
	}
}

// An agent that did not ask gets today's request, byte for byte, and costs no embed.
func TestOJCPSemanticSearchIsNotRunUnlessAsked(t *testing.T) {
	for name, flag := range map[string]*bool{"absent": nil, "false": boolPtr(false)} {
		t.Run(name, func(t *testing.T) {
			s := &hybridRecordingSearcher{}
			e := &fakeQueryEmbedder{vec: []float32{1}}

			resp, err := semanticHandlers(s, e).SearchJobs(context.Background(), semanticInput("payments", flag))
			if err != nil {
				t.Fatalf("SearchJobs: %v", err)
			}
			if e.calls != 0 {
				t.Errorf("embedder called %d times, want 0", e.calls)
			}
			if len(s.calls) != 1 || s.calls[0].SemanticVector != nil || s.calls[0].ScoreThreshold != 0 {
				t.Errorf("search calls = %+v, want one plain keyword request", s.calls)
			}
			if resp.Semantic != "" {
				t.Errorf("semantic = %q, want omitted", resp.Semantic)
			}
		})
	}
}

func TestOJCPSemanticSearchRunsAHybridWithTheAgentsFilters(t *testing.T) {
	s := &hybridRecordingSearcher{}
	e := &fakeQueryEmbedder{vec: []float32{0.1, 0.2}}

	resp, err := semanticHandlers(s, e).SearchJobs(context.Background(), semanticInput("payments", boolPtr(true)))
	if err != nil {
		t.Fatalf("SearchJobs: %v", err)
	}
	if e.got != "payments" {
		t.Errorf("embedded %q, want the query text", e.got)
	}
	if len(s.calls) != 1 {
		t.Fatalf("search calls = %d, want 1", len(s.calls))
	}
	got := s.calls[0]
	if len(got.SemanticVector) != 2 || got.ScoreThreshold != 0.7 || got.Query != "payments" {
		t.Errorf("request = %+v, want the query, its vector and the threshold", got)
	}
	if got.Filter == nil {
		t.Error("the hybrid request dropped the agent's country filter")
	}
	if resp.Semantic != ojcp.SemanticApplied {
		t.Errorf("semantic = %q, want %q", resp.Semantic, ojcp.SemanticApplied)
	}
}

func TestOJCPSemanticSearchFallsBackWhenTheEmbedFails(t *testing.T) {
	s := &hybridRecordingSearcher{}
	e := &fakeQueryEmbedder{err: errors.New("context deadline exceeded")}

	resp, err := semanticHandlers(s, e).SearchJobs(context.Background(), semanticInput("payments", boolPtr(true)))
	if err != nil {
		t.Fatalf("SearchJobs: %v — a failed embed must not fail the search", err)
	}
	if len(s.calls) != 1 || s.calls[0].SemanticVector != nil {
		t.Errorf("search calls = %+v, want one keyword request", s.calls)
	}
	if resp.Semantic != ojcp.SemanticUnavailable {
		t.Errorf("semantic = %q, want %q", resp.Semantic, ojcp.SemanticUnavailable)
	}
}

func TestOJCPSemanticSearchWithoutABackendIsUnavailable(t *testing.T) {
	s := &hybridRecordingSearcher{}

	resp, err := semanticHandlers(s, nil).SearchJobs(context.Background(), semanticInput("payments", boolPtr(true)))
	if err != nil {
		t.Fatalf("SearchJobs: %v", err)
	}
	if len(s.calls) != 1 || s.calls[0].SemanticVector != nil {
		t.Errorf("search calls = %+v, want one keyword request", s.calls)
	}
	if resp.Semantic != ojcp.SemanticUnavailable {
		t.Errorf("semantic = %q, want %q", resp.Semantic, ojcp.SemanticUnavailable)
	}
}

// The rollout hazard: a binary that sends a `desc` vector before the live index declares
// the embedder gets a 400. That must cost the agent its semantic ranking, not its answer.
func TestOJCPSemanticSearchFallsBackWhenTheEngineRefusesTheHybrid(t *testing.T) {
	s := &hybridRecordingSearcher{refuseHybrid: true}
	e := &fakeQueryEmbedder{vec: []float32{1}}

	resp, err := semanticHandlers(s, e).SearchJobs(context.Background(), semanticInput("payments", boolPtr(true)))
	if err != nil {
		t.Fatalf("SearchJobs: %v", err)
	}
	if len(s.calls) != 2 || s.calls[1].SemanticVector != nil {
		t.Errorf("search calls = %+v, want the refused hybrid then a keyword retry", s.calls)
	}
	if resp.Semantic != ojcp.SemanticUnavailable {
		t.Errorf("semantic = %q, want %q", resp.Semantic, ojcp.SemanticUnavailable)
	}
}

func TestOJCPSemanticSearchWithNoQueryReportsTheFlagIgnored(t *testing.T) {
	s := &hybridRecordingSearcher{}
	e := &fakeQueryEmbedder{vec: []float32{1}}

	resp, err := semanticHandlers(s, e).SearchJobs(context.Background(), semanticInput("", boolPtr(true)))
	if err != nil {
		t.Fatalf("SearchJobs: %v", err)
	}
	if e.calls != 0 {
		t.Errorf("embedder called %d times, want 0 — there is no text to embed", e.calls)
	}
	if !slices.Contains(resp.IgnoredParams, "semantic") {
		t.Errorf("ignored_params = %v, want it to name semantic", resp.IgnoredParams)
	}
	if resp.Semantic != "" {
		t.Errorf("semantic = %q, want omitted when the flag was not honoured at all", resp.Semantic)
	}
}

// A hybrid's total is "at least this many", and the agent is told so; a keyword answer
// keeps its ordinary total and says nothing.
func TestOJCPSemanticSearchMarksItsTotalAsALowerBound(t *testing.T) {
	e := &fakeQueryEmbedder{vec: []float32{1}}
	resp, err := semanticHandlers(&hybridRecordingSearcher{}, e).SearchJobs(context.Background(), semanticInput("payments", boolPtr(true)))
	if err != nil {
		t.Fatalf("SearchJobs: %v", err)
	}
	if resp.TotalResults != 5 || !resp.TotalIsLowerBound {
		t.Errorf("total=%d lowerBound=%v, want 5 marked as a lower bound", resp.TotalResults, resp.TotalIsLowerBound)
	}

	resp, err = semanticHandlers(&hybridRecordingSearcher{}, e).SearchJobs(context.Background(), semanticInput("payments", nil))
	if err != nil {
		t.Fatalf("SearchJobs: %v", err)
	}
	if resp.TotalIsLowerBound {
		t.Errorf("keyword answer marked as a lower bound")
	}
}
