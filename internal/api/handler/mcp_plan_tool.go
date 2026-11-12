package handler

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/strelov1/freehire/internal/ai/assistant"
)

// The plan tool of the signed-in MCP server (openspec/changes/mcp-plan-standing): an agent
// reads what the account may still use today BEFORE it queues metered work, rather than
// learning it from a 402 afterwards. It reads loadPlan, the use case GET /me/plan serves, so
// the two doors cannot disagree about what a caller is on.
//
// Not the support agent's my_account: that one carries prices, the payment source and plan
// history, which are for freehire's own support chat and not for a third-party model.

// mcpPlan is an allowlist, like accountSummary: a field added to the plan surface later
// reaches an agent only by being added here. No price, payment source or invoice — billing
// is the website's, and PlanURL is where the agent sends the person for it.
type mcpPlan struct {
	Plan string `json:"plan"`
	// PaidUntil is when the paid plan the caller is on runs out; absent on the free plan.
	PaidUntil  *time.Time     `json:"paid_until,omitempty"`
	ResetsAt   time.Time      `json:"resets_at"`
	Allowances []mcpAllowance `json:"allowances_today"`
	PlanURL    string         `json:"plan_url"`
}

// mcpAllowance is one feature's standing today. Limit and Remaining are omitted for an
// unlimited feature, for the reason allowanceView omits Limit: the fair-use guard is not
// what the person bought. Every allowance resets at mcpPlan.ResetsAt, so it is stated once.
type mcpAllowance struct {
	Feature   string `json:"feature"`
	Used      int    `json:"used"`
	Limit     *int   `json:"limit,omitempty"`
	Remaining *int   `json:"remaining,omitempty"`
	Unlimited bool   `json:"unlimited"`
	Enforced  bool   `json:"enforced"`
}

// mcpPlanFrom projects the plan surface onto what an agent is told.
func mcpPlanFrom(p planResponse, origin string) mcpPlan {
	out := mcpPlan{
		Plan: p.Plan, PaidUntil: p.ProUntil, ResetsAt: p.ResetsAt,
		Allowances: make([]mcpAllowance, 0, len(p.Allowances)),
		PlanURL:    strings.TrimSuffix(origin, "/") + upgradePath,
	}
	for _, a := range p.Allowances {
		v := mcpAllowance{Feature: a.Feature, Used: a.Used, Unlimited: a.Unlimited, Enforced: a.Enforced}
		if !a.Unlimited {
			limit, remaining := a.Limit, max(a.Limit-a.Used, 0)
			v.Limit, v.Remaining = &limit, &remaining
		}
		out.Allowances = append(out.Allowances, v)
	}
	return out
}

// planMCPTool is served to OAuth grants and full-scope API keys alike: GET /me/plan has
// always been open to a key over REST, and reading a plan changes nothing.
func (h *assistantHandlers) planMCPTool(origin string) assistant.Tool {
	return assistant.Tool{
		Name:       "get_plan",
		Title:      "Read my plan and today's AI allowances",
		Effect:     assistant.EffectReads,
		Idempotent: true,
		Description: "Read the signed-in account's plan (free, pro or ultra), when a paid plan runs out, and for " +
			"each metered AI feature what today's allowance is, how much is used and how much remains. " +
			"Features: tailor (CV tailoring sessions), match (job fit analyses), cover-letter, assistant, " +
			"dictation, auto-apply (applications prepared for review). All reset together at resets_at (UTC). " +
			"A limit that is not enforced is only being measured and refuses nothing yet. " +
			"Read it before starting AI work for the person. The free plan includes no AI features: on free, " +
			"or with nothing remaining, do not retry — tell the person to upgrade at plan_url. " +
			"Prices and payments are not shown here; they are on plan_url too. Takes no arguments.",
		Schema: map[string]any{"type": "object", "properties": map[string]any{}, "additionalProperties": false},
		Run: func(ctx context.Context, userID int64, _ json.RawMessage) (any, error) {
			if h.plans == nil || h.queries == nil {
				return nil, errors.New("plan information is unavailable")
			}
			p, err := loadPlan(ctx, h.plans, h.queries, userID)
			if err != nil {
				return nil, err
			}
			return mcpPlanFrom(p, origin), nil
		},
	}
}
