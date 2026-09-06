package gmailsync

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"golang.org/x/oauth2"
)

// The classification both syncs read, stated as a table because the cost of getting it
// wrong is asymmetric: a revocation missed costs one candidate a stale mailbox until they
// notice, while a transient failure MISREAD as a revocation disconnects every mailbox we
// hold at once, each needing a restricted-scope consent to restore.
func TestRevokedGrant(t *testing.T) {
	for _, tc := range []struct {
		name    string
		err     error
		revoked bool
	}{
		{"api refuses the access token", &APIError{Op: "gmail: list", StatusCode: 401, Status: "401 Unauthorized"}, true},
		{"api forbids the scope", &APIError{Op: "gmail: list", StatusCode: 403, Status: "403 Forbidden"}, true},
		{"api is rate limiting", &APIError{Op: "gmail: list", StatusCode: 429, Status: "429 Too Many Requests"}, false},
		{"api is unwell", &APIError{Op: "gmail: list", StatusCode: 500, Status: "500 Internal Server Error"}, false},
		{
			"token endpoint refuses the refresh token",
			&url.Error{Op: "Get", URL: "https://gmail.googleapis.com/", Err: &oauth2.RetrieveError{ErrorCode: "invalid_grant"}},
			true,
		},
		{
			"token endpoint refuses OUR client",
			&url.Error{Op: "Get", URL: "https://gmail.googleapis.com/", Err: &oauth2.RetrieveError{ErrorCode: "invalid_client"}},
			false,
		},
		{"transport failed", &url.Error{Op: "Get", URL: "https://gmail.googleapis.com/", Err: errors.New("connection reset by peer")}, false},
		{"run cancelled", context.Canceled, false},
		{"deadline passed", context.DeadlineExceeded, false},
		{"wrapped once more", fmt.Errorf("list events: %w", &APIError{StatusCode: 401, Status: "401 Unauthorized"}), true},
		{"nothing at all", errors.New("something went wrong"), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := RevokedGrant(tc.err); got != tc.revoked {
				t.Errorf("RevokedGrant(%v) = %v, want %v", tc.err, got, tc.revoked)
			}
		})
	}
}

// The reader has to MARK what it saw for the rule above to have anything to read: an
// untyped fmt.Errorf carrying the status only in its text makes a 401 and a 503 the same
// value. Both entry points, since a wave's list and its per-message fetches fail apart.
func TestAPIReaderCarriesTheStatus(t *testing.T) {
	for _, status := range []int{http.StatusUnauthorized, http.StatusTooManyRequests} {
		r := &apiReader{client: &http.Client{Transport: answering(status)}}

		var apiErr *APIError
		if err := r.getJSON(context.Background(), "https://example.test/messages", &struct{}{}); !errors.As(err, &apiErr) {
			t.Fatalf("getJSON error = %v (%T), want an *APIError", err, err)
		}
		if apiErr.StatusCode != status {
			t.Errorf("getJSON status = %d, want %d", apiErr.StatusCode, status)
		}

		apiErr = nil
		if _, err := r.GetMessage(context.Background(), "m1"); !errors.As(err, &apiErr) {
			t.Fatalf("GetMessage error = %v (%T), want an *APIError", err, err)
		}
		if apiErr.StatusCode != status {
			t.Errorf("GetMessage status = %d, want %d", apiErr.StatusCode, status)
		}
	}
}

// answering is a transport that gives every request the same status, so the reader's own
// hardcoded Gmail URLs need no seam to be tested against.
type answering int

func (status answering) RoundTrip(req *http.Request) (*http.Response, error) {
	return &http.Response{
		StatusCode: int(status),
		Status:     fmt.Sprintf("%d %s", int(status), http.StatusText(int(status))),
		Body:       io.NopCloser(strings.NewReader("")),
		Request:    req,
	}, nil
}
