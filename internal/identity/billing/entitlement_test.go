package billing

import (
	"testing"
	"time"
)

const proPrice = "price_pro_monthly"

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
