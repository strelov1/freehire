package search

import (
	"context"

	"github.com/meilisearch/meilisearch-go"
)

// fakeSearchIndex stubs meilisearch.IndexManager for unit tests that only need to
// control Search: embedding the interface satisfies every other method
// (unused by these tests, so a nil call would simply never happen). Shared across
// this package's test files (e.g. facets_test.go's Client.facet-scoped tests).
type fakeSearchIndex struct {
	meilisearch.IndexManager
	searchFn func(ctx context.Context, query string, req *meilisearch.SearchRequest) (*meilisearch.SearchResponse, error)
}

func (f fakeSearchIndex) Search(ctx context.Context, query string, req *meilisearch.SearchRequest) (*meilisearch.SearchResponse, error) {
	return f.searchFn(ctx, query, req)
}

// fakeManager stubs meilisearch.ServiceManager for unit tests that only need
// MultiSearch (the federated OR path). Embedding the interface
// satisfies every other method.
type fakeManager struct {
	meilisearch.ServiceManager
	multiFn func(ctx context.Context, req *meilisearch.MultiSearchRequest) (*meilisearch.MultiSearchResponse, error)
}

func (f fakeManager) MultiSearch(ctx context.Context, req *meilisearch.MultiSearchRequest) (*meilisearch.MultiSearchResponse, error) {
	return f.multiFn(ctx, req)
}
