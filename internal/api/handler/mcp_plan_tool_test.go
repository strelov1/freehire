package handler

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/strelov1/freehire/internal/ai/assistant"
	"github.com/strelov1/freehire/internal/ai/plan"
)

// get_plan reports what is left of each allowance, and leaves a limit and a remainder out
// for an unlimited feature rather than presenting the fair-use guard as what was bought.
func TestMCPPlanFrom_ReportsRemainingPerFeature(t *testing.T) {
	resets := time.Date(2026, 11, 13, 0, 0, 0, 0, time.UTC)
	until := time.Date(2026, 12, 1, 9, 0, 0, 0, time.UTC)
	got := mcpPlanFrom(planResponse{
		Plan: "pro", ResetsAt: resets, ProUntil: &until, ProSource: "stripe",
		Allowances: []allowanceView{
			{Feature: "auto-apply", Used: 2, Limit: 5, Enforced: true, ResetsAt: resets},
			{Feature: "cover-letter", Used: 7, Limit: 5, Enforced: true, ResetsAt: resets},
			{Feature: "match", Used: 7, Limit: 5, Enforced: false, ResetsAt: resets},
			{Feature: "assistant", Used: 40, Unlimited: true, Enforced: true, ResetsAt: resets},
		},
	}, testMCPOrigin)

	if got.Plan != "pro" || got.PaidUntil == nil || !got.PaidUntil.Equal(until) || !got.ResetsAt.Equal(resets) {
		t.Fatalf("header = %+v", got)
	}
	if got.PlanURL != testMCPOrigin+"/my/plan" {
		t.Errorf("plan_url = %q", got.PlanURL)
	}
	want := []struct {
		feature          string
		limit, remaining *int
		enforced         bool
	}{
		{"auto-apply", ptr(5), ptr(3), true},
		// Over an enforced limit: nothing remains, never a negative number.
		{"cover-letter", ptr(5), ptr(0), true},
		// A shadow limit refuses no paid plan, so it claims no remainder.
		{"match", ptr(5), nil, false},
		{"assistant", nil, nil, true},
	}
	if len(got.Allowances) != len(want) {
		t.Fatalf("allowances = %+v", got.Allowances)
	}
	for i, w := range want {
		a := got.Allowances[i]
		if a.Feature != w.feature || !sameInt(a.Limit, w.limit) || !sameInt(a.Remaining, w.remaining) || a.Enforced != w.enforced {
			t.Errorf("allowance %d = %+v, want %+v", i, a, w)
		}
	}
}

// Billing stays on the website: no payment source, price or invoice reaches a third-party
// model, and a free account carries no end date.
func TestMCPPlanFrom_CarriesNoPaymentDetails(t *testing.T) {
	got := mcpPlanFrom(planResponse{
		Plan: "free", ProSource: "stripe",
		Allowances: []allowanceView{{Feature: "auto-apply", Used: 0, Limit: 0, Enforced: true}},
	}, testMCPOrigin)
	raw, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	for _, banned := range []string{"stripe", "source", "price", "invoice", "paid_until"} {
		if strings.Contains(string(raw), banned) {
			t.Errorf("get_plan result carries %q: %s", banned, raw)
		}
	}
	if got.Allowances[0].Remaining == nil || *got.Allowances[0].Remaining != 0 {
		t.Errorf("free allowance = %+v, want 0 remaining", got.Allowances[0])
	}
}

// The description is what keeps an agent from retrying a refusal on a free account.
func TestMCPPlanTool_IsAReadThatExplainsThePaidGate(t *testing.T) {
	tool := (&assistantHandlers{}).planMCPTool(testMCPOrigin)
	if tool.Name != "get_plan" || tool.Effect != assistant.EffectReads || tool.Title == "" {
		t.Fatalf("tool = %s effect=%d title=%q", tool.Name, tool.Effect, tool.Title)
	}
	wants := []string{"free plan includes no AI", "upgrade", "it may renew", "Takes no arguments"}
	// Every metered feature is named, quoted so that one id inside another ("tailor" in a
	// future "tailor-turns") does not pass for it: a feature added to the plan cannot reach
	// the result under an id the description never explained.
	for _, f := range plan.AllFeatures() {
		wants = append(wants, `"`+string(f)+`"`)
	}
	for _, want := range wants {
		if !strings.Contains(tool.Description, want) {
			t.Errorf("description omits %q: %s", want, tool.Description)
		}
	}
}

func sameInt(a, b *int) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}
