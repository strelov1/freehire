package sources

import (
	"context"
	"errors"
	"testing"

	"golang.org/x/net/html"
)

// recordingWaiter is a fake rate-limit gate: it counts Wait calls and can force an error.
type recordingWaiter struct {
	calls int
	err   error
}

func (w *recordingWaiter) Wait(context.Context) error {
	w.calls++
	return w.err
}

// recordingHTMLGetter records the URLs it was asked to fetch and returns a fixed node.
type recordingHTMLGetter struct {
	urls []string
	node *html.Node
}

func (g *recordingHTMLGetter) GetHTML(_ context.Context, url string) (*html.Node, error) {
	g.urls = append(g.urls, url)
	return g.node, nil
}

func TestRateLimitedHTMLGetter_GatesThenDelegates(t *testing.T) {
	waiter := &recordingWaiter{}
	node := &html.Node{}
	inner := &recordingHTMLGetter{node: node}
	g := rateLimitedHTMLGetter{inner: inner, limiter: waiter}

	got, err := g.GetHTML(context.Background(), "https://example.careers-page.com/jobs/x")
	if err != nil {
		t.Fatalf("GetHTML returned error: %v", err)
	}
	if waiter.calls != 1 {
		t.Fatalf("limiter.Wait called %d times, want 1", waiter.calls)
	}
	if len(inner.urls) != 1 {
		t.Fatalf("inner GetHTML called %d times, want 1", len(inner.urls))
	}
	if got != node {
		t.Fatalf("GetHTML did not pass through the inner node")
	}
}

func TestRateLimitedHTMLGetter_WaitErrorShortCircuits(t *testing.T) {
	sentinel := errors.New("rate wait cancelled")
	waiter := &recordingWaiter{err: sentinel}
	inner := &recordingHTMLGetter{node: &html.Node{}}
	g := rateLimitedHTMLGetter{inner: inner, limiter: waiter}

	_, err := g.GetHTML(context.Background(), "https://example.careers-page.com/")
	if !errors.Is(err, sentinel) {
		t.Fatalf("GetHTML error = %v, want %v", err, sentinel)
	}
	if len(inner.urls) != 0 {
		t.Fatalf("inner GetHTML called despite Wait error (%d times)", len(inner.urls))
	}
}
