package search

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/meilisearch/meilisearch-go"
)

// fakeSearchIndex stubs meilisearch.IndexManager for unit tests that only need to
// control SearchWithContext: embedding the interface satisfies every other method
// (unused by these tests, so a nil call would simply never happen).
type fakeSearchIndex struct {
	meilisearch.IndexManager
	searchFn func(ctx context.Context, query string, req *meilisearch.SearchRequest) (*meilisearch.SearchResponse, error)
}

func (f fakeSearchIndex) SearchWithContext(ctx context.Context, query string, req *meilisearch.SearchRequest) (*meilisearch.SearchResponse, error) {
	return f.searchFn(ctx, query, req)
}

// A cancelled context must re-raise the context sentinel, not the SDK's opaque
// communication error, so a client disconnect is not misreported upstream as a fault.
func TestRecommendByVector_CancelledContextReRaisesSentinel(t *testing.T) {
	c := &Client{semantic: fakeSearchIndex{searchFn: func(context.Context, string, *meilisearch.SearchRequest) (*meilisearch.SearchResponse, error) {
		return nil, errors.New("meilisearch: could not perform request: context canceled")
	}}}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := c.RecommendByVector(ctx, []float64{0.1, 0.2}, nil, 10, 0)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
}

// A Meili 400 (malformed filter) must map to ErrBadQuery so the handler can render
// it as a client mistake and skip Sentry.
func TestRecommendByVector_BadRequestMapsToErrBadQuery(t *testing.T) {
	c := &Client{semantic: fakeSearchIndex{searchFn: func(context.Context, string, *meilisearch.SearchRequest) (*meilisearch.SearchResponse, error) {
		return nil, &meilisearch.Error{StatusCode: http.StatusBadRequest}
	}}}

	_, err := c.RecommendByVector(context.Background(), []float64{0.1, 0.2}, nil, 10, 0)
	if !errors.Is(err, ErrBadQuery) {
		t.Fatalf("err = %v, want ErrBadQuery", err)
	}
}
