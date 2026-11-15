package plan

import "time"

// Decision is the answer to "may this user do this now", with where the user stands, when that
// changes, and which kind of no it is. Charge is what the caller must record: 0 for a refusal
// and for an already-paid reference.
type Decision struct {
	Allowed   bool
	Tier      Tier
	Feature   Feature
	Used      int
	Limit     int
	Unlimited bool
	ResetsAt  time.Time
	Charge    int

	// Shadowed marks a decision that WOULD have been a refusal with enforcement on; it is what
	// the shadow run counts.
	Shadowed bool

	// FairUse marks a refusal by the fair-use guard rather than a plan limit. Never show it as a
	// ceiling on what the user bought.
	FairUse bool

	// Enforced says whether this feature's ceiling turns anybody away yet. Clients that pre-block
	// an action need it: hiding a button while enforcement is off refuses what the server allows.
	Enforced bool
}

// decide is the whole rule, as a pure function of the plan, the feature, the day's usage and
// whether this reference was charged before. It touches no database or clock, so every branch
// is testable directly.
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

	// An unpaid tier is refused first, even for an already-charged reference: free gets no
	// metered feature, so no config, env var or ledger row may reopen one, and a lapsed
	// subscription does not keep buying recomputes. Other fields stay populated so the refusal
	// renders like any other.
	if !tier.Paid() {
		return d
	}

	// An already-charged reference is always allowed and never charged again; this outranks every
	// ceiling so a recompute, retry or reconnect is not billed twice.
	if alreadyCharged {
		d.Allowed = true
		return d
	}

	// An unconfigured feature allows nothing, so it shows up refused in a shadow run rather than
	// uncounted.
	if allowance.Limit == 0 && !allowance.Unlimited {
		return d
	}

	// The fair-use guard belongs to any UNLIMITED tier; a tier with a real daily allowance is
	// bounded by it, and reaching the guard there is a misconfiguration. When Unlimited, Limit IS
	// the guard, so a new tier needs no branch here.
	//
	// The enforcement switch does not apply: the guard sits far above human behaviour, so what
	// reaches it is automation, and it protects the gateway rather than a price.
	if allowance.Unlimited && used >= allowance.Limit {
		d.FairUse = true
		return d
	}

	if allowance.Unlimited || used < allowance.Limit {
		d.Allowed, d.Charge, d.Used = true, 1, used+1
		return d
	}

	// Over the plan's ceiling. With enforcement off the consumption is still recorded, so the
	// counter describes what would have been stopped, and the caller goes through.
	if !c.Enforced(f) {
		d.Allowed, d.Charge, d.Used, d.Shadowed = true, 1, used+1, true
	}
	return d
}
