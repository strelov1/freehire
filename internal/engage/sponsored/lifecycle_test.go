package sponsored

import "testing"

func TestCanTransition(t *testing.T) {
	allowed := map[Status][]Status{
		StatusDraft:          {StatusPendingPayment, StatusPendingReview, StatusEnded},
		StatusPendingPayment: {StatusPendingReview, StatusEnded},
		StatusPendingReview:  {StatusActive, StatusRejected, StatusEnded},
		StatusActive:         {StatusPaused, StatusExhausted, StatusEnded},
		StatusPaused:         {StatusActive, StatusEnded},
		StatusExhausted:      {StatusActive, StatusEnded},
		StatusRejected:       {},
		StatusEnded:          {},
	}
	for from, tos := range allowed {
		ok := map[Status]bool{}
		for _, to := range tos {
			ok[to] = true
		}
		for to := range allowed {
			if got := CanTransition(from, to); got != ok[to] {
				t.Errorf("CanTransition(%s -> %s) = %v, want %v", from, to, got, ok[to])
			}
		}
	}
	if CanTransition("bogus", StatusActive) {
		t.Error("unknown status must not transition")
	}
}

func i64(v int64) *int64 { return &v }

func TestDecide(t *testing.T) {
	active := State{Status: StatusActive, CPCCents: 50, BudgetCents: 1000, SpentCents: 0}
	cases := []struct {
		name       string
		state      State
		click      Click
		dup        bool
		dailySpent int64
		want       Decision
	}{
		{"valid full cpc", active, Click{}, false, 0, Decision{Valid: true, CostCents: 50}},
		{"bot", active, Click{Bot: true}, false, 0, Decision{Reason: ReasonBot}},
		{"duplicate", active, Click{}, true, 0, Decision{Reason: ReasonDuplicate}},
		{"rate", active, Click{RateLimited: true}, false, 0, Decision{Reason: ReasonRate}},
		{"inactive paused", State{Status: StatusPaused, CPCCents: 50, BudgetCents: 1000}, Click{}, false, 0, Decision{Reason: ReasonInactive}},
		{"bot beats inactive", State{Status: StatusPaused, CPCCents: 50, BudgetCents: 1000}, Click{Bot: true}, false, 0, Decision{Reason: ReasonBot}},
		{"budget gone", State{Status: StatusActive, CPCCents: 50, BudgetCents: 100, SpentCents: 100}, Click{}, false, 0, Decision{Reason: ReasonBudget}},
		{"capped by budget left, exhausts", State{Status: StatusActive, CPCCents: 50, BudgetCents: 130, SpentCents: 100}, Click{}, false, 0, Decision{Valid: true, CostCents: 30, Reindex: true}},
		{"exact budget exhausts", State{Status: StatusActive, CPCCents: 50, BudgetCents: 50}, Click{}, false, 0, Decision{Valid: true, CostCents: 50, Reindex: true}},
		{"daily cap partial, caps today", State{Status: StatusActive, CPCCents: 50, BudgetCents: 1000, DailyCapCents: i64(100)}, Click{}, false, 80, Decision{Valid: true, CostCents: 20, Reindex: true}},
		{"daily cap reached", State{Status: StatusActive, CPCCents: 50, BudgetCents: 1000, DailyCapCents: i64(100)}, Click{}, false, 100, Decision{Reason: ReasonBudget}},
		{"daily cap room left", State{Status: StatusActive, CPCCents: 50, BudgetCents: 1000, DailyCapCents: i64(100)}, Click{}, false, 10, Decision{Valid: true, CostCents: 50}},
	}
	for _, c := range cases {
		if got := Decide(c.state, c.click, c.dup, c.dailySpent); got != c.want {
			t.Errorf("%s: got %+v, want %+v", c.name, got, c.want)
		}
	}
}
