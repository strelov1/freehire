package wikicompany

import (
	"fmt"
	"net/http"
	"time"
)

// doWithRetry issues req, retrying on 429 and 5xx responses up to maxAttempts times
// with backoff between attempts. It gives up and returns an error carrying the last
// status code once maxAttempts is reached. A network-level error is not retried,
// since Wikidata/Wikipedia rate limiting surfaces as a status code, not a transport
// failure.
func doWithRetry(doer *http.Client, req *http.Request, maxAttempts int, backoff time.Duration) (*http.Response, error) {
	var lastResp *http.Response
	var lastErr error

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		resp, err := doer.Do(req)
		if err != nil {
			return nil, err
		}
		if resp.StatusCode != http.StatusTooManyRequests && resp.StatusCode < 500 {
			return resp, nil
		}
		if lastResp != nil {
			lastResp.Body.Close()
		}
		lastResp, lastErr = resp, fmt.Errorf("%s returned status %d after %d attempt(s)", req.URL, resp.StatusCode, attempt)
		if attempt < maxAttempts && backoff > 0 {
			time.Sleep(backoff)
		}
	}
	return lastResp, lastErr
}
