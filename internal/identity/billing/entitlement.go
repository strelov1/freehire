package billing

import "time"

// subscriber is the provider's record of what one customer is currently paying for, as
// much of it as we read. The provider's own record — invoices, payment method, proration,
// tax — stays with the provider; this type exists to be reduced to a single timestamp and
// then forgotten.
//
// It is a list of SUBSCRIPTIONS rather than a single one because a customer can hold more
// than one, and because the provider will not tell us which is "the" subscription. The
// rule below picks by reach, not by count.
type subscriber struct {
	Subscriptions []subscription
}

// subscription is one of the customer's subscriptions.
//
// Status matters and cannot be inferred from the dates. `active` and `trialing` both
// entitle; `past_due` entitles too, because the provider is still retrying the card and
// taking access away over a failed retry is the same mistake as ignoring a grace period.
// `canceled`, `unpaid` and `incomplete` do not.
type subscription struct {
	Status string
	// CurrentPeriodEnd is when the paid-for period runs out — the instant access should
	// lapse if nothing renews it.
	CurrentPeriodEnd time.Time
	// CancelAt is set when the customer has cancelled but the period they already paid for
	// has not finished. It is NOT a reason to revoke: they bought that time.
	CancelAt time.Time
	// PriceIDs are the prices this subscription is for. A subscription for something other
	// than Pro must not confer Pro.
	PriceIDs []string
}

// entitlingStatuses are the subscription statuses that grant Pro.
//
// past_due is deliberately here. The provider retries a failed card for days before giving
// up, and a subscriber whose renewal is mid-retry has not stopped paying — cutting them off
// on the first failed attempt turns a card that needs updating into a cancelled customer.
// The subscription's own period end still bounds it, so this cannot grant time nobody
// bought.
var entitlingStatuses = map[string]bool{
	"active":   true,
	"trialing": true,
	"past_due": true,
}

// neverExpires is what a subscription with no end is written as.
//
// The provider always sends a period end for a recurring subscription, so this is reached
// only by a one-off lifetime grant. Zero would be the natural Go answer and it is the
// dangerous one: it reads as long expired, so a lifetime purchaser would be quietly
// downgraded to the free plan and nobody would be looking for it.
//
// The sentinel is safe because the column is DERIVED rather than accumulated. A refund
// removes the subscription, the next sync derives the zero time, and the sentinel is gone —
// it can never outlive what produced it.
var neverExpires = time.Date(9999, time.December, 31, 0, 0, 0, 0, time.UTC)

// proUntilFrom reduces a customer's subscriptions to the single timestamp users.pro_until
// holds: the furthest point at which any Pro-conferring subscription still stands. The zero
// time means no such subscription exists, which resolves to the free plan.
//
// A LAPSED SUBSCRIPTION STILL RETURNS ITS DATE while its status entitles. Whether the plan
// is over is plan.TierOf's question, asked against now; answering it here would erase the
// only record of when the subscription ended.
//
// A CANCELLED-BUT-NOT-YET-ENDED subscription still confers. The customer paid for the
// period they are in; cancelling says "do not renew", not "refund me".
func proUntilFrom(sub subscriber, proPrices []string) time.Time {
	var out time.Time
	for _, s := range sub.Subscriptions {
		if !entitlingStatuses[s.Status] {
			continue
		}
		if !s.coversAny(proPrices) {
			continue
		}
		if until := s.until(); until.After(out) {
			out = until
		}
	}
	return out
}

// coversAny reports whether this subscription is for one of the prices that grant Pro.
//
// An empty configured list matches NOTHING rather than everything: a deployment that forgot
// to name its price should refuse to make anyone Pro, not make everyone Pro.
func (s subscription) coversAny(proPrices []string) bool {
	for _, want := range proPrices {
		for _, have := range s.PriceIDs {
			if have == want {
				return true
			}
		}
	}
	return false
}

// until is how far this one subscription reaches: the end of the period already paid for.
func (s subscription) until() time.Time {
	if s.CurrentPeriodEnd.IsZero() {
		return neverExpires
	}
	return s.CurrentPeriodEnd
}
