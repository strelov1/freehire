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
	// PaidUntil is the end of the current paid period — for an auto-renewing subscription the
	// renewal date, not an end of service. Absent on the free plan.
	PaidUntil  *time.Time     `json:"paid_until,omitempty"`
	ResetsAt   time.Time      `json:"resets_at"`
	Allowances []mcpAllowance `json:"allowances_today"`
	PlanURL    string         `json:"plan_url"`
}

// mcpAllowance is one feature's standing today. Limit and Remaining are omitted for an
// unlimited feature, for the reason allowanceView omits Limit: the fair-use guard is not
// what the person bought. Remaining is also omitted for a limit that is not enforced. Every allowance resets at mcpPlan.ResetsAt, so it is stated once.
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
			limit := a.Limit
			v.Limit = &limit
			// A remainder is only a stop sign where the limit refuses; under a shadow limit a
			// paid plan goes on past zero, and "0 remaining" would tell the agent otherwise.
			if a.Enforced {
				remaining := max(a.Limit-a.Used, 0)
				v.Remaining = &remaining
			}
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
		// Phrased around auto-apply, the one metered feature an MCP tool spends; the rest are
		// listed so the agent can read the result, not as something it can start here. The
		// stop rule is the plan's own (internal/ai/plan/decide.go): free is refused whatever
		// the allowance, an unlimited plan's fair-use guard ignores enforcement, and a charge
		// is per posting, so retrying or reviewing an attempt is never charged.
		Description: "Read the signed-in account's plan (\"free\", \"pro\" or \"ultra\") and today's allowance of " +
			"each metered AI feature: limit, used and remaining, all resetting at resets_at (UTC). " +
			"paid_until is the end of the current paid period (it may renew; see plan_url). " +
			"Here the metered feature is \"auto-apply\" (applications prepared for review): starting one for a new " +
			"posting uses one; retrying, reviewing or cancelling an existing attempt, or starting the same posting " +
			"again, uses nothing. The others (\"tailor\", \"match\", \"cover-letter\", \"assistant\", \"dictation\") " +
			"are used on the website. The free plan includes no AI features and is refused every new auto-apply: " +
			"tell the person to upgrade at plan_url rather than retrying. On a paid plan, an enforced limit with " +
			"nothing remaining refuses new starts until resets_at; an unenforced limit is only measured, so it " +
			"carries no remaining. An unlimited feature still has a high fair-use cap. " +
			"Prices and payments are not shown; they are on plan_url. Takes no arguments.",
		Schema: assistant.ClosedEmptySchema,
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
