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

func TestConcurrencyLimitedJSONGetter_AcquiresThenDelegates(t *testing.T) {
	inner := &recordingJSONGetter{}
	g := concurrencyLimitedJSONGetter{inner: inner, sem: make(chan struct{}, 2)}

	if err := g.GetJSON(context.Background(), "https://opendata.trudvsem.ru/api/v1/vacancies/region/x", nil); err != nil {
		t.Fatalf("GetJSON returned error: %v", err)
	}
	if len(inner.urls) != 1 {
		t.Fatalf("inner GetJSON called %d times, want 1", len(inner.urls))
	}
	// The slot must be released after the call, so the getter is reusable up to its cap.
	if len(g.sem) != 0 {
		t.Fatalf("semaphore slot not released: len=%d, want 0", len(g.sem))
	}
}

func TestConcurrencyLimitedJSONGetter_CancelledContextShortCircuits(t *testing.T) {
	inner := &recordingJSONGetter{}
	sem := make(chan struct{}, 1)
	sem <- struct{}{} // fill the only slot so the next acquire must wait
	g := concurrencyLimitedJSONGetter{inner: inner, sem: sem}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := g.GetJSON(ctx, "https://opendata.trudvsem.ru/", nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("GetJSON error = %v, want context.Canceled", err)
	}
	if len(inner.urls) != 0 {
		t.Fatalf("inner GetJSON called despite no free slot (%d times)", len(inner.urls))
	}
}

// challengingHTMLGetter always fails the fetch with a WAF ChallengeError.
type challengingHTMLGetter struct{ url string }

func (g challengingHTMLGetter) GetHTML(_ context.Context, _ string) (*html.Node, error) {
	return nil, &ChallengeError{URL: g.url}
}

func TestPacedClinchGetter_PropagatesChallengeError(t *testing.T) {
	g := pacedClinchGetter(challengingHTMLGetter{url: "https://careers.example.com/jobs/x"})

	// The first call is admitted immediately by the limiter's burst, so this exercises the
	// wiring without any timing dependency.
	_, err := g.GetHTML(context.Background(), "https://careers.example.com/jobs/x")
	var chErr *ChallengeError
	if !errors.As(err, &chErr) {
		t.Fatalf("GetHTML error = %v, want the inner *ChallengeError to propagate unchanged", err)
	}
}
