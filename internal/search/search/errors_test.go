package search

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/meilisearch/meilisearch-go"
)

// A 400 from Meilisearch is the engine rejecting a malformed query or filter —
// the request's fault. isBadRequest must single it out so Search can tag it with
// ErrBadQuery (which handlers map to 400 and do not report), while transport
// failures and other status codes stay unexpected faults.
func TestIsBadRequest(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"meili 400 is a bad request", &meilisearch.Error{StatusCode: http.StatusBadRequest}, true},
		{"wrapped meili 400 is a bad request", fmt.Errorf("search: query: %w", &meilisearch.Error{StatusCode: http.StatusBadRequest}), true},
		{"meili 500 is not a bad request", &meilisearch.Error{StatusCode: http.StatusInternalServerError}, false},
		{"non-meili error is not a bad request", errors.New("context canceled"), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isBadRequest(tt.err); got != tt.want {
				t.Errorf("isBadRequest = %v, want %v", got, tt.want)
			}
		})
	}
}

// A full search queue answers 503: queryErr must tag it ErrBusy (503 + Retry-After, not
// reported) and leave other engine failures untagged faults.
func TestQueryErrTagsAFullSearchQueueAsBusy(t *testing.T) {
	ctx := context.Background()
	busy := &meilisearch.Error{StatusCode: http.StatusServiceUnavailable}
	if err := queryErr(ctx, "search: query", busy); !errors.Is(err, ErrBusy) || errors.Is(err, ErrBadQuery) {
		t.Errorf("503: err = %v, want ErrBusy only", err)
	}
	if err := queryErr(ctx, "search: query", &meilisearch.Error{StatusCode: http.StatusInternalServerError}); errors.Is(err, ErrBusy) {
		t.Errorf("500: err = %v, want no ErrBusy", err)
	}
}
