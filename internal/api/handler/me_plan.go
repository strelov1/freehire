package handler

import (
	"context"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/ai/plan"
	"github.com/strelov1/freehire/internal/api/apigen"
	"github.com/strelov1/freehire/internal/platform/db"
)

// planHandlers serves the caller's own plan: what they are on, what each metered feature
// allows today, and what they have used of it. Nothing here is a currency and no field
// says "credits".
type planHandlers struct {
	plans   *plan.Store
	queries *db.Queries
}

func newPlanHandlers(plans *plan.Store, queries *db.Queries) *planHandlers {
	return &planHandlers{plans: plans, queries: queries}
}

func (h *planHandlers) register(api fiber.Router, mw middleware, contract fiber.Handler) {
	api.Get("/me/plan", mw.key, contract)
	api.Get("/me/plan/history", mw.key, contract)
}

// planResponse is the whole plan surface: which plan, every metered feature's standing
// today, and the instant they all reset.
type planResponse struct {
	Plan     string    `json:"plan"`
	ResetsAt time.Time `json:"resets_at"`
	// ProUntil is when the caller's PAID plan lapses, absent on the free plan. Read from the
	// stored column, never the billing provider, so the endpoint answers when the provider
	// does not. It carries the tier the caller is actually on (an ultra subscriber gets the
	// ultra entitlement); the `pro_` spelling stays because clients read it.
	ProUntil *time.Time `json:"pro_until,omitempty"`
	// ProSource is where the plan was bought: "stripe", "revenuecat" or "granted". Absent on
	// the free plan, alongside ProUntil. Clients branch on it: Apple forbids sending an in-app
	// subscriber to a web page to cancel, and offering an in-app purchase to a Stripe payer
	// sells the same plan twice.
	ProSource  string          `json:"pro_source,omitempty"`
	Allowances []allowanceView `json:"allowances"`
}

// planSourceOf names which origin conferred the plan: the source column equal to the derived
// pro_until. The check order is the tie-break, stated so the answer is stable; Stripe goes
// first because it is the likeliest purchase surface. It takes the derived instant and this
// tier's three sources rather than the whole row, so adding a tier cannot silently answer
// about the wrong one.
func planSourceOf(derived, stripe, revenuecat, granted pgtype.Timestamptz) string {
	if !derived.Valid {
		return ""
	}
	for _, c := range []struct {
		name  string
		value pgtype.Timestamptz
	}{
		{"stripe", stripe},
		{"revenuecat", revenuecat},
		{"granted", granted},
	} {
		if c.value.Valid && c.value.Time.Equal(derived.Time) {
			return c.name
		}
	}
	// Unreachable while pro_until is GREATEST of the three: something equal to it must exist.
	// Answering "" rather than guessing keeps a schema change from inventing an origin.
	return ""
}

// GetMyPlan returns the caller's plan and today's usage across every metered feature,
// without consuming anything. Cookie or API key; never calls the LLM.
func (h *planHandlers) GetMyPlan(ctx context.Context) (*apigen.GetMyPlanOK, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	out, err := loadPlan(ctx, h.plans, h.queries, userID)
	if err != nil {
		return nil, err
	}
	data := apigen.PlanStanding{
		Plan: apigen.PlanTier(out.Plan), ResetsAt: out.ResetsAt,
		Allowances: make([]apigen.PlanAllowance, len(out.Allowances)),
	}
	if out.ProUntil != nil {
		data.ProUntil = apigen.NewOptDateTime(*out.ProUntil)
	}
	if out.ProSource != "" {
		data.ProSource = apigen.NewOptPlanSource(apigen.PlanSource(out.ProSource))
	}
	for i, a := range out.Allowances {
		data.Allowances[i] = apigen.PlanAllowance{Feature: a.Feature, Used: int64(a.Used),
			Unlimited: a.Unlimited, Enforced: a.Enforced, ResetsAt: a.ResetsAt}
		// Omitted when zero, as the Go type's omitempty did.
		if a.Limit != 0 {
			data.Allowances[i].Limit = apigen.NewOptInt64(int64(a.Limit))
		}
	}
	return &apigen.GetMyPlanOK{Data: data}, nil
}

// loadPlan reads the plan surface for one user, shared by GetMyPlan and the support agent's
// `my_account` tool so the two cannot disagree. Every feature is listed, including untouched
// ones, so a missing row does not read as a feature the caller lacks.
func loadPlan(ctx context.Context, plans *plan.Store, queries *db.Queries, userID int64) (planResponse, error) {
	tier, usage, resets, err := plans.Usage(ctx, userID)
	if err != nil {
		return planResponse{}, err
	}
	out := planResponse{Plan: string(tier), ResetsAt: resets, Allowances: make([]allowanceView, 0, len(usage))}
	// One extra row read, not a provider call. A lapsed or absent value leaves both fields out.
	//
	// A FAILED read FAILS THE RESPONSE: a 200 without pro_source would let a client offer an
	// in-app purchase to someone already paying. A 500 is retried, a wrong 200 is believed.
	sources, err := queries.GetProUntilSources(ctx, userID)
	if err != nil {
		return planResponse{}, err
	}
	// Read the columns of the tier the caller is ACTUALLY on: reading the pro ones for an
	// ultra subscriber answered "ultra" with no until and no source, risking a double sale.
	derived, stripe, revenuecat, granted := sources.ProUntil, sources.ProUntilStripe,
		sources.ProUntilRevenuecat, sources.ProUntilGranted
	if tier == plan.TierUltra {
		derived, stripe, revenuecat, granted = sources.UltraUntil, sources.UltraUntilStripe,
			sources.UltraUntilRevenuecat, sources.UltraUntilGranted
	}
	if derived.Valid && derived.Time.After(time.Now()) {
		when := derived.Time
		out.ProUntil = &when
		out.ProSource = planSourceOf(derived, stripe, revenuecat, granted)
	}
	for _, u := range usage {
		out.Allowances = append(out.Allowances, view(u.Feature, u.Used, u.Limit, u.Unlimited, u.Enforced, resets))
	}
	return out, nil
}
