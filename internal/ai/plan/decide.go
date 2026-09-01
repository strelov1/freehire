package plan

import "time"

// Decision is the answer to "may this user do this now", plus everything the caller needs
// to explain it: where the user stands, when that changes, and — when the answer is no —
// which kind of no it is.
//
// Charge is what the caller must record. It is 0 for a refusal and 0 for a reference that
// was already paid for, so a caller never has to work out whether this particular yes
// costs anything.
type Decision struct {
	Allowed   bool
	Tier      Tier
	Feature   Feature
	Used      int
	Limit     int
	Unlimited bool
	ResetsAt  time.Time
	Charge    int

	// Shadowed marks a decision that WOULD have been a refusal if the feature's
	// enforcement were on. It is the entire measurement the shadow run collects: how many
	// people a ceiling would have stopped, and where in their day.
	Shadowed bool

	// FairUse marks a refusal by the pro plan's guard rather than by a plan limit. The
	// two must never be confused at the surface: a plan limit is what a subscription is
	// sold against, and the guard is an infrastructure defence a paying user should never
	// be shown as a ceiling on what they bought.
	FairUse bool

	// Enforced says whether this feature's ceiling actually turns anybody away yet. It
	// travels with the decision because the clients that pre-block an action need it: a
	// surface that hides a button on a spent allowance while enforcement is off refuses
	// somebody the server would have let through, and biases the shadow measurement by
	// suppressing exactly the requests it is meant to count.
	Enforced bool
}

// decide is the whole rule, as a pure function of the plan, the feature, what the day
// already holds, and whether this reference was charged before. It touches no database
// and no clock beyond the instant it is handed, which is what lets every branch of it —
// including the ones that are awkward to reach through a transaction — be tested directly.
func (c Config) decide(tier Tier, f Feature, used int, alreadyCharged bool, now time.Time) Decision {
	d := Decision{
		Tier:     tier,
		Feature:  f,
		Used:     used,
		ResetsAt: ResetsAt(now),
		Enforced: c.Enforced(f),
	}
	allowance := c.Allowance(tier, f)
	d.Limit, d.Unlimited = allowance.Limit, allowance.Unlimited

	// A reference already paid for is always allowed and never charged again. This comes
	// first because it outranks every ceiling: a recompute, a retry and a reconnect are
	// the same work the user has already been billed for, and refusing one would charge
	// them for looking at their own result twice.
	if alreadyCharged {
		d.Allowed = true
		return d
	}

	// An unconfigured feature allows nothing. A surface somebody forgot to configure
	// should show up refused in a shadow run rather than uncounted in a bill.
	if allowance.Limit == 0 && !allowance.Unlimited {
		return d
	}

	// The fair-use guard belongs to the pro plan, which is the only one with no ceiling of
	// its own to stop a runaway. A free account is already bounded by its daily allowance,
	// and reaching this number there would mean the allowance was configured above the
	// guard — a misconfiguration to fix, not a caller to accuse of automation.
	//
	// It is not subject to the enforcement switch. Shadow mode protects people from
	// ceilings nobody has verified; the guard sits twenty times above human behaviour, so
	// what reaches it is automation, and what it protects is the gateway rather than a price.
	if tier == TierPro && used >= c.ProFairUse(f) {
		d.FairUse = true
		return d
	}

	if allowance.Unlimited || used < allowance.Limit {
		d.Allowed, d.Charge, d.Used = true, 1, used+1
		return d
	}

	// Over the plan's ceiling. With enforcement on this is the refusal; with it off the
	// consumption is still recorded — a counter that skipped what it would have stopped
	// would describe a day that did not happen — and the caller goes through.
	if !c.Enforced(f) {
		d.Allowed, d.Charge, d.Used, d.Shadowed = true, 1, used+1, true
	}
	return d
}
