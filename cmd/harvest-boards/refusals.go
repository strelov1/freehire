package main

import (
	"context"
	"errors"
	"net/http"
	"sync"

	"golang.org/x/net/html"

	"github.com/strelov1/freehire/internal/sources"
)

// countingClient records what the probers deliberately throw away. A prober turns every
// transport failure into ("", 0, nil) — "no such board" — which is right for a dead host and
// wrong for a platform that refused to answer. Because the error never reaches probeAll, its
// all-probes-failed guard can never fire for those probers: they do not fail, they report
// absence. So the transport is the last place that still knows the difference, and it counts
// it here.
//
// This matters more than it sounds. A harvest run against a rate-limiting platform otherwise
// ends with a confident "found=N" that is short by an unknown amount, indistinguishable from
// a run that genuinely found nothing new.
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

// record classifies one transport outcome. Only a 429 is a refusal: it is the one status that
// unambiguously means "the platform is there and is declining". Every other status is the
// platform answering — a 404, or a 503, which reads like a refusal but is not one. Traffit
// answers an unknown tenant with 503 and a live board with 200, so counting it turned a run
// where every candidate was simply absent into "refused=15467, nothing written, exit 1".
// Anything that is not an HTTP status at all — a timeout, a DNS failure, a closed connection
// — is a dead host, which is neither refused nor answered.
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

// refusalsDominated reports whether a run was mostly turned away rather than answered. Such a
// run is not a harvest that found little; it is a harvest that did not happen, and reporting
// it as the former is how a truncated board file gets committed.
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
