package handler

import (
	"context"
	"slices"
	"testing"

	"github.com/strelov1/freehire/internal/api/ojcp"
)

func semanticHandlers(s searcher) *ojcpHandlers {
	return newOJCPHandlers(s, fakeOJCPStore{}, "https://freehire.me", nil)
}

func semanticInput(query string, semantic *bool) ojcp.SearchInput {
	return ojcp.SearchInput{
		Query:    query,
		Location: &ojcp.SearchLocation{Country: "DE"},
		Semantic: semantic,
	}
}

// An agent that did not ask hears nothing about semantic search.
func TestOJCPSemanticSearchIsNotReportedUnlessAsked(t *testing.T) {
	for name, flag := range map[string]*bool{"absent": nil, "false": boolPtr(false)} {
		t.Run(name, func(t *testing.T) {
			resp, err := semanticHandlers(&fakeSearcher{}).SearchJobs(context.Background(), semanticInput("payments", flag))
			if err != nil {
				t.Fatalf("SearchJobs: %v", err)
			}
			if resp.Semantic != "" {
				t.Errorf("semantic = %q, want omitted", resp.Semantic)
			}
		})
	}
}

// The flag is accepted but cannot be honoured: the keyword answer, with the agent's
// filters, says it was not answered by meaning.
func TestOJCPSemanticSearchIsUnavailable(t *testing.T) {
	s := &fakeSearcher{}
	resp, err := semanticHandlers(s).SearchJobs(context.Background(), semanticInput("payments", boolPtr(true)))
	if err != nil {
		t.Fatalf("SearchJobs: %v", err)
	}
	if s.got.Query != "payments" || s.got.Filter == nil {
		t.Errorf("request = %+v, want the keyword query with the agent's country filter", s.got)
	}
	if resp.Semantic != ojcp.SemanticUnavailable {
		t.Errorf("semantic = %q, want %q", resp.Semantic, ojcp.SemanticUnavailable)
	}
}

func TestOJCPSemanticSearchWithNoQueryReportsTheFlagIgnored(t *testing.T) {
	resp, err := semanticHandlers(&fakeSearcher{}).SearchJobs(context.Background(), semanticInput("", boolPtr(true)))
	if err != nil {
		t.Fatalf("SearchJobs: %v", err)
	}
	if !slices.Contains(resp.IgnoredParams, "semantic") {
		t.Errorf("ignored_params = %v, want it to name semantic", resp.IgnoredParams)
	}
	if resp.Semantic != "" {
		t.Errorf("semantic = %q, want omitted when the flag was not honoured at all", resp.Semantic)
	}
}
