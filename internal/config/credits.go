package config

// Credits holds the tunables for the AI-credits points system: the monthly grant
// every user receives and the per-action cost of each metered AI feature. All three
// are env-overridable so the free-tier economics can be tuned without a deploy, and
// so a future paid tier can widen the grant. See the add-ai-credits change.
type Credits struct {
	MonthlyGrant       int // points granted per calendar month (use-it-or-lose-it)
	CostMatch          int // points debited per fresh résumé fit analysis
	CostTailor         int // points debited per new tailored CV
	ContributionReward int // points earned per accepted board contribution (banks, no expiry)
}

// LoadCredits reads the credits tunables from the environment, falling back to the
// free-tier defaults. Values are clamped to safe floors: a negative grant would 402
// every action, and a non-positive cost would append meaningless zero-delta debits.
func LoadCredits() Credits {
	c := Credits{
		MonthlyGrant:       envInt("CREDITS_MONTHLY_GRANT", 20),
		CostMatch:          envInt("CREDITS_COST_MATCH", 1),
		CostTailor:         envInt("CREDITS_COST_TAILOR", 3),
		ContributionReward: envInt("CREDITS_CONTRIBUTION_REWARD", 1),
	}
	if c.MonthlyGrant < 0 {
		c.MonthlyGrant = 0
	}
	if c.CostMatch < 1 {
		c.CostMatch = 1
	}
	if c.CostTailor < 1 {
		c.CostTailor = 1
	}
	if c.ContributionReward < 0 {
		c.ContributionReward = 0
	}
	return c
}
