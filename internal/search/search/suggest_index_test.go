package search

import (
	"errors"
	"net/http"
	"testing"

	"github.com/meilisearch/meilisearch-go"
)

// A dictionary that has not been built yet is "no suggestions", not a server fault.
//
// Found on production: the endpoint answered 500 to every completion between the
// deploy and the first cmd/build-suggestions run, because the index did not exist yet.
// The box fires one of those per settled keystroke, so the whole window would have
// been a broken-looking dropdown and a stream of errors in the tracker — for a state
// that is simply "not built".
func TestIsIndexMissing(t *testing.T) {
	// A 404 from a search against this index has one meaning. The status is the whole
	// test rather than the API's error code because the SDK keeps that code in an
	// unexported type, so no caller can construct one — a check written against it
	// could never be tested.
	if !isIndexMissing(&meilisearch.Error{StatusCode: http.StatusNotFound}) {
		t.Error("a missing index must read as an empty dictionary")
	}

	// Everything else stays a real failure. Swallowing those would turn an outage into
	// a box that quietly offers nothing.
	for name, err := range map[string]error{
		"bad request":       &meilisearch.Error{StatusCode: http.StatusBadRequest},
		"engine error":      &meilisearch.Error{StatusCode: http.StatusInternalServerError},
		"wrong key":         &meilisearch.Error{StatusCode: http.StatusForbidden},
		"transport failure": errors.New("connection refused"),
		"nil":               nil,
	} {
		if isIndexMissing(err) {
			t.Errorf("%s must not read as a missing index", name)
		}
	}
}
