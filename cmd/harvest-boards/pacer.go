package main

import (
	"context"

	"golang.org/x/net/html"
	"golang.org/x/time/rate"
)

// waiter gates a request until the limiter admits it. *rate.Limiter satisfies it; tests
// inject a fake so the gate can be asserted without timing flake. Mirrors the ingest-side
// pacer in internal/sources.
type waiter interface {
	Wait(ctx context.Context) error
}

// pacedClient wraps the probe transport with a shared limiter so a run's AGGREGATE request
// rate to one platform stays under its per-IP window budget, independent of worker count.
//
// The worker pool bounds how many requests are in flight; it does not bound how many go out
// per minute. A harvest is a far denser traffic shape than the ingest crawl the platforms are
// used to — the first orphan harvest put ~15k probes at Workable in minutes, and partway
// through it began answering with an HTML challenge instead of JSON. The prober reads a
// non-JSON body as a dead board, so the candidates arriving during the penalty window were
// not judged absent, they were never judged at all: a silently truncated harvest that looks
// like a complete one.
//
// Every verb is gated, not just the JSON ones, so a provider whose prober scrapes HTML or
// posts a query shares the same bucket.
type pacedClient struct {
	inner   httpClient
	limiter waiter
}

func (c pacedClient) GetJSON(ctx context.Context, url string, v any) error {
	if err := c.limiter.Wait(ctx); err != nil {
		return err
	}
	return c.inner.GetJSON(ctx, url, v)
}

func (c pacedClient) PostJSON(ctx context.Context, url string, body, v any) error {
	if err := c.limiter.Wait(ctx); err != nil {
		return err
	}
	return c.inner.PostJSON(ctx, url, body, v)
}

func (c pacedClient) GetXML(ctx context.Context, url string, v any) error {
	if err := c.limiter.Wait(ctx); err != nil {
		return err
	}
	return c.inner.GetXML(ctx, url, v)
}

func (c pacedClient) GetHTML(ctx context.Context, url string) (*html.Node, error) {
	if err := c.limiter.Wait(ctx); err != nil {
		return nil, err
	}
	return c.inner.GetHTML(ctx, url)
}

func (c pacedClient) GetText(ctx context.Context, url string) (string, error) {
	if err := c.limiter.Wait(ctx); err != nil {
		return "", err
	}
	return c.inner.GetText(ctx, url)
}

func (c pacedClient) GetJSONWithHeaders(ctx context.Context, url string, headers map[string]string, v any) error {
	if err := c.limiter.Wait(ctx); err != nil {
		return err
	}
	return c.inner.GetJSONWithHeaders(ctx, url, headers, v)
}

func (c pacedClient) PostJSONWithHeaders(ctx context.Context, url string, headers map[string]string, body, v any) error {
	if err := c.limiter.Wait(ctx); err != nil {
		return err
	}
	return c.inner.PostJSONWithHeaders(ctx, url, headers, body, v)
}

// paced wraps a client to admit at most ratePerSec requests per second, or returns it
// unchanged when ratePerSec is zero — an unpaced run stays the default, since most platforms
// answer a whole harvest without complaint and pacing only lengthens it.
func paced(c httpClient, ratePerSec float64) httpClient {
	if ratePerSec <= 0 {
		return c
	}
	// Burst 1: the point is the total per window, and a burst bucket would let a fresh run
	// fire its whole allowance at once — which is the shape that drew the challenge.
	return pacedClient{inner: c, limiter: rate.NewLimiter(rate.Limit(ratePerSec), 1)}
}
