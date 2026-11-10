package handler

import (
	"context"
	"slices"
	"testing"

	"github.com/strelov1/freehire/internal/api/mcpapp"
)

// A model that did not ask hears nothing about semantic search.
func TestTheAppSemanticSearchIsNotReportedUnlessAsked(t *testing.T) {
	got, err := mcpappUnder(&fakeSearcher{}, nil, fakeOJCPStore{}).SearchJobs(context.Background(), mcpapp.SearchInput{Query: "payments"})
	if err != nil {
		t.Fatal(err)
	}
	if got.Semantic != "" {
		t.Errorf("semantic = %q, want omitted", got.Semantic)
	}
}

// semantic: true answers by keywords and says so; no text reports the flag dropped.
func TestTheAppSemanticSearchIsUnavailable(t *testing.T) {
	s := &fakeSearcher{}
	got, err := mcpappUnder(s, nil, fakeOJCPStore{}).SearchJobs(context.Background(), mcpapp.SearchInput{
		Query: "help kids learn to read", Countries: []string{"US"}, Semantic: boolPtr(true),
	})
	if err != nil {
		t.Fatal(err)
	}
	if s.got.Filter == nil {
		t.Errorf("request = %+v, want the country filter kept", s.got)
	}
	if got.Semantic != "unavailable" {
		t.Errorf("semantic = %q, want unavailable", got.Semantic)
	}

	got, err = mcpappUnder(&fakeSearcher{}, nil, fakeOJCPStore{}).SearchJobs(context.Background(), mcpapp.SearchInput{
		Countries: []string{"DE"}, Semantic: boolPtr(true),
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Semantic != "" || !slices.Contains(got.IgnoredParams, "semantic") {
		t.Errorf("semantic=%q ignored=%v, want no report, semantic listed as ignored", got.Semantic, got.IgnoredParams)
	}
}
