package search

import (
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/meilisearch/meilisearch-go"
)

// A 400 from Meilisearch is the engine rejecting a malformed query or filter —
// the request's fault. isBadRequest must single it out so Search can tag it with
// ErrBadQuery (which handlers map to 400 and keep out of Sentry), while transport
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
