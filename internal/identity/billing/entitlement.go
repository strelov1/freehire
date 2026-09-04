package billing

import (
	"slices"
	"time"
)

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
	// lapse if nothing renews it. The zero time means we could not read it, and every caller
	// treats that as entitling nobody; see the note below on why there is no sentinel.
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

// There is no never-expires sentinel here, and its absence is deliberate — the first draft
// had one and it was a live hazard.
//
// The reasoning that put it there came from a different provider, where a null expiry was a
// documented "this entitlement does not expire". A Stripe SUBSCRIPTION has no such state:
// every recurring subscription has a period end, and a one-off lifetime purchase is not a
// subscription at all and never appears in this list. So an unreadable period end does not
// mean "forever" — it means we read the wrong field.
//
// That is exactly what happened. The provider moved `current_period_end` from the
// subscription onto its items; the client still read the top level, got zero, and a sentinel
// would have turned every subscriber into a permanent one whose cancellation never took
// effect. Silently, and in the direction of giving the product away.
//
// So a subscription whose end cannot be read entitles nobody. Failing closed costs a
// subscriber one support message; failing open costs the revenue and hides itself.

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
	return bestEntitling(sub, proPrices).CurrentPeriodEnd
}

// bestEntitling is the subscription that decides the plan: of the ones that entitle, the one
// reaching furthest. The zero subscription when none does.
//
// One function because two callers need the SAME answer — the plan derivation and the
// billing section — and a subscriber whose plan came from one subscription while the price
// on screen came from another would be looking at a contradiction we published.
func bestEntitling(sub subscriber, proPrices []string) subscription {
	var best subscription
	for _, s := range sub.Subscriptions {
		if s.entitles(proPrices) && s.CurrentPeriodEnd.After(best.CurrentPeriodEnd) {
			best = s
		}
	}
	return best
}

// entitles reports whether this subscription grants Pro right now: a status that entitles,
// for a price we sell.
//
// It is one predicate rather than two checks at each call site because two callers ask it —
// the plan derivation and the billing section — and they must never disagree about which
// subscription is "the" one. A status added to entitlingStatuses is then added in one place.
//
// An empty configured price list matches NOTHING rather than everything: a deployment that
// forgot to name its price should refuse to make anyone Pro, not make everyone Pro.
func (s subscription) entitles(proPrices []string) bool {
	if !entitlingStatuses[s.Status] {
		return false
	}
	for _, have := range s.PriceIDs {
		if slices.Contains(proPrices, have) {
			return true
		}
	}
	return false
}
