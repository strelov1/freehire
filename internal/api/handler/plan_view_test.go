package handler

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/strelov1/freehire/internal/ai/plan"
)

// The enforcement flag has to survive the trip to the wire, because the clients that
// pre-block read it and nothing else tells them whether a ceiling is live. Dropped from the
// JSON it reads as false everywhere, which is the safe direction — but silently reinstating
// it as a struct field nobody serialises would be the unsafe one, so this asserts the key
// is really there.
func TestAllowanceViewCarriesEnforcementToTheWire(t *testing.T) {
	resets := time.Date(2026, 9, 16, 0, 0, 0, 0, time.UTC)

	for _, enforced := range []bool{true, false} {
		raw, err := json.Marshal(viewStanding(plan.Standing{
			Feature: plan.FeatureFit, Used: 3, Limit: 3, ResetsAt: resets, Enforced: enforced,
		}))
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		var got map[string]any
		if err := json.Unmarshal(raw, &got); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if _, ok := got["enforced"]; !ok {
			t.Fatalf("the allowance body carries no 'enforced' key: %s — every client then reads a shadow ceiling as a live one", raw)
		}
		if got["enforced"] != enforced {
			t.Errorf("enforced = %v, want %v", got["enforced"], enforced)
		}
	}
}

// A Decision and a Standing describe the same facts and must reach the wire identically:
// a pre-check's refusal and the charge's own are the same refusal to whoever reads them.
func TestDecisionAndStandingRenderTheSameAllowance(t *testing.T) {
	resets := time.Date(2026, 9, 16, 0, 0, 0, 0, time.UTC)

	fromStanding := viewStanding(plan.Standing{
		Feature: plan.FeatureTailor, Used: 2, Limit: 2, ResetsAt: resets, Enforced: true,
	})
	fromDecision := viewDecision(plan.Decision{
		Feature: plan.FeatureTailor, Used: 2, Limit: 2, ResetsAt: resets, Enforced: true,
	})
	if fromStanding != fromDecision {
		t.Errorf("standing rendered %+v and decision %+v; the two refusals must be indistinguishable", fromStanding, fromDecision)
	}
}

// An unlimited caller is sent no limit at all: the number behind it is the fair-use guard,
// and presenting an infrastructure defence as "limit" would sell it as the thing they bought.
func TestUnlimitedAllowanceOmitsTheGuard(t *testing.T) {
	raw, err := json.Marshal(viewStanding(plan.Standing{
		Feature: plan.FeatureAssistant, Used: 12, Limit: 200, Unlimited: true, Enforced: true,
	}))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if _, ok := got["limit"]; ok {
		t.Errorf("an unlimited allowance sent a limit: %s", raw)
	}
}
