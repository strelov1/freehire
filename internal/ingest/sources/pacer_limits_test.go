package sources

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"

	"golang.org/x/net/html"
)

type blockingCovetoGetter struct {
	covetoGetter
	entered chan struct{}
	release chan struct{}
}

func (g blockingCovetoGetter) GetHTML(context.Context, string) (*html.Node, error) {
	g.entered <- struct{}{}
	<-g.release
	return nil, nil
}

func (blockingCovetoGetter) GetText(context.Context, string) (string, error) {
	return "feed", nil
}

func TestLimitedCovetoGetter_CapsPagesWithoutHoldingFeed(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		const requests = covetoMaxInFlight + 1
		inner := blockingCovetoGetter{entered: make(chan struct{}, requests), release: make(chan struct{})}
		getter := limitedCovetoGetter(inner)
		pages := make(chan error, requests)
		for range requests {
			go func() {
				_, err := getter.GetHTML(t.Context(), "https://example.com/job")
				pages <- err
			}()
		}
		synctest.Wait()
		if got := len(inner.entered); got != covetoMaxInFlight {
			t.Errorf("%d posting requests reached the inner getter, want %d", got, covetoMaxInFlight)
		}

		feed := make(chan string, 1)
		go func() {
			body, err := getter.GetText(t.Context(), "https://example.com/feed")
			if err != nil {
				t.Errorf("GetText: %v", err)
			}
			feed <- body
		}()
		synctest.Wait()
		select {
		case body := <-feed:
			if body != "feed" {
				t.Errorf("feed body = %q, want %q", body, "feed")
			}
		default:
			t.Error("feed request waited for a posting slot")
		}

		close(inner.release)
		for range requests {
			if err := <-pages; err != nil {
				t.Errorf("GetHTML: %v", err)
			}
		}
	})
}

func TestLimitedFriendWorkGetter_UsesOneSlot(t *testing.T) {
	getter, ok := limitedFriendWorkGetter(&recordingJSONGetter{}).(concurrencyLimitedJSONGetter)
	if !ok {
		t.Fatal("FriendWork getter has no concurrency limiter")
	}
	if got := cap(getter.sem); got != 1 {
		t.Errorf("FriendWork allows %d requests in flight, want 1", got)
	}
}

func TestConcurrencyLimitedJSONPoster_AcquiresThenDelegates(t *testing.T) {
	inner := &recordingJSONPoster{}
	poster := concurrencyLimitedJSONPoster{inner: inner, sem: make(inFlight, 1)}
	const url = "https://example.com/jobs"

	if err := poster.PostJSON(t.Context(), url, nil, nil); err != nil {
		t.Fatal(err)
	}
	if len(inner.urls) != 1 || inner.urls[0] != url {
		t.Errorf("PostJSON URLs = %v, want one request to %s", inner.urls, url)
	}
	if got := len(poster.sem); got != 0 {
		t.Errorf("PostJSON left %d slots occupied, want 0", got)
	}
}

func TestConcurrencyLimitedJSONPoster_CancelledContextShortCircuits(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		inner := &recordingJSONPoster{}
		sem := make(inFlight, 1)
		sem <- struct{}{}
		poster := concurrencyLimitedJSONPoster{inner: inner, sem: sem}
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		done := make(chan error, 1)

		go func() { done <- poster.PostJSON(ctx, "https://example.com/jobs", nil, nil) }()
		synctest.Wait()
		cancel()
		synctest.Wait()
		select {
		case err := <-done:
			if !errors.Is(err, context.Canceled) {
				t.Errorf("PostJSON error = %v, want context.Canceled", err)
			}
		default:
			t.Error("PostJSON kept waiting after cancellation")
		}
		if len(inner.urls) != 0 {
			t.Errorf("PostJSON reached the inner poster without a free slot: %v", inner.urls)
		}
		// Unblock a broken implementation before leaving the bubble.
		<-sem
	})
}
