package sponsored

import "time"

// Reason is why a click was not charged; empty for a valid click.
type Reason string

const (
	ReasonBot       Reason = "bot"
	ReasonDuplicate Reason = "duplicate"
	ReasonRate      Reason = "rate"
	ReasonInactive  Reason = "inactive"
	ReasonBudget    Reason = "budget"
)

// DuplicateWindow is how long one visitor's valid click on a campaign suppresses the next.
const DuplicateWindow = 24 * time.Hour

// Click is one click as the redirect endpoint saw it. Bot and RateLimited are the caller's
// verdicts (user-agent list, per-IP limiter).
type Click struct {
	Placement   string
	VisitorHash string
	// PrevVisitorHash is the same visitor's hash under the previous day's salt: the salt rotates
	// at UTC midnight, and without it the 24h duplicate window would reset then.
	PrevVisitorHash string
	Bot             bool
	RateLimited     bool
}

// State is the part of a locked campaign row that decides a click.
type State struct {
	Status        Status
	CPCCents      int64
	DailyCapCents *int64 // nil: no daily cap
	BudgetCents   int64
	SpentCents    int64
}

// Decision is the verdict on one click. Reindex is set when the charge ended the campaign's
// budget or today's cap, so the job must leave the sponsored index.
type Decision struct {
	Valid     bool
	Reason    Reason
	CostCents int64
	Reindex   bool
}

// Decide is the pure charging rule (design D7): reasons in the order bot, duplicate, rate,
// inactive, budget; a valid click costs min(cpc, budget left, daily left).
func Decide(s State, c Click, duplicate bool, dailySpent int64) Decision {
	switch {
	case c.Bot:
		return Decision{Reason: ReasonBot}
	case duplicate:
		return Decision{Reason: ReasonDuplicate}
	case c.RateLimited:
		return Decision{Reason: ReasonRate}
	case s.Status != StatusActive:
		return Decision{Reason: ReasonInactive}
	}
	cost := s.CPCCents
	budgetLeft := s.BudgetCents - s.SpentCents
	cost = min(cost, budgetLeft)
	dailyLeft := int64(-1)
	if s.DailyCapCents != nil {
		dailyLeft = max(*s.DailyCapCents-dailySpent, 0)
		cost = min(cost, dailyLeft)
	}
	if cost <= 0 {
		return Decision{Reason: ReasonBudget}
	}
	hitBudget := cost == budgetLeft
	hitDaily := dailyLeft >= 0 && cost == dailyLeft
	return Decision{Valid: true, CostCents: cost, Reindex: hitBudget || hitDaily}
}
