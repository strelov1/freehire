package wikicompany

import (
	"fmt"
	"net/http"
	"time"
)

// doWithRetry issues req, retrying on 429 and 5xx responses up to maxAttempts times
// with backoff between attempts. On success it returns the response with a nil
// error; once maxAttempts is exhausted it closes the last response's body itself
// and returns (nil, err) — no caller ever needs a response body on the error path,
// so this is the only place that can reliably close it. A network-level error is
// not retried, since Wikidata/Wikipedia rate limiting surfaces as a status code,
// not a transport failure.
func doWithRetry(doer *http.Client, req *http.Request, maxAttempts int, backoff time.Duration) (*http.Response, error) {
	var lastErr error

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		resp, err := doer.Do(req)
		if err != nil {
			return nil, err
		}
		if resp.StatusCode != http.StatusTooManyRequests && resp.StatusCode < 500 {
			return resp, nil
		}
		lastErr = fmt.Errorf("%s returned status %d after %d attempt(s)", req.URL, resp.StatusCode, attempt)
		resp.Body.Close()
		if attempt < maxAttempts && backoff > 0 {
			time.Sleep(backoff)
		}
	}
	return nil, lastErr
}
