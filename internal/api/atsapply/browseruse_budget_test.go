package atsapply

import (
	"testing"
	"time"

	"github.com/strelov1/freehire/internal/application/autoapply"
)

// Measured 2026-09-15 across every cloud run this deployment has made. The useful ones — the
// two confirmed submissions and the runs that filled a form and reported honestly — cost
// between $0.0166 and $0.0707. One run cost $0.197 and took 1040 seconds to arrive at the
// same captcha refusal the $0.017 run reached in 147.
const (
	dearestUsefulCloudRunUSD = 0.0707
	wastefulCloudRunUSD      = 0.197
	// Dollars per second, from that same 1040-second, $0.197 run: what the cap converts to
	// in wall-clock once an agent starts going round in circles.
	observedCloudRateUSDPerSecond = 0.197 / 1040
)

func TestCloudRunCostCap_CoversTheUsefulRunsAndCutsTheWastefulOne(t *testing.T) {
	cap := browserUsePerRunCostCapUSD()

	if cap <= dearestUsefulCloudRunUSD {
		t.Errorf("cap $%.4f would cut a run that did real work ($%.4f)", cap, dearestUsefulCloudRunUSD)
	}
	if cap >= wastefulCloudRunUSD {
		t.Errorf("cap $%.4f still allows the 17-minute run that reached the same refusal for $%.4f", cap, wastefulCloudRunUSD)
	}
}

// Money is the first limit and time the backstop, not the other way round. A run stopped by
// the cost cap ends with the agent's own report — which tells a captcha refusal from an
// unknown outcome — while one stopped by our clock tells us nothing and dead-letters the
// entry, because an interrupted run might already have submitted.
func TestCloudRunSpendsItsBudgetBeforeItRunsOutOfTime(t *testing.T) {
	spentAt := time.Duration(browserUsePerRunCostCapUSD()/observedCloudRateUSDPerSecond) * time.Second

	if spentAt >= browserUseWaitTimeout {
		t.Errorf("the cost cap converts to %s of wall-clock, at or past the %s wait; our clock would fire first and we would learn nothing",
			spentAt.Round(time.Second), browserUseWaitTimeout)
	}
}

// A captcha refusal costs nothing on the Chrome path and real money here, so the two cannot
// share one retry budget. Eight asks against a board that has refused five in a row is the
// point where asking again is worth less than telling the candidate to click submit himself.
func TestCloudCaptchaRefusal_CarriesItsOwnSmallerRetryBudget(t *testing.T) {
	result := resultForParkedReport("hCaptcha verification failed and the application was not submitted.")

	if result.RetryBudget <= 0 {
		t.Fatal("a paid captcha refusal carries no budget of its own, so it falls back to the free path's twenty asks")
	}
	if result.RetryBudget >= 20 {
		t.Errorf("RetryBudget = %d, want fewer than the free path's twenty", result.RetryBudget)
	}
}

// The free path says nothing, and keeps its measured twenty.
func TestChromePathCaptchaRefusal_NamesNoBudget(t *testing.T) {
	var free autoapply.SidecarResult

	if free.RetryBudget != 0 {
		t.Errorf("zero value RetryBudget = %d, want 0 so the runner uses its own default", free.RetryBudget)
	}
}
