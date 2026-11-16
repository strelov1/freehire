package sources

import "net/http"

// highermeAttemptGetter puts admission below Client's retry loop without changing the
// shared client's transport, retry policy, or host backoff for other providers.
func highermeAttemptGetter(c *Client, limiter waiter) JSONGetter {
	copyClient := *c
	copyClient.httpClient = highermePacedHTTP(c.httpClient, limiter, c.backoff)
	if c.refusalClient != nil {
		copyClient.refusalClient = highermePacedHTTP(c.refusalClient, limiter, nil)
	}
	return &copyClient
}

// Admission waits outside the attempt timeout. Otherwise queued boards exhaust the
// shared client's 15-second timeout before they are permitted to send a request.
func highermePacedHTTP(c *http.Client, limiter waiter, backoff *hostBackoff) *http.Client {
	outer, inner := *c, *c
	outer.Timeout = 0
	inner.Jar = nil
	inner.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	outer.Transport = highermeTransport{inner: &inner, limiter: limiter, backoff: backoff}
	return &outer
}

type highermeTransport struct {
	inner   *http.Client
	limiter waiter
	backoff *hostBackoff
}

func (t highermeTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	for {
		if err := t.limiter.Wait(r.Context()); err != nil {
			return nil, err
		}
		if t.backoff == nil {
			break
		}
		t.backoff.mu.Lock()
		held := t.backoff.until[r.URL.Host].After(t.backoff.now())
		t.backoff.mu.Unlock()
		if !held {
			break
		}
		if err := t.backoff.wait(r.Context(), r.URL.Host); err != nil {
			return nil, err
		}
		// A token spent before the hold expired cannot admit a request after it.
	}
	return t.inner.Do(r)
}
