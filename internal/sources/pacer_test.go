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

// recordingJSONGetter records the URLs it was asked to fetch.
type recordingJSONGetter struct {
	urls []string
}

func (g *recordingJSONGetter) GetJSON(_ context.Context, url string, _ any) error {
	g.urls = append(g.urls, url)
	return nil
}

func TestRateLimitedJSONGetter_GatesThenDelegates(t *testing.T) {
	waiter := &recordingWaiter{}
	inner := &recordingJSONGetter{}
	g := rateLimitedJSONGetter{inner: inner, limiter: waiter}

	if err := g.GetJSON(context.Background(), "https://opendata.trudvsem.ru/api/v1/vacancies/region/x", nil); err != nil {
		t.Fatalf("GetJSON returned error: %v", err)
	}
	if waiter.calls != 1 {
		t.Fatalf("limiter.Wait called %d times, want 1", waiter.calls)
	}
	if len(inner.urls) != 1 {
		t.Fatalf("inner GetJSON called %d times, want 1", len(inner.urls))
	}
}

func TestRateLimitedJSONGetter_WaitErrorShortCircuits(t *testing.T) {
	sentinel := errors.New("rate wait cancelled")
	waiter := &recordingWaiter{err: sentinel}
	inner := &recordingJSONGetter{}
	g := rateLimitedJSONGetter{inner: inner, limiter: waiter}

	if err := g.GetJSON(context.Background(), "https://opendata.trudvsem.ru/", nil); !errors.Is(err, sentinel) {
		t.Fatalf("GetJSON error = %v, want %v", err, sentinel)
	}
	if len(inner.urls) != 0 {
		t.Fatalf("inner GetJSON called despite Wait error (%d times)", len(inner.urls))
	}
}
