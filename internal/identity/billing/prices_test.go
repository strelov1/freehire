package billing

import (
	"context"
	"net/http"
	"sync"
	"testing"
	"time"
)

const monthlyPriceBody = `{"id":"price_pro_monthly","unit_amount":500,"currency":"usd","recurring":{"interval":"month"}}`

// pricedService is a Service configured to sell one price, talking to a stub that counts how
// often it is asked.
func pricedService(t *testing.T, h http.HandlerFunc) (*Service, *int) {
	t.Helper()
	var calls int
	var mu sync.Mutex
	s := &Service{cfg: Config{
		APIKey:        "sk_test",
		WebhookSecret: testSecret,
		Prices:        []string{"price_pro_monthly"},
		SiteURL:       "https://freehire.me",
	}}
	s.client = testClient(t, func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		calls++
		mu.Unlock()
		h(w, r)
	})
	return s, &calls
}

func TestPublicPricesCachesASuccess(t *testing.T) {
	s, calls := pricedService(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(monthlyPriceBody))
	})

	for i := range 5 {
		got := s.PublicPrices(context.Background())
		if len(got) != 1 || got[0].AmountCents != 500 || !got[0].Default {
			t.Fatalf("call %d: want the one configured price marked default, got %+v", i, got)
		}
	}
	if *calls != 1 {
		t.Fatalf("the provider must be asked once for five page views, was asked %d times", *calls)
	}
}

// TestPublicPricesBacksOffAfterAFailure is the load rule, and it is why a failure has to
// expire the cache rather than leave it looking stale forever.
//
// The endpoint behind this is public, unauthenticated and unrate-limited, and most of the
// traffic reaching it is crawlers. Without a backoff an unreachable provider turns every one
// of those page views into another call to the provider — arriving exactly when it can least
// take it, and spending the API rate limit the webhook's own reads need.
func TestPublicPricesBacksOffAfterAFailure(t *testing.T) {
	fail := true
	s, calls := pricedService(t, func(w http.ResponseWriter, _ *http.Request) {
		if fail {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(monthlyPriceBody))
	})

	for range 5 {
		if got := s.PublicPrices(context.Background()); len(got) != 0 {
			t.Fatalf("an unreachable provider and nothing cached must yield no prices, got %+v", got)
		}
	}
	if *calls != 1 {
		t.Fatalf("a failed read must be honoured for %s, but the provider was asked %d times", priceRetryAfter, *calls)
	}

	// The backoff expires and the page recovers on its own — it is a pause, not a latch.
	fail = false
	s.prices.until = time.Now().Add(-time.Second)
	if got := s.PublicPrices(context.Background()); len(got) != 1 {
		t.Fatalf("want the price once the provider answers again, got %+v", got)
	}
}

// TestPublicPricesServesTheHeldAnswerWhileRefreshing pins the two properties that keep this
// off the request path: the lock never spans a provider call, and a cache miss sends ONE
// caller to the provider while the rest are answered from what is already held.
//
// The stub blocks until released, so the second caller is genuinely concurrent with an
// in-flight refresh. If claim() queued it instead of answering it, this test deadlocks on
// its own timeout — which is the failure mode in production too, one goroutine per visitor.
func TestPublicPricesServesTheHeldAnswerWhileRefreshing(t *testing.T) {
	release := make(chan struct{})
	entered := make(chan struct{}, 1)
	s, calls := pricedService(t, func(w http.ResponseWriter, _ *http.Request) {
		entered <- struct{}{}
		<-release
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(monthlyPriceBody))
	})

	// Prime the cache with an answer, then expire it so the next read is a miss.
	close(release)
	if got := s.PublicPrices(context.Background()); len(got) != 1 {
		t.Fatalf("priming the cache: want one price, got %+v", got)
	}
	<-entered
	release, s.prices.until = make(chan struct{}), time.Now().Add(-time.Second)

	refreshed := make(chan []PublicPrice, 1)
	go func() { refreshed <- s.PublicPrices(context.Background()) }()
	<-entered // the refresh is now inside the provider call, holding the claim

	done := make(chan []PublicPrice, 1)
	go func() { done <- s.PublicPrices(context.Background()) }()
	select {
	case got := <-done:
		if len(got) != 1 {
			t.Fatalf("a caller arriving during a refresh must get the held answer, got %+v", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("a caller arriving during a refresh waited on it — the cache is serialising visitors behind the provider")
	}

	close(release)
	<-refreshed
	if *calls != 2 {
		t.Fatalf("want one call to prime and one to refresh, got %d", *calls)
	}
}
