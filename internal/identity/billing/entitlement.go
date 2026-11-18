package billing

import (
	"slices"
	"time"
)

// subscriber is the provider's record of one customer's subscriptions, as much as we read.
// It is a list because a customer can hold several and the provider does not say which is
// "the" one; the rule picks by reach, not by count.
type subscriber struct {
	Subscriptions []subscription
}

// subscription is one of the customer's subscriptions.
// Status cannot be inferred from the dates. active, trialing and past_due entitle (past_due:
// the provider is still retrying the card); canceled, unpaid and incomplete do not.
type subscription struct {
	// ID and ItemID name this subscription and its first item at the provider, so a plan change
	// can modify it in place. Empty in fixtures that never set them.
	ID     string
	ItemID string
	Status string
	// CurrentPeriodEnd is when the paid period runs out. Zero means unreadable and entitles
	// nobody (see the note on the missing sentinel below).
	CurrentPeriodEnd time.Time
	// CancelAt is set when the customer cancelled but the paid period has not finished. It is
	// NOT a reason to revoke.
	CancelAt time.Time
	// PriceIDs are the prices this subscription is for; one for something other than Pro must
	// not confer Pro.
	PriceIDs []string
}

// entitlingStatuses are the subscription statuses that grant Pro. past_due is deliberate: the
// provider retries a failed card for days, and the period end still bounds the access.
var entitlingStatuses = map[string]bool{
	"active":   true,
	"trialing": true,
	"past_due": true,
}

// There is deliberately no never-expires sentinel. A Stripe subscription always has a period
// end, so an unreadable one means we read the wrong field (the provider moved
// current_period_end onto items once); a sentinel would have made every subscriber permanent
// and silently given the product away. An unreadable end entitles nobody: failing closed costs
// a support message, failing open costs revenue unnoticed.

// proUntilFrom reduces a customer's subscriptions to the timestamp users.pro_until holds: the
// furthest end of any subscription conferring the given prices. Zero means none, i.e. free.
//
// A lapsed subscription still returns its date while its status entitles; whether the plan is
// over is plan.TierOf's question against now. A cancelled-but-not-ended subscription still
// confers: the customer paid for the period.
func proUntilFrom(sub subscriber, proPrices []string) time.Time {
	return bestEntitling(sub, proPrices).CurrentPeriodEnd
}

// entitlement is how far ONE provider's entitlement reaches, per tier. Zero means this
// provider confers that tier on nobody; another origin may still confer it.
type entitlement struct {
	Pro   time.Time
	Ultra time.Time
	// API and Data are the API product's tiers. Only Stripe sells them; other providers leave
	// them zero.
	API  time.Time
	Data time.Time
}

// entitlementFrom resolves every tier of both products from ONE re-read of the subscriber.
// A subscription's tier is decided by which configured price list holds its price, so an unset
// list confers nothing. A price in two lists is a config mistake and fails safe: the account
// resolves to the better tier.
func entitlementFrom(sub subscriber, cfg Config) entitlement {
	return entitlement{
		Pro:   proUntilFrom(sub, cfg.Prices),
		Ultra: proUntilFrom(sub, cfg.UltraPrices),
		API:   proUntilFrom(sub, cfg.APIPrices),
		Data:  proUntilFrom(sub, cfg.APIDataPrices),
	}
}

// checkoutFamily is the pair of price lists decideCheckoutTarget chooses within: the API
// product's when the price is its own, Pro/Ultra otherwise. Without it a Pro subscriber buying
// API access would have their Pro subscription changed into it.
func (c Config) checkoutFamily(priceID string) (a, b []string) {
	if c.isAPIPrice(priceID) {
		return c.APIPrices, c.APIDataPrices
	}
	return c.Prices, c.UltraPrices
}

// isAPIPrice reports whether a price belongs to the API product rather than to the plans.
func (c Config) isAPIPrice(priceID string) bool {
	return slices.Contains(c.APIPrices, priceID) || slices.Contains(c.APIDataPrices, priceID)
}

// billedSubscription is the subscription the billing section describes: the one the PLAN came
// from. It resolves between the two tiers as plan.TierOf does (ultra while live, then pro), not
// by the furthest reach of a merged list: both can stand at once during a portal upgrade, and
// a merged list would show the old Pro subscription while features run on Ultra.
//
// When neither is live it falls back to the furthest of the two: past_due entitles by status
// after its period ran out, and that subscription is what the subscriber needs to see. It also
// returns the price list it selected under, because one subscription can carry an item from
// each tier; choosing ultra and then reading the first listed price would show Pro's amount
// under Ultra's status.
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

// tierFirst orders a subscription's price ids so the selected tier's come first. It orders
// rather than filters: a subscriber on a retired price still pays that price, and filtering
// would turn it into "cannot read your bill".
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

// checkoutTarget is CheckoutURL's decision for one request. The zero value means no entitling
// subscription exists: open a new Checkout Session.
type checkoutTarget struct {
	// SubscriptionID and ItemID name an existing subscription and item to change in place. Set
	// together; both empty (and the flags false) means open a checkout.
	SubscriptionID string
	ItemID         string
	// AlreadyOnPrice is true when the current entitling subscription already carries the
	// requested price.
	AlreadyOnPrice bool
	// Ambiguous is true when the current entitling subscription carries more than one item, so
	// which item the requested price replaces cannot be recovered from price ids. Refuse rather
	// than guess item[0].
	Ambiguous bool
}

// decideCheckoutTarget decides what to do with a requested price given what the customer holds,
// so a second subscription is never opened in parallel. It is pure and DB-free.
//
// It asks across BOTH price lists for the single best-entitling subscription (the furthest
// reaching one when several entitle; extras are left for an operator). It shares bestEntitling's
// "unreadable period end entitles nobody" rule so checkout never disagrees with SyncUser and the
// plan derivation.
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

// combinedPrices merges both tiers' price lists for callers asking whether a subscription
// entitles at all, whichever tier.
func combinedPrices(proPrices, ultraPrices []string) []string {
	combined := make([]string, 0, len(proPrices)+len(ultraPrices))
	combined = append(combined, proPrices...)
	combined = append(combined, ultraPrices...)
	return combined
}

// moreThanOneEntitling reports whether a customer holds more than one entitling subscription
// across both price lists. It counts subscriptions, never price ids: one subscription can
// carry an item of each tier (see billedSubscription).
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

// bestEntitling is the entitling subscription reaching furthest, or the zero subscription. The
// plan derivation and the billing section share it so the plan and the price on screen never
// come from different subscriptions.
func bestEntitling(sub subscriber, proPrices []string) subscription {
	var best subscription
	for _, s := range sub.Subscriptions {
		if s.entitles(proPrices) && s.CurrentPeriodEnd.After(best.CurrentPeriodEnd) {
			best = s
		}
	}
	return best
}

// entitles reports whether this subscription grants Pro right now: an entitling status for a
// price we sell. One predicate so the plan derivation and billing section never disagree. An
// empty price list matches NOTHING, so a deployment that forgot its price makes nobody Pro.
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
