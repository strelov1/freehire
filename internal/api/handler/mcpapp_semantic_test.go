package handler

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/strelov1/freehire/internal/api/mcpapp"
)

func mcpappSemantic(s searcher, e queryEmbedder) *mcpappHandlers {
	h := mcpappUnder(s, nil, fakeOJCPStore{})
	h.semantic = semanticSearch{embedder: e, threshold: 0.7}
	return h
}

// A model that did not ask gets today's request and costs no embed.
func TestTheAppSemanticSearchIsNotRunUnlessAsked(t *testing.T) {
	s := &hybridRecordingSearcher{}
	e := &fakeQueryEmbedder{vec: []float32{1}}

	got, err := mcpappSemantic(s, e).SearchJobs(context.Background(), mcpapp.SearchInput{Query: "payments"})
	if err != nil {
		t.Fatal(err)
	}
	if e.calls != 0 {
		t.Errorf("embedder called %d times, want 0", e.calls)
	}
	if len(s.calls) != 1 || s.calls[0].SemanticVector != nil {
		t.Errorf("search calls = %+v, want one plain keyword request", s.calls)
	}
	if got.Semantic != "" {
		t.Errorf("semantic = %q, want omitted", got.Semantic)
	}
}

// semantic: true runs the shared hybrid over the same filter and says it was applied.
func TestTheAppSemanticSearchRunsTheHybrid(t *testing.T) {
	s := &hybridRecordingSearcher{}
	e := &fakeQueryEmbedder{vec: []float32{0.1}}

	got, err := mcpappSemantic(s, e).SearchJobs(context.Background(), mcpapp.SearchInput{
		Query: "help kids learn to read", Countries: []string{"US"}, Semantic: boolPtr(true),
	})
	if err != nil {
		t.Fatal(err)
	}
	if e.got != "help kids learn to read" {
		t.Errorf("embedded %q, want the query", e.got)
	}
	if len(s.calls) != 1 || s.calls[0].SemanticVector == nil || s.calls[0].Filter == nil || s.calls[0].ScoreThreshold != 0.7 {
		t.Errorf("search calls = %+v, want one hybrid request keeping the filter and the floor", s.calls)
	}
	if got.Semantic != "applied" {
		t.Errorf("semantic = %q, want applied", got.Semantic)
	}
}

// A failed embed answers keyword-only and says so; no text to embed reports the flag dropped.
func TestTheAppSemanticSearchDegradesAndSaysSo(t *testing.T) {
	s := &hybridRecordingSearcher{}
	failing := &fakeQueryEmbedder{err: errors.New("deadline exceeded")}
	got, err := mcpappSemantic(s, failing).SearchJobs(context.Background(), mcpapp.SearchInput{Query: "payments", Semantic: boolPtr(true)})
	if err != nil {
		t.Fatal(err)
	}
	if got.Semantic != "unavailable" {
		t.Errorf("semantic = %q, want unavailable", got.Semantic)
	}

	e := &fakeQueryEmbedder{vec: []float32{1}}
	got, err = mcpappSemantic(&hybridRecordingSearcher{}, e).SearchJobs(context.Background(), mcpapp.SearchInput{
		Countries: []string{"DE"}, Semantic: boolPtr(true),
	})
	if err != nil {
		t.Fatal(err)
	}
	if e.calls != 0 || got.Semantic != "" || !slices.Contains(got.IgnoredParams, "semantic") {
		t.Errorf("embeds=%d semantic=%q ignored=%v, want no embed, no report, semantic listed as ignored",
			e.calls, got.Semantic, got.IgnoredParams)
	}
}

func TestTheAppSemanticTotalIsMarkedALowerBound(t *testing.T) {
	e := &fakeQueryEmbedder{vec: []float32{1}}
	got, err := mcpappSemantic(&hybridRecordingSearcher{}, e).SearchJobs(context.Background(), mcpapp.SearchInput{
		Query: "payments", Semantic: boolPtr(true),
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Total != 5 || !got.TotalIsLowerBound {
		t.Errorf("total=%d lowerBound=%v, want 5 marked as a lower bound", got.Total, got.TotalIsLowerBound)
	}
}
