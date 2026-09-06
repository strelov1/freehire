package billing

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
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

// subscribedTo answers the provider with one active subscription per price, and the price
// behind each. The unix timestamps are the period ends, so a test says in one line which
// subscriptions stand and how far each reaches.
func subscribedTo(prices map[string]int64) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/subscriptions" {
			items := make([]string, 0, len(prices))
			for id, end := range prices {
				items = append(items, fmt.Sprintf(
					`{"status":"active","items":{"data":[{"current_period_end":%d,"price":{"id":%q}}]}}`, end, id))
			}
			_, _ = fmt.Fprintf(w, `{"object":"list","data":[%s]}`, strings.Join(items, ","))
			return
		}
		if id, ok := strings.CutPrefix(r.URL.Path, "/prices/"); ok {
			// The amount identifies the price on the way back, so an assertion on it says
			// which subscription the section chose.
			_, _ = fmt.Fprintf(w, `{"id":%q,"unit_amount":%d,"currency":"usd","recurring":{"interval":"month"}}`,
				id, priceAmounts[id])
			return
		}
		_, _ = w.Write([]byte(`{"object":"list","data":[]}`))
	}
}

// priceAmounts is what each configured price charges, in cents — the two tiers' real
// figures, so a wrong choice reads as the wrong plan rather than as a wrong number.
var priceAmounts = map[string]int64{proPrice: 500, ultraPrice: 1900}

// Period ends far enough apart to tell the rules apart: both are live, and the Pro one
// reaches further.
const (
	endsIn2030 = 1893456000
	endsIn2100 = 4102444800
)

// TestSubscriptionOverviewDescribesAnUltraSubscription covers the tier this section could not
// see. Ultra's price ids live in a list of their own, so reading only the Pro one left an
// Ultra subscriber's own billing page answering "no subscription": a 404 with no log, and no
// status, no amount, no renewal date and no receipts for the more expensive plan.
func TestSubscriptionOverviewDescribesAnUltraSubscription(t *testing.T) {
	// serviceWithProvider leaves this list alone, so a test that sells Ultra sets it — the
	// state of any deployment where the tier is actually on sale.
	t.Setenv("STRIPE_ULTRA_PRICE_IDS", ultraPrice)
	s := serviceWithProvider(t, subscribedTo(map[string]int64{ultraPrice: endsIn2030}))

	out, err := s.overviewFor(context.Background(), "cus_9")
	if err != nil {
		t.Fatalf("an Ultra subscriber has no billing section: %v", err)
	}
	if out.Status != "active" || out.AmountCents != priceAmounts[ultraPrice] {
		t.Fatalf("overview read wrong: %+v", out)
	}
	if out.RenewsAt == nil || out.RenewsAt.Unix() != endsIn2030 {
		t.Fatalf("want the Ultra subscription's renewal date, got %v", out.RenewsAt)
	}
}

// TestSubscriptionOverviewDescribesTheTierThePlanCameFrom is the upgrade case. Both
// subscriptions stand — the provider's portal makes that ordinary — and the older Pro one
// reaches further, so a rule picking by reach shows Pro's price and Pro's date to somebody
// whose allowances are all running on Ultra.
func TestSubscriptionOverviewDescribesTheTierThePlanCameFrom(t *testing.T) {
	t.Setenv("STRIPE_ULTRA_PRICE_IDS", ultraPrice)
	s := serviceWithProvider(t, subscribedTo(map[string]int64{
		proPrice:   endsIn2100,
		ultraPrice: endsIn2030,
	}))

	out, err := s.overviewFor(context.Background(), "cus_9")
	if err != nil {
		t.Fatalf("want a billing section, got %v", err)
	}
	if out.AmountCents != priceAmounts[ultraPrice] {
		t.Fatalf("the section describes the %d-cent subscription; the plan is Ultra, which "+
			"charges %d", out.AmountCents, priceAmounts[ultraPrice])
	}
	if out.RenewsAt == nil || out.RenewsAt.Unix() != endsIn2030 {
		t.Fatalf("want the Ultra subscription's renewal date, got %v", out.RenewsAt)
	}
}
