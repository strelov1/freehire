package handler

import (
	"encoding/json"
	"io"
	"net/http"
	"testing"
)

// decodeJSON reads resp's body into v and returns the status code.
//
// A 2xx whose body is non-empty and does not decode fails the test. Discarding
// that error — `_ = json.Unmarshal(b, &v)` — is what let a handler answer 200 with
// Fiber's bare "OK" body for weeks: the assertions that followed only read the
// status, so nothing ever looked at the payload, and the helper was more forgiving
// than any real client (see #1186).
//
// An empty 2xx body is not an error: that is a 204, which several endpoints here
// answer by design. A non-2xx body is not decoded strictly either — an error
// response may legitimately be plain text, and those tests assert on the status.
func decodeJSON(t *testing.T, resp *http.Response, v any) int {
	t.Helper()
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	ok2xx := resp.StatusCode >= 200 && resp.StatusCode < 300
	if len(b) > 0 {
		if uerr := json.Unmarshal(b, v); uerr != nil && ok2xx {
			t.Fatalf("status %d body does not decode as JSON: %v (body %q)", resp.StatusCode, uerr, b)
		}
	}
	return resp.StatusCode
}
