package billing

import "time"

// subscriber is the provider's record of one account, as much of it as we read. The
// provider's own record — product, status, period, payment method — stays with the
// provider; this type exists to be reduced to a single timestamp and then forgotten.
type subscriber struct {
	Entitlements map[string]entitlement `json:"entitlements"`
	// ManagementURL is where this subscriber cancels. It is the provider's answer, not
	// ours, which is what the delete-account surface must link to: a destination we
	// composed would be wrong the first time they change it.
	ManagementURL string `json:"management_url"`
}

// entitlement is one entry of the provider's entitlements map.
//
// Both dates are pointers because both are genuinely absent sometimes, and the two
// absences mean different things. A missing grace period means there is none. A missing
// expiry means the entitlement does not expire — see neverExpires.
type entitlement struct {
	ExpiresDate            *time.Time `json:"expires_date"`
	GracePeriodExpiresDate *time.Time `json:"grace_period_expires_date"`
	ProductIdentifier      string     `json:"product_identifier"`
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
// We do not currently sell a lifetime product. That is exactly why this has to be right:
// the case will first occur by accident, in a hand-granted entitlement or a promotion, and
// there will be no reason for anyone to check it.
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
	for _, id := range proEntitlements {
		ent, ok := sub.Entitlements[id]
		if !ok {
			continue
		}
		if until := ent.until(); until.After(out) {
			out = until
		}
	}
	return out
}

// until is how far this one entitlement reaches.
func (e entitlement) until() time.Time {
	if e.ExpiresDate == nil {
		return neverExpires
	}
	out := *e.ExpiresDate
	if e.GracePeriodExpiresDate != nil && e.GracePeriodExpiresDate.After(out) {
		out = *e.GracePeriodExpiresDate
	}
	return out
}
