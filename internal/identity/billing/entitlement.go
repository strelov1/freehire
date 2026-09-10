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
	// ID and ItemID name this subscription and its first item at the provider — what an
	// upgrade or downgrade needs to modify it in place instead of opening a second
	// subscription. Empty when the source is a fixture that never set them.
	ID     string
	ItemID string
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

// entitlement is how far ONE provider's entitlement reaches, per tier. A zero time means
// this provider confers that tier on nobody — which is not the same as the account not
// holding it, since another origin may still confer.
type entitlement struct {
	Pro   time.Time
	Ultra time.Time
}

// entitlementFrom resolves every tier from ONE re-read of the subscriber.
//
// Which tier a subscription confers is decided by which configured price list its price
// appears in, reusing the same filter rather than a second copy of it. That is also why an
// unset list confers nothing rather than everything: `entitles` already refuses an empty
// list, so a deployment that names no Ultra prices simply never resolves anybody to ultra
// and pro behaves exactly as before.
//
// A price could in principle appear in both lists, which would make one subscription confer
// both tiers. That is a configuration mistake rather than a case to encode, and it fails
// safe: the account resolves to ultra, which is the better of the two.
func entitlementFrom(sub subscriber, proPrices, ultraPrices []string) entitlement {
	return entitlement{
		Pro:   proUntilFrom(sub, proPrices),
		Ultra: proUntilFrom(sub, ultraPrices),
	}
}

// billedSubscription is the subscription a subscriber's own billing section describes: the
// one their PLAN came from, whichever tier that is.
//
// It asks each configured price list through the same `bestEntitling` the derivation above
// uses, and then resolves between the two answers exactly as plan.TierOf resolves the tier
// from the columns they become — ultra while it is live, then pro. Merging the two lists and
// taking the furthest reach would be a different rule, and the difference is not academic:
// both tiers can stand at once, which the provider's portal makes ordinary during an upgrade,
// and the Ultra one need not reach furthest. Under a merged list, an account that upgraded
// part-way through a Pro year would be shown its old Pro subscription as "your plan" — Pro's
// price, Pro's renewal date — while every metered feature ran on Ultra's allowance.
//
// When NEITHER is live it falls back to the furthest of the two rather than to nothing, and
// that case is real: `past_due` entitles by status while its period has already run out, so
// the plan has lapsed but the subscription is the very thing the subscriber needs to see.
// It returns the price list it selected under alongside the subscription, because one
// provider subscription can carry an item from each tier — an upgrade that adds the new
// price to the existing subscription rather than opening a second one — and then both
// `bestEntitling` calls answer with that same subscription. Choosing "ultra" and then
// reading whichever price the provider happened to list first would put Pro's amount under
// Ultra's status, which is the contradiction this function exists to prevent.
func billedSubscription(sub subscriber, proPrices, ultraPrices []string, now time.Time) (subscription, []string) {
	pro, ultra := bestEntitling(sub, proPrices), bestEntitling(sub, ultraPrices)
	switch {
	case ultra.CurrentPeriodEnd.After(now):
		return ultra, ultraPrices
	case pro.CurrentPeriodEnd.After(now):
		return pro, proPrices
	case ultra.CurrentPeriodEnd.After(pro.CurrentPeriodEnd):
		return ultra, ultraPrices
	default:
		return pro, proPrices
	}
}

// tierFirst orders a subscription's price ids so the selected tier's come first.
//
// It ORDERS rather than filters, and the difference is the whole point: a subscriber on a
// price we no longer sell is paying THAT price, so a tier list — which holds only what we
// sell today — must decide which id to prefer and never which ids exist. Filtering would
// turn a retired price into "we cannot read your bill".
func tierFirst(ids, tier []string) []string {
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if slices.Contains(tier, id) {
			out = append(out, id)
		}
	}
	for _, id := range ids {
		if !slices.Contains(tier, id) {
			out = append(out, id)
		}
	}
	return out
}

// checkoutTarget is CheckoutURL's whole decision for one request: what to do with the price
// the customer asked for, given what they already have.
//
// The zero value means "no entitling subscription exists — open a new Checkout Session",
// exactly as CheckoutURL always did before this type existed.
type checkoutTarget struct {
	// SubscriptionID and ItemID name an existing subscription and its item to change in
	// place. Set together; both empty (and AlreadyOnPrice, Ambiguous both false) means open
	// a checkout.
	SubscriptionID string
	ItemID         string
	// AlreadyOnPrice is true when the customer's current entitling subscription already
	// carries the requested price. Nothing to do.
	AlreadyOnPrice bool
	// Ambiguous is true when the customer's current entitling subscription carries more
	// than one item — the shape billedSubscription's own comment describes an upgrade
	// through the provider's portal leaving behind — so which item id the requested price
	// should replace cannot be recovered from price ids alone. Refusing rather than guessing
	// item[0] avoids silently changing the wrong one.
	Ambiguous bool
}

// decideCheckoutTarget is the fix for freehire's duplicate-subscription bug, kept pure and
// DB-free: before this existed, CheckoutURL always opened a new Checkout Session regardless
// of what the customer already had, leaving two subscriptions billing them in parallel.
//
// It asks across BOTH configured price lists — a customer's current tier may be either —
// for their single best-entitling subscription, reusing the same selection
// bestEntitling/billedSubscription already make. A customer who already holds more than one
// entitling subscription (the pre-existing-bug state) has this pick the furthest-reaching
// one; the other is left for an operator to clean up by hand.
//
// It deliberately shares bestEntitling's "a subscription whose period end cannot be read
// entitles nobody" rule rather than picking a different one for checkout purposes: a
// selection that disagreed would let this method believe a customer has an entitling
// subscription while the rest of billing (SyncUser, the plan derivation) believes they have
// none — a worse inconsistency than opening a checkout for an account the whole system
// already treats as unentitled.
func decideCheckoutTarget(sub subscriber, requestedPrice string, proPrices, ultraPrices []string) checkoutTarget {
	current := bestEntitling(sub, combinedPrices(proPrices, ultraPrices))
	if current.Status == "" {
		return checkoutTarget{}
	}
	if slices.Contains(current.PriceIDs, requestedPrice) {
		return checkoutTarget{AlreadyOnPrice: true}
	}
	if len(current.PriceIDs) > 1 {
		return checkoutTarget{Ambiguous: true}
	}
	return checkoutTarget{SubscriptionID: current.ID, ItemID: current.ItemID}
}

// combinedPrices merges both tiers' price lists for the callers that must ask "does this
// subscription entitle at ALL", regardless of which tier — deciding what to modify on an
// upgrade, and counting how many subscriptions currently entitle.
func combinedPrices(proPrices, ultraPrices []string) []string {
	combined := make([]string, 0, len(proPrices)+len(ultraPrices))
	combined = append(combined, proPrices...)
	combined = append(combined, ultraPrices...)
	return combined
}

// moreThanOneEntitling reports whether a customer holds more than one active entitling
// subscription across both configured price lists — the duplicate-subscription bug's own
// signature, for a customer it already happened to.
//
// It counts SUBSCRIPTIONS (elements of sub.Subscriptions), never price ids: one Stripe
// subscription can legitimately carry an item of each tier (see billedSubscription's own
// comment on the shape an upgrade that adds a price to the existing subscription leaves
// behind), and that is one subscription, not two.
func moreThanOneEntitling(sub subscriber, proPrices, ultraPrices []string) bool {
	prices := combinedPrices(proPrices, ultraPrices)
	count := 0
	for _, s := range sub.Subscriptions {
		if s.entitles(prices) {
			count++
			if count > 1 {
				return true
			}
		}
	}
	return false
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
