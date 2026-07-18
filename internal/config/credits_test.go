package config

import "testing"

func TestLoadCredits_defaults(t *testing.T) {
	t.Setenv("CREDITS_MONTHLY_GRANT", "")
	t.Setenv("CREDITS_COST_MATCH", "")
	t.Setenv("CREDITS_COST_TAILOR", "")

	got := LoadCredits()
	if got.MonthlyGrant != 20 {
		t.Errorf("MonthlyGrant default = %d, want 20", got.MonthlyGrant)
	}
	if got.CostMatch != 1 {
		t.Errorf("CostMatch default = %d, want 1", got.CostMatch)
	}
	if got.CostTailor != 3 {
		t.Errorf("CostTailor default = %d, want 3", got.CostTailor)
	}
	if got.ContributionReward != 5 {
		t.Errorf("ContributionReward default = %d, want 5", got.ContributionReward)
	}
}

func TestLoadCredits_overrides(t *testing.T) {
	t.Setenv("CREDITS_MONTHLY_GRANT", "50")
	t.Setenv("CREDITS_COST_MATCH", "2")
	t.Setenv("CREDITS_COST_TAILOR", "5")
	t.Setenv("CREDITS_CONTRIBUTION_REWARD", "10")

	got := LoadCredits()
	if got.MonthlyGrant != 50 || got.CostMatch != 2 || got.CostTailor != 5 || got.ContributionReward != 10 {
		t.Errorf("overrides not applied: %+v", got)
	}
}

func TestLoadCredits_clampsInvalid(t *testing.T) {
	// A negative grant would push every balance below any cost and 402 all actions;
	// a zero/negative cost would append meaningless zero-delta debits. Floor grant at
	// 0 (a valid "no free credits" mode) and costs at 1.
	t.Setenv("CREDITS_MONTHLY_GRANT", "-5")
	t.Setenv("CREDITS_COST_MATCH", "0")
	t.Setenv("CREDITS_COST_TAILOR", "-2")
	t.Setenv("CREDITS_CONTRIBUTION_REWARD", "-9")

	got := LoadCredits()
	if got.MonthlyGrant != 0 {
		t.Errorf("MonthlyGrant clamp = %d, want 0", got.MonthlyGrant)
	}
	if got.CostMatch != 1 {
		t.Errorf("CostMatch clamp = %d, want 1", got.CostMatch)
	}
	if got.CostTailor != 1 {
		t.Errorf("CostTailor clamp = %d, want 1", got.CostTailor)
	}
	if got.ContributionReward != 0 {
		t.Errorf("ContributionReward clamp = %d, want 0", got.ContributionReward)
	}
}
