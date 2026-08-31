package plan

import (
	"testing"
	"time"
)

func TestTierOf(t *testing.T) {
	now := time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)

	cases := []struct {
		name     string
		proUntil time.Time
		want     Tier
	}{
		{"no subscription ever", time.Time{}, TierFree},
		{"subscription in force", now.Add(time.Hour), TierPro},
		{"subscription lapsed", now.Add(-time.Hour), TierFree},
		// The instant it expires it is over. There is no grace here on purpose: a grace
		// window is a product decision, and burying one in a comparison is how it becomes
		// nobody's decision.
		{"expiring exactly now", now, TierFree},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := TierOf(tc.proUntil, now); got != tc.want {
				t.Errorf("TierOf(%v) = %q, want %q", tc.proUntil, got, tc.want)
			}
		})
	}
}

func TestDayAndReset(t *testing.T) {
	// A moment late on one day and the first moment of the next must land on different
	// keys, or the allowance would not reset at midnight.
	late := time.Date(2026, 9, 15, 23, 59, 59, 0, time.UTC)
	next := time.Date(2026, 9, 16, 0, 0, 0, 0, time.UTC)

	if Day(late).Equal(Day(next)) {
		t.Fatal("23:59:59 and the following midnight share a day key; the allowance would not reset")
	}
	if got, want := Day(late), time.Date(2026, 9, 15, 0, 0, 0, 0, time.UTC); !got.Equal(want) {
		t.Errorf("Day() = %v, want %v", got, want)
	}
	if got, want := ResetsAt(late), next; !got.Equal(want) {
		t.Errorf("ResetsAt() = %v, want %v", got, want)
	}
	// A non-UTC input is answered on the UTC day, because that is the day the counter is
	// keyed by. Reading it in local time would put a user's own reading of "today" out of
	// step with the row that bounds them.
	tokyo := time.FixedZone("JST", 9*3600)
	if got, want := Day(time.Date(2026, 9, 16, 8, 0, 0, 0, tokyo)), time.Date(2026, 9, 15, 0, 0, 0, 0, time.UTC); !got.Equal(want) {
		t.Errorf("Day() of a JST morning = %v, want the UTC day %v", got, want)
	}
}

func TestDefaultConfigMatchesTheSpec(t *testing.T) {
	cfg := DefaultConfig()

	free := map[Feature]int{
		FeatureTailor:    2,
		FeatureFit:       3,
		FeatureAssistant: 10,
		FeatureDictation: 10,
	}
	for feature, want := range free {
		if got := cfg.FreeDaily(feature); got != want {
			t.Errorf("free daily allowance for %q = %d, want %d", feature, got, want)
		}
	}
	if got := cfg.TailorTurnsPerSession; got != 15 {
		t.Errorf("tailor turn ceiling = %d, want 15", got)
	}
}

func TestEnforcementStartsOff(t *testing.T) {
	cfg := DefaultConfig()
	for _, feature := range AllFeatures() {
		if cfg.Enforced(feature) {
			t.Errorf("%q ships enforcing; the numbers come from three weeks of one August under no paywall, "+
				"and refusing on an unread number is what the shadow run exists to avoid", feature)
		}
	}
}

func TestEveryPlanOffersEveryFeature(t *testing.T) {
	cfg := DefaultConfig()
	for _, feature := range AllFeatures() {
		if cfg.FreeDaily(feature) <= 0 {
			t.Errorf("%q is unreachable on the free plan; a plan differs in how much it allows, never in whether the feature exists", feature)
		}
		if cfg.ProFairUse(feature) <= cfg.FreeDaily(feature) {
			t.Errorf("pro's fair-use guard for %q (%d) is not above the free allowance (%d)",
				feature, cfg.ProFairUse(feature), cfg.FreeDaily(feature))
		}
	}
}

func TestAllowanceForTier(t *testing.T) {
	cfg := DefaultConfig()

	free := cfg.Allowance(TierFree, FeatureFit)
	if free.Unlimited || free.Limit != 3 {
		t.Errorf("free fit allowance = %+v, want a limit of 3", free)
	}
	pro := cfg.Allowance(TierPro, FeatureFit)
	if !pro.Unlimited {
		t.Error("pro reports a countable limit; the pro plan has no user-facing ceiling, only a fair-use guard behind it")
	}
	if pro.Limit != cfg.ProFairUse(FeatureFit) {
		t.Errorf("pro fair-use guard = %d, want %d", pro.Limit, cfg.ProFairUse(FeatureFit))
	}
}

func TestUnknownFeatureIsNotSilentlyFree(t *testing.T) {
	cfg := DefaultConfig()
	// A feature nobody configured must not read as "unlimited on every plan" — that is
	// how metering a new surface gets forgotten and discovered in a bill.
	if got := cfg.FreeDaily(Feature("no-such-feature")); got != 0 {
		t.Errorf("an unconfigured feature allows %d a day, want 0", got)
	}
	if a := cfg.Allowance(TierFree, Feature("no-such-feature")); a.Unlimited {
		t.Error("an unconfigured feature reads as unlimited on the free plan")
	}
}
