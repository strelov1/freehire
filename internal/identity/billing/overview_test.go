package billing

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

// serviceWithProvider builds an enabled Service whose provider is a stub. No database: the
// rule under test is about what to say, not about whose account asked.
func serviceWithProvider(t *testing.T, h http.HandlerFunc) *Service {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)

	setEnv(t, "sk_test", testSecret, proPrice, "https://freehire.me")
	s := New(ConfigFromEnv(), nil)
	s.client = newClient("sk_test", srv.URL, srv.Client())
	return s
}

// TestSubscriptionOverviewRefusesToInventZeroes covers the two ways this screen could tell
// somebody their card is not being charged when it is.
//
// Both were caught in review, and both share a shape: a partial answer that renders as a
// confident "$0 /" rather than as an absent section. On a screen about money, silence is the
// only honest failure — an amount is a claim, and a zero is the most expensive claim to get
// wrong.
func TestSubscriptionOverviewRefusesToInventZeroes(t *testing.T) {
	t.Run("no eligible subscription is no section", func(t *testing.T) {
		// A former subscriber: the subscription exists but has ended, so nothing entitles.
		s := serviceWithProvider(t, func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`{"object":"list","data":[{"status":"canceled","items":{"data":[{"current_period_end":1,"price":{"id":"price_pro_monthly"}}]}}]}`))
		})
		if _, err := s.overviewFor(context.Background(), "cus_9"); !errors.Is(err, ErrNoSubscription) {
			t.Fatalf("want ErrNoSubscription, got %v", err)
		}
	})

	t.Run("a subscription for a price we do not sell is no section", func(t *testing.T) {
		s := serviceWithProvider(t, func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`{"object":"list","data":[{"status":"active","items":{"data":[{"current_period_end":4102444800,"price":{"id":"price_something_else"}}]}}]}`))
		})
		if _, err := s.overviewFor(context.Background(), "cus_9"); !errors.Is(err, ErrNoSubscription) {
			t.Fatalf("want ErrNoSubscription, got %v", err)
		}
	})

	t.Run("an unreadable price is no section", func(t *testing.T) {
		// The subscription lists fine, but the price behind it cannot be read. We know they
		// are subscribed and NOT what for — which is not the same as knowing it is free.
		s := serviceWithProvider(t, func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/subscriptions" {
				_, _ = w.Write([]byte(`{"object":"list","data":[{"status":"active","items":{"data":[{"current_period_end":4102444800,"price":{"id":"price_pro_monthly"}}]}}]}`))
				return
			}
			w.WriteHeader(http.StatusInternalServerError)
		})
		if _, err := s.overviewFor(context.Background(), "cus_9"); err == nil {
			t.Fatal("want an error when no price resolves, got nil")
		}
	})
}

// TestSubscriptionOverviewReadsTheSubscribedPrice asserts the happy path, and specifically
// that the amount comes from the SUBSCRIPTION's price rather than from configuration —
// somebody on a price we no longer sell is paying that price.
func TestSubscriptionOverviewReadsTheSubscribedPrice(t *testing.T) {
	s := serviceWithProvider(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/subscriptions":
			_, _ = w.Write([]byte(`{"object":"list","data":[{"status":"active","items":{"data":[{"current_period_end":4102444800,"price":{"id":"price_pro_monthly"}}]}}]}`))
		case "/prices/price_pro_monthly":
			_, _ = w.Write([]byte(`{"id":"price_pro_monthly","unit_amount":500,"currency":"usd","recurring":{"interval":"month"}}`))
		default:
			_, _ = w.Write([]byte(`{"object":"list","data":[]}`))
		}
	})

	out, err := s.overviewFor(context.Background(), "cus_9")
	if err != nil {
		t.Fatalf("want no error, got %v", err)
	}
	if out.Status != "active" || out.AmountCents != 500 || out.Interval != "month" {
		t.Fatalf("overview read wrong: %+v", out)
	}
	// Not cancelled, so a renewal date and no end date. The two are mutually exclusive on
	// purpose: "renews on the 4th" beside "you cancelled" is a contradiction.
	if out.RenewsAt == nil || out.EndsAt != nil {
		t.Fatalf("want a renewal date and no end date, got renews=%v ends=%v", out.RenewsAt, out.EndsAt)
	}
}
