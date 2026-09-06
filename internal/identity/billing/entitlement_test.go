package billing

import (
	"testing"
	"time"
)

// The two tiers' prices, as the tests configure them. They live here rather than beside the
// tests that sell Ultra because both the untagged tests and the integration ones name them,
// and a const declared in an integration-tagged file exists only under that tag.
const (
	proPrice   = "price_pro_monthly"
	ultraPrice = "price_ultra_monthly"
)

func at(t *testing.T, s string) time.Time {
	t.Helper()
	v, err := time.Parse(time.RFC3339, s)
	if err != nil {
		t.Fatalf("bad test timestamp %q: %v", s, err)
	}
	return v
}

// sub builds a customer carrying the given subscriptions.
func sub(items ...subscription) subscriber {
	return subscriber{Subscriptions: items}
}

// TestProUntilFrom walks every shape a customer's subscriptions arrive in.
//
// Two of these cases are the ones that would cost a paying customer their plan quietly, and
// neither is reachable from the dates alone: a card mid-retry (`past_due`) and a
// cancellation that has not reached the end of the period already paid for.
func TestProUntilFrom(t *testing.T) {
	cases := []struct {
		name   string
		sub    subscriber
		prices []string
		want   string // RFC3339, or "" for the zero time
	}{
		{
			name: "an active subscription",
			sub: sub(subscription{
				Status: "active", CurrentPeriodEnd: at(t, "2026-10-01T00:00:00Z"), PriceIDs: []string{proPrice},
			}),
			want: "2026-10-01T00:00:00Z",
		},
		{
			name: "a trial entitles like a paid period",
			sub: sub(subscription{
				Status: "trialing", CurrentPeriodEnd: at(t, "2026-09-20T00:00:00Z"), PriceIDs: []string{proPrice},
			}),
			want: "2026-09-20T00:00:00Z",
		},
		{
			// The provider retries a failed card for days. Cutting access on the first failed
			// attempt turns a card that needs updating into a cancelled customer.
			name: "a card mid-retry still entitles",
			sub: sub(subscription{
				Status: "past_due", CurrentPeriodEnd: at(t, "2026-10-01T00:00:00Z"), PriceIDs: []string{proPrice},
			}),
			want: "2026-10-01T00:00:00Z",
		},
		{
			// Cancelling says "do not renew", not "refund me". They bought this period.
			name: "cancelled but the paid period has not ended",
			sub: sub(subscription{
				Status:           "active",
				CurrentPeriodEnd: at(t, "2026-10-01T00:00:00Z"),
				CancelAt:         at(t, "2026-10-01T00:00:00Z"),
				PriceIDs:         []string{proPrice},
			}),
			want: "2026-10-01T00:00:00Z",
		},
		{
			name: "a finished cancellation entitles nobody",
			sub: sub(subscription{
				Status: "canceled", CurrentPeriodEnd: at(t, "2026-10-01T00:00:00Z"), PriceIDs: []string{proPrice},
			}),
			want: "",
		},
		{
			name: "an unpaid subscription entitles nobody",
			sub: sub(subscription{
				Status: "unpaid", CurrentPeriodEnd: at(t, "2026-10-01T00:00:00Z"), PriceIDs: []string{proPrice},
			}),
			want: "",
		},
		{
			name: "no subscriptions at all",
			sub:  sub(),
			want: "",
		},
		{
			// Somebody subscribed to something else entirely must not become Pro.
			name: "an active subscription for another price",
			sub: sub(subscription{
				Status: "active", CurrentPeriodEnd: at(t, "2026-10-01T00:00:00Z"), PriceIDs: []string{"price_something_else"},
			}),
			want: "",
		},
		{
			// A deployment that forgot to name its price should make NOBODY Pro, rather than
			// everybody.
			name: "no configured prices matches nothing",
			sub: sub(subscription{
				Status: "active", CurrentPeriodEnd: at(t, "2026-10-01T00:00:00Z"), PriceIDs: []string{proPrice},
			}),
			prices: []string{},
			want:   "",
		},
		{
			name: "two subscriptions; the furthest reach wins",
			sub: sub(
				subscription{Status: "active", CurrentPeriodEnd: at(t, "2026-10-01T00:00:00Z"), PriceIDs: []string{proPrice}},
				subscription{Status: "active", CurrentPeriodEnd: at(t, "2027-03-01T00:00:00Z"), PriceIDs: []string{"price_pro_annual"}},
			),
			prices: []string{proPrice, "price_pro_annual"},
			want:   "2027-03-01T00:00:00Z",
		},
		{
			// The trap, and it bit: the provider moved current_period_end onto the items, the
			// client read the top level, and an earlier "no end means forever" fallback would
			// have made every subscriber permanent with cancellation never taking effect.
			// A period end we cannot read means we read the wrong field, not that it lasts
			// forever — so it entitles nobody.
			name: "a subscription whose end cannot be read entitles nobody",
			sub: sub(subscription{
				Status: "active", PriceIDs: []string{proPrice},
			}),
			want: "",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			prices := tc.prices
			if prices == nil {
				prices = []string{proPrice}
			}
			got := proUntilFrom(tc.sub, prices)

			if tc.want == "" {
				if !got.IsZero() {
					t.Fatalf("want the zero time, got %s", got.Format(time.RFC3339))
				}
				return
			}
			want := at(t, tc.want)
			if !got.Equal(want) {
				t.Fatalf("want %s, got %s", want.Format(time.RFC3339), got.Format(time.RFC3339))
			}
		})
	}
}

// TestBilledSubscription pins which subscription a subscriber's billing section describes
// when more than one could answer. It has to be the one their PLAN came from: a page naming
// a price and a renewal date from a subscription other than the one their allowances run on
// is a contradiction we published about somebody's money.
func TestBilledSubscription(t *testing.T) {
	now := at(t, "2026-09-05T00:00:00Z")
	live := func(price, end string) subscription {
		return subscription{Status: "active", CurrentPeriodEnd: at(t, end), PriceIDs: []string{price}}
	}

	cases := []struct {
		name string
		sub  subscriber
		want string // the price the chosen subscription is for, or "" for no subscription
	}{
		{
			name: "an Ultra subscription is found at all",
			sub:  sub(live(ultraPrice, "2026-10-01T00:00:00Z")),
			want: ultraPrice,
		},
		{
			// The upgrade case, and the reason reach alone is the wrong rule: an annual Pro
			// bought in March still runs when Ultra is added in September, and it reaches
			// further. plan.TierOf resolves that account to ultra, so this section must too.
			name: "an Ultra subscription beside a Pro one that reaches further",
			sub: sub(
				live(proPrice, "2027-03-01T00:00:00Z"),
				live(ultraPrice, "2026-10-01T00:00:00Z"),
			),
			want: ultraPrice,
		},
		{
			// Ultra has run out and Pro has not: the plan is pro, and so is the section.
			name: "a lapsed Ultra beside a live Pro",
			sub: sub(
				live(proPrice, "2027-03-01T00:00:00Z"),
				subscription{
					Status: "past_due", CurrentPeriodEnd: at(t, "2026-08-01T00:00:00Z"),
					PriceIDs: []string{ultraPrice},
				},
			),
			want: proPrice,
		},
		{
			// Neither is live, which is what a card mid-retry looks like — and it is exactly
			// when a subscriber opens this page. The furthest of the two still shows.
			name: "nothing live still describes the furthest subscription",
			sub: sub(
				subscription{
					Status: "past_due", CurrentPeriodEnd: at(t, "2026-07-01T00:00:00Z"),
					PriceIDs: []string{proPrice},
				},
				subscription{
					Status: "past_due", CurrentPeriodEnd: at(t, "2026-08-01T00:00:00Z"),
					PriceIDs: []string{ultraPrice},
				},
			),
			want: ultraPrice,
		},
		{
			name: "a Pro subscriber on a deployment that also sells Ultra",
			sub:  sub(live(proPrice, "2026-10-01T00:00:00Z")),
			want: proPrice,
		},
		{
			name: "no subscription at all",
			sub:  sub(),
			want: "",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := billedSubscription(tc.sub, []string{proPrice}, []string{ultraPrice}, now)
			if tc.want == "" {
				if got.Status != "" {
					t.Fatalf("want the zero subscription, got %+v", got)
				}
				return
			}
			if len(got.PriceIDs) != 1 || got.PriceIDs[0] != tc.want {
				t.Fatalf("want the subscription for %s, got %+v", tc.want, got)
			}
		})
	}
}

// TestProUntilFromIsIdempotent asserts the property the whole design rests on: the column is
// DERIVED from provider state, so applying the same state twice yields the same answer and a
// repeated sync is free.
func TestProUntilFromIsIdempotent(t *testing.T) {
	s := sub(subscription{
		Status: "active", CurrentPeriodEnd: at(t, "2026-10-01T00:00:00Z"), PriceIDs: []string{proPrice},
	})
	first := proUntilFrom(s, []string{proPrice})
	second := proUntilFrom(s, []string{proPrice})
	if !first.Equal(second) {
		t.Fatalf("not idempotent: %s then %s", first, second)
	}
}
