package billing

import "time"

// subscriber is the provider's record of one account, as much of it as we read. The
// provider's own record — product, status, period, payment method — stays with the
// provider; this type exists to be reduced to a single timestamp and then forgotten.
//
// This is the API v2 customer, not the v1 subscriber. The project's secret key is a v2
// key and v1 refuses it outright ("secret API key incompatible with RevenueCat API V1"),
// so the v1 shape this package first spoke was not a stylistic choice between two working
// options — it was 403 on every call.
//
// The v2 list carries only entitlements that are ACTIVE, which quietly removes work rather
// than adding it: a lapsed, refunded or transferred entitlement simply is not there, and
// the provider has already applied its own grace-period rule to what remains. There is no
// grace field to reconcile because there is nothing left to reconcile.
type subscriber struct {
	ActiveEntitlements struct {
		Items []entitlement `json:"items"`
	} `json:"active_entitlements"`
}

// entitlement is one active entitlement.
//
// ExpiresAt is milliseconds since the epoch, and it is a pointer because null is a real
// value with its own meaning: the entitlement does not expire. See neverExpires.
type entitlement struct {
	EntitlementID string `json:"entitlement_id"`
	ExpiresAt     *int64 `json:"expires_at"`
}

// neverExpires is what an entitlement with no expiry is written as.
//
// The provider reports a non-expiring entitlement — a lifetime purchase — as a null
// expires_date, and users.pro_until is a timestamp with no room for "never". Zero would be
// the natural Go answer and it is the dangerous one: it reads as long expired, so a
// lifetime purchaser would be quietly downgraded to the free plan and nobody would be
// looking for it.
//
// The sentinel is safe because the column is DERIVED rather than accumulated. A refund
// removes the entitlement from the provider's map, the next sync derives the zero time,
// and the sentinel is gone — it can never outlive the entitlement that produced it.
//
// This is NOT hypothetical, which the first draft of this comment assumed. The provider's
// catalogue already carries a `lifetime` non_consumable beside the monthly and yearly
// subscriptions, so the first purchase of one produces a null expiry on day one — and
// nobody would be watching for a subscriber who quietly reads as free.
var neverExpires = time.Date(9999, time.December, 31, 0, 0, 0, 0, time.UTC)

// proUntilFrom reduces a subscriber's entitlements to the single timestamp users.pro_until
// holds: the furthest point at which any pro-conferring entitlement still stands. The zero
// time means no such entitlement exists, which resolves to the free plan.
//
// A LAPSED ENTITLEMENT STILL RETURNS ITS DATE. Whether the plan is over is plan.TierOf's
// question, asked against now; answering it here would erase the only record of when the
// subscription ended.
//
// Within one entitlement the answer is the LATER of its expiry and its grace period. A
// grace period is the provider saying the payment failed but the subscriber is still
// entitled, so reading the expiry alone would take access from someone who has it — over a
// card that needs renewing.
func proUntilFrom(sub subscriber, proEntitlements []string) time.Time {
	var out time.Time
	for _, ent := range sub.ActiveEntitlements.Items {
		if !confers(ent.EntitlementID, proEntitlements) {
			continue
		}
		if until := ent.until(); until.After(out) {
			out = until
		}
	}
	return out
}

// confers reports whether this entitlement identifier is one that grants Pro.
//
// The configured list holds BOTH names the provider uses for the same entitlement — the
// human lookup key ("freehire Pro") and the internal id ("entl…") — because the customer
// payload names it with one of them and which one is not something to find out from a
// production incident. Matching either costs nothing: an identifier that is neither is not
// ours whichever field it came from.
func confers(id string, proEntitlements []string) bool {
	for _, want := range proEntitlements {
		if id == want {
			return true
		}
	}
	return false
}

// until is how far this one entitlement reaches. A null expiry is not expired — see
// neverExpires.
func (e entitlement) until() time.Time {
	if e.ExpiresAt == nil {
		return neverExpires
	}
	return time.UnixMilli(*e.ExpiresAt).UTC()
}
