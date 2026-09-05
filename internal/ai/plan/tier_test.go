package plan

import (
	"testing"
	"time"
)

var noon = time.Date(2026, 9, 5, 12, 0, 0, 0, time.UTC)

func TestTierOfResolvesTheBetterOfTwoEntitlements(t *testing.T) {
	past := noon.Add(-time.Hour)
	future := noon.Add(time.Hour)
	further := noon.Add(48 * time.Hour)

	for _, tc := range []struct {
		name       string
		pro, ultra time.Time
		want       Tier
		why        string
	}{
		{"nothing at all", time.Time{}, time.Time{}, TierFree,
			"an account holding no entitlement is free"},
		{"only pro", future, time.Time{}, TierPro,
			"a live pro entitlement and no ultra one is pro"},
		{"only ultra", time.Time{}, future, TierUltra,
			"a live ultra entitlement is ultra even with no pro entitlement at all"},
		{"both live", future, further, TierUltra,
			"holding both must never give somebody less than either alone — the portal makes " +
				"two live subscriptions possible during an upgrade"},
		{"both live, pro reaches further", further, future, TierUltra,
			"the better TIER wins, not the further date: a pro entitlement outlasting an " +
				"ultra one does not demote a paying Ultra subscriber"},
		{"ultra lapsed, pro live", future, past, TierPro,
			"an expired ultra entitlement falls back to what is still held"},
		{"both lapsed", past, past, TierFree,
			"nothing live is free"},
		{"ultra exactly now", time.Time{}, noon, TierFree,
			"the comparison is strict, so an entitlement is over at the instant it expires"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := TierOf(tc.pro, tc.ultra, noon); got != tc.want {
				t.Fatalf("TierOf = %q, want %q — %s", got, tc.want, tc.why)
			}
		})
	}
}

func TestAllowanceAnswersForEveryTier(t *testing.T) {
	c := DefaultConfig()

	free := c.Allowance(TierFree, FeatureTailor)
	if free.Unlimited || free.Limit != 2 {
		t.Fatalf("free tailor = %+v, want a real limit of 2", free)
	}

	pro := c.Allowance(TierPro, FeatureTailor)
	if !pro.Unlimited || pro.Limit != 40 {
		t.Fatalf("pro tailor = %+v, want unlimited with the fair-use guard behind it", pro)
	}

	ultra := c.Allowance(TierUltra, FeatureTailor)
	if !ultra.Unlimited || ultra.Limit != 120 {
		t.Fatalf("ultra tailor = %+v, want unlimited with a higher guard", ultra)
	}
}

func TestAnUnconfiguredFeatureAllowsNothingOnEveryTier(t *testing.T) {
	c := DefaultConfig()

	for _, tier := range []Tier{TierFree, TierPro, TierUltra} {
		got := c.Allowance(tier, Feature("no-such-feature"))
		if got.Unlimited || got.Limit != 0 {
			t.Fatalf("%s allowance for an unknown feature = %+v, want nothing — a surface "+
				"somebody forgot to configure should show up refused in a shadow run, not "+
				"free in a bill", tier, got)
		}
	}
}

func TestAutoApplyIsMeteredAndProHasARealCeiling(t *testing.T) {
	c := DefaultConfig()

	free := c.Allowance(TierFree, FeatureAutoApply)
	if free.Unlimited || free.Limit != 0 {
		t.Fatalf("free auto-apply = %+v, want nothing — it is a paid feature today and "+
			"stays one", free)
	}

	pro := c.Allowance(TierPro, FeatureAutoApply)
	if pro.Unlimited {
		t.Fatal("pro auto-apply is unlimited — this is the one feature where pro carries a " +
			"real daily ceiling, and without it the tier above sells nothing")
	}
	if pro.Limit != 3 {
		t.Fatalf("pro auto-apply = %d a day, want 3", pro.Limit)
	}

	ultra := c.Allowance(TierUltra, FeatureAutoApply)
	if !ultra.Unlimited {
		t.Fatal("ultra auto-apply is not unlimited — it is the thing the tier is sold on")
	}
}

func TestAutoApplyEnforcesOnArrival(t *testing.T) {
	c := DefaultConfig()

	if !c.Enforced(FeatureAutoApply) {
		t.Fatal("auto-apply ships in shadow mode — a pro ceiling that only counts leaves " +
			"Ultra selling nothing, and the route it replaces already hard-refuses today")
	}
	for _, f := range []Feature{FeatureTailor, FeatureFit, FeatureAssistant, FeatureDictation, FeatureCoverLetter} {
		if c.Enforced(f) {
			t.Fatalf("%s ships enforcing — every feature but auto-apply is read in shadow "+
				"first, and this change does not turn any of them on", f)
		}
	}
}

func TestUltraIsNeverGivenLessThanPro(t *testing.T) {
	c := DefaultConfig().Enforcing()

	// The tailoring turn ceiling. Pro has none; if Ultra kept one, buying the more
	// expensive plan would cut a tailoring session short where the cheaper one ran on.
	pro := c.decideTurn(TierPro, 1, 500)
	ultra := c.decideTurn(TierUltra, 1, 500)
	if !pro.Allowed || !pro.Unlimited {
		t.Fatalf("pro turn = %+v, want unlimited — this is the behaviour ultra must match", pro)
	}
	if !ultra.Allowed || !ultra.Unlimited {
		t.Fatalf("ultra turn = %+v, want unlimited — a plan that costs more must never "+
			"allow less than the one below it", ultra)
	}

	// And the daily allowances, feature by feature.
	for _, f := range AllFeatures() {
		p, u := c.Allowance(TierPro, f), c.Allowance(TierUltra, f)
		if p.Unlimited && !u.Unlimited {
			t.Fatalf("%s: pro is unlimited and ultra is not", f)
		}
		if !p.Unlimited && !u.Unlimited && u.Limit < p.Limit {
			t.Fatalf("%s: ultra allows %d a day and pro allows %d", f, u.Limit, p.Limit)
		}
	}
}

func TestTheFairUseGuardFollowsWhicheverTierIsUnlimited(t *testing.T) {
	c := DefaultConfig().Enforcing()

	// One under the ultra guard passes; the guard itself refuses. The guard is not subject
	// to the enforcement switch, so this holds whatever PLAN_ENFORCE says.
	if d := c.decide(TierUltra, FeatureTailor, 119, false, noon); !d.Allowed {
		t.Fatalf("an ultra account one under its guard was refused: %+v", d)
	}
	d := c.decide(TierUltra, FeatureTailor, 120, false, noon)
	if d.Allowed || !d.FairUse {
		t.Fatalf("decide at the ultra guard = %+v, want a fair-use refusal — the guard "+
			"belongs to every unlimited tier, not to pro alone", d)
	}
}
