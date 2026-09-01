package plan

import (
	"testing"
	"time"
)

// enforcing is the config the shadow run ends with: every feature refusing.
func enforcing() Config { return DefaultConfig().Enforcing() }

func TestDecideWithinAllowance(t *testing.T) {
	now := time.Date(2026, 9, 15, 10, 0, 0, 0, time.UTC)
	d := enforcing().decide(TierFree, FeatureFit, 1, false, now)

	if !d.Allowed {
		t.Fatal("a free user one analysis into an allowance of three was refused")
	}
	if d.Charge != 1 {
		t.Errorf("Charge = %d, want 1", d.Charge)
	}
	if d.Used != 2 {
		t.Errorf("Used = %d, want 2 — the decision reports where the user stands after it, not before", d.Used)
	}
	if !d.ResetsAt.Equal(time.Date(2026, 9, 16, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("ResetsAt = %v, want the next UTC midnight", d.ResetsAt)
	}
}

func TestDecideRefusesAtTheAllowance(t *testing.T) {
	now := time.Date(2026, 9, 15, 10, 0, 0, 0, time.UTC)
	d := enforcing().decide(TierFree, FeatureFit, 3, false, now)

	if d.Allowed {
		t.Fatal("a free user who has used all three analyses was allowed a fourth")
	}
	if d.Charge != 0 {
		t.Errorf("a refusal charged %d; nothing is taken from someone who was told no", d.Charge)
	}
	if d.FairUse {
		t.Error("an ordinary plan refusal was reported as a fair-use refusal; one is sold against, the other is not shown at all")
	}
	if d.Used != 3 {
		t.Errorf("Used = %d, want 3 unchanged", d.Used)
	}
}

func TestDecideChargesAnAlreadyPaidRefNothing(t *testing.T) {
	now := time.Date(2026, 9, 15, 10, 0, 0, 0, time.UTC)
	// Over the allowance AND already charged: a recompute of work already paid for must
	// go through, or a user would be punished for looking at their own result again.
	d := enforcing().decide(TierFree, FeatureFit, 99, true, now)

	if !d.Allowed {
		t.Fatal("a recompute of an already-charged reference was refused")
	}
	if d.Charge != 0 {
		t.Errorf("Charge = %d, want 0 — the reference was already paid for", d.Charge)
	}
	if d.Used != 99 {
		t.Errorf("Used = %d, want 99 unchanged", d.Used)
	}
}

func TestDecideShadowRecordsButNeverRefuses(t *testing.T) {
	now := time.Date(2026, 9, 15, 10, 0, 0, 0, time.UTC)
	d := DefaultConfig().decide(TierFree, FeatureFit, 3, false, now) // enforcement off

	if !d.Allowed {
		t.Fatal("shadow mode refused; the point of it is to count what a limit WOULD stop, without stopping anyone")
	}
	if !d.Shadowed {
		t.Error("the decision does not record that it would have refused; that flag is the whole measurement")
	}
	if d.Charge != 1 {
		t.Errorf("Charge = %d, want 1 — shadow mode still records consumption, or the counter describes nothing", d.Charge)
	}
	if d.Enforced {
		t.Error("a shadow decision reports itself enforced; the clients that pre-block read this flag, and one that reads true refuses what the server just allowed")
	}
}

// Enforcement travels with the decision because every surface that pre-blocks needs it. A
// client that can only see used-against-limit hides a button on a spent allowance the
// server would still honour — a wall nobody meant to build, and one that suppresses the
// very requests the shadow run is counting.
func TestDecisionCarriesWhetherTheCeilingIsLive(t *testing.T) {
	now := time.Date(2026, 9, 15, 10, 0, 0, 0, time.UTC)
	d := enforcing().decide(TierFree, FeatureFit, 0, false, now)

	if !d.Enforced {
		t.Error("an enforcing configuration produced a decision that says it is not enforced")
	}
}

// Standing.Refuses is the same rule, asked by whoever holds a standing rather than by the
// store — including the SPA, which receives the flag on the wire.
func TestStandingRefusesOnlyWhenTheCeilingIsLive(t *testing.T) {
	spent := Standing{Feature: FeatureFit, Used: 3, Limit: 3}

	if !spent.Exhausted() {
		t.Fatal("a standing at its limit does not report itself exhausted")
	}
	if spent.Refuses() {
		t.Error("a spent allowance refuses while enforcement is off; that is the shadow run refusing people")
	}
	spent.Enforced = true
	if !spent.Refuses() {
		t.Error("a spent allowance does not refuse with enforcement on")
	}
	unlimited := Standing{Feature: FeatureFit, Used: 500, Unlimited: true, Enforced: true}
	if unlimited.Refuses() {
		t.Error("an unlimited standing refuses; the fair-use guard belongs at the point of use, not here")
	}
}

func TestDecideProHasNoPlanCeiling(t *testing.T) {
	now := time.Date(2026, 9, 15, 10, 0, 0, 0, time.UTC)
	cfg := enforcing()
	d := cfg.decide(TierPro, FeatureFit, cfg.FreeDaily(FeatureFit)+50, false, now)

	if !d.Allowed {
		t.Fatal("a pro user was refused well below the fair-use guard")
	}
	if !d.Unlimited {
		t.Error("a pro decision reports a countable limit; pro is the same product without the ceilings")
	}
}

func TestDecideFairUseGuardStopsAScript(t *testing.T) {
	now := time.Date(2026, 9, 15, 10, 0, 0, 0, time.UTC)
	cfg := enforcing()
	guard := cfg.ProFairUse(FeatureFit)
	d := cfg.decide(TierPro, FeatureFit, guard, false, now)

	if d.Allowed {
		t.Fatal("a pro account past its fair-use guard was allowed; one subscription can drain the gateway")
	}
	if !d.FairUse {
		t.Error("the refusal is not marked as a fair-use one, so it would be shown to the user as a plan limit")
	}
}

func TestFairUseGuardHoldsEvenInShadow(t *testing.T) {
	now := time.Date(2026, 9, 15, 10, 0, 0, 0, time.UTC)
	cfg := DefaultConfig() // enforcement off
	d := cfg.decide(TierPro, FeatureFit, cfg.ProFairUse(FeatureFit), false, now)

	// Shadow mode exists so nobody is refused on an unread number. The guard is not that
	// kind of number: it sits twenty times above human behaviour, so anything reaching it
	// is automation, and it protects the gateway rather than selling a subscription.
	if d.Allowed {
		t.Fatal("the fair-use guard was disabled by shadow mode; shadow protects people from unproven ceilings, not the gateway from a script")
	}
	if !d.FairUse {
		t.Error("the shadow-mode guard refusal is not marked as a fair-use one")
	}
}

func TestDecideRefusesAnUnconfiguredFeature(t *testing.T) {
	now := time.Date(2026, 9, 15, 10, 0, 0, 0, time.UTC)
	d := enforcing().decide(TierFree, Feature("no-such-feature"), 0, false, now)

	if d.Allowed {
		t.Fatal("a feature nobody configured was allowed; forgetting to configure a surface must surface as refused, not as free")
	}
}
