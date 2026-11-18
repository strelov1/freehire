package main

import (
	"context"

	"golang.org/x/net/html"
	"golang.org/x/time/rate"
)

// waiter gates a request until the limiter admits it. *rate.Limiter satisfies it; tests
// inject a fake so the gate can be asserted without timing flake. Mirrors the ingest-side
// pacer in internal/ingest/sources.
type waiter interface {
	Wait(ctx context.Context) error
}

// pacedClient wraps the probe transport with a shared limiter so a run's AGGREGATE request rate
// to one platform stays under its per-IP window budget, independent of worker count. The worker
// pool bounds requests in flight, not per minute, and a harvest is far denser than the crawl
// the platforms are used to. A platform that answers with an HTML challenge mid-run gets read
// as a dead board by the prober, so the candidates arriving in the penalty window are never
// judged at all: a silently truncated harvest that looks complete. Every verb is gated, so a
// prober that scrapes HTML or posts a query shares the same bucket.
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
// unchanged when ratePerSec is zero: most platforms answer a whole harvest without complaint
// and pacing only lengthens it.
func paced(c httpClient, ratePerSec float64) httpClient {
	if ratePerSec <= 0 {
		return c
	}
	// Burst 1: the point is the total per window; a larger burst would let a fresh run fire its
	// whole allowance at once, the shape that draws a challenge.
	return pacedClient{inner: c, limiter: rate.NewLimiter(rate.Limit(ratePerSec), 1)}
}

// rateCappedProber is the optional half of the prober contract for a platform whose origin
// refuses a fast client in a way the refusal counter cannot see. Pandapé answers a burst with
// 403, not 429, so every refused probe reads as an answered, absent board and the harvest comes
// back short with every check green. Such a prober names its own ceiling, which the run never
// exceeds whatever -pace says.
type rateCappedProber interface {
	maxRequestsPerSecond() float64
}

// effectivePace is the run's probe rate: the -pace flag, lowered to the prober's own ceiling
// when it declares one. An unpaced flag (0) takes the ceiling outright.
func effectivePace(p prober, flagPace float64) float64 {
	c, ok := p.(rateCappedProber)
	if !ok {
		return flagPace
	}
	ceiling := c.maxRequestsPerSecond()
	if flagPace <= 0 || flagPace > ceiling {
		return ceiling
	}
	return flagPace
}
