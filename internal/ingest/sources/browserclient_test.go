package sources

import (
	"errors"
	"net/http"
	"testing"
)

// The browser client's errors must be indistinguishable from the plain client's, because the
// helpers that read them — isRateLimited, the 404-means-gone check — match on *StatusError and
// not on a message. A browser-fetched 404 that arrived as a bare error would silently stop
// meaning "this posting is gone".
func TestBrowserStatusErrorIsTheSameTypeThePlainClientReturns(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		want   bool // want an error
	}{
		{"200 is not an error", http.StatusOK, false},
		{"204 is not an error", http.StatusNoContent, false},
		{"299 is not an error", 299, false},
		{"404 is an error", http.StatusNotFound, true},
		{"403 is an error", http.StatusForbidden, true},
		{"429 is an error", http.StatusTooManyRequests, true},
		{"500 is an error", http.StatusInternalServerError, true},
		{"300 is an error", http.StatusMultipleChoices, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := browserStatusError("https://example.test/x", tc.status)
			if (err != nil) != tc.want {
				t.Fatalf("browserStatusError(%d) = %v, want error: %v", tc.status, err, tc.want)
			}
			if err == nil {
				return
			}
			var se *StatusError
			if !errors.As(err, &se) {
				t.Fatalf("error is %T, want *StatusError so the existing helpers can read it", err)
			}
			if se.Code != tc.status {
				t.Errorf("StatusError.Code = %d, want %d", se.Code, tc.status)
			}
			if se.URL != "https://example.test/x" {
				t.Errorf("StatusError.URL = %q, want the fetched url", se.URL)
			}
		})
	}
}

// The two helpers that decide what a status MEANS are the reason the type matters, so the
// test walks them rather than trusting that it does.
func TestBrowserStatusErrorIsReadableByTheExistingHelpers(t *testing.T) {
	if !isRateLimited(browserStatusError("https://example.test/x", http.StatusTooManyRequests)) {
		t.Error("a browser-fetched 429 is not recognised as rate limiting")
	}
	if isRateLimited(browserStatusError("https://example.test/x", http.StatusNotFound)) {
		t.Error("a browser-fetched 404 is being read as rate limiting")
	}
}
