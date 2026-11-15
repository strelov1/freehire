package main

import (
	"context"
	"errors"
	"net/http"
	"sync"

	"golang.org/x/net/html"

	"github.com/strelov1/freehire/internal/ingest/sources"
)

// countingClient records what the probers deliberately throw away. A prober turns every transport
// failure into ("", 0, nil), which is right for a dead host and wrong for a platform that refused to
// answer. That error never reaches probeAll, so its all-probes-failed guard cannot fire; the transport
// is the last place that knows the difference. Without it, a run against a rate-limiting platform ends
// with a confident "found=N" that is short by an unknown amount.
type countingClient struct {
	inner httpClient
	mu    sync.Mutex
	// refusedN counts responses that mean "not now" — the platform is there and declined.
	refusedN int
	// answeredN counts requests the platform answered at all, success or a plain absence.
	answeredN int
}

func newCountingClient(inner httpClient) *countingClient {
	return &countingClient{inner: inner}
}

// record classifies one transport outcome. Only a 429 is a refusal. Any other status is the platform
// answering, including a 503: Traffit answers an unknown tenant with 503, and counting it turned an
// all-absent run into "refused=15467". A non-HTTP failure (timeout, DNS, closed connection) is a dead
// host, neither refused nor answered.
func (c *countingClient) record(err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err == nil {
		c.answeredN++
		return
	}
	var se *sources.StatusError
	if !errors.As(err, &se) {
		return
	}
	if se.Code == http.StatusTooManyRequests {
		c.refusedN++
		return
	}
	c.answeredN++
}

func (c *countingClient) refused() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.refusedN
}

func (c *countingClient) answered() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.answeredN
}

// refusalsDominated reports whether a run was mostly turned away rather than answered. Such a run
// did not happen, and reporting it as a harvest that found little gets a truncated board file committed.
func refusalsDominated(refused, answered int) bool {
	return refused > 0 && refused > answered
}

func (c *countingClient) GetJSON(ctx context.Context, url string, v any) error {
	err := c.inner.GetJSON(ctx, url, v)
	c.record(err)
	return err
}

func (c *countingClient) PostJSON(ctx context.Context, url string, body, v any) error {
	err := c.inner.PostJSON(ctx, url, body, v)
	c.record(err)
	return err
}

func (c *countingClient) GetXML(ctx context.Context, url string, v any) error {
	err := c.inner.GetXML(ctx, url, v)
	c.record(err)
	return err
}

func (c *countingClient) GetHTML(ctx context.Context, url string) (*html.Node, error) {
	n, err := c.inner.GetHTML(ctx, url)
	c.record(err)
	return n, err
}

func (c *countingClient) GetText(ctx context.Context, url string) (string, error) {
	s, err := c.inner.GetText(ctx, url)
	c.record(err)
	return s, err
}

func (c *countingClient) GetJSONWithHeaders(ctx context.Context, url string, headers map[string]string, v any) error {
	err := c.inner.GetJSONWithHeaders(ctx, url, headers, v)
	c.record(err)
	return err
}

func (c *countingClient) PostJSONWithHeaders(ctx context.Context, url string, headers map[string]string, body, v any) error {
	err := c.inner.PostJSONWithHeaders(ctx, url, headers, body, v)
	c.record(err)
	return err
}

// boardGone reports whether err is the platform saying the thing asked for does not exist —
// the one error a prober may read as "not a board" rather than surface as a probe failure.
func boardGone(err error) bool {
	var se *sources.StatusError
	return errors.As(err, &se) && (se.Code == http.StatusNotFound || se.Code == http.StatusGone)
}
