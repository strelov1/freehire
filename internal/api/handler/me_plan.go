package handler

import (
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/ai/plan"
	"github.com/strelov1/freehire/internal/platform/db"
)

// planHandlers serves the caller's own plan: what they are on, what each metered feature
// allows today, and what they have used of it.
//
// It replaces the credits surface. The difference is not the numbers but the noun: a
// balance said "you own 12 of something", and this says "you have used 1 of today's 3
// analyses". Nothing here is a currency, nothing accumulates, and the word "credits"
// appears in no field.
type planHandlers struct {
	plans   *plan.Store
	queries *db.Queries
}

func newPlanHandlers(plans *plan.Store, queries *db.Queries) *planHandlers {
	return &planHandlers{plans: plans, queries: queries}
}

func (h *planHandlers) register(api fiber.Router, mw middleware) {
	api.Get("/me/plan", mw.key, h.GetMyPlan)
	api.Get("/me/plan/history", mw.key, h.GetMyPlanHistory)
}

// planResponse is the whole plan surface: which plan, every metered feature's standing
// today, and the instant they all reset.
type planResponse struct {
	Plan       string          `json:"plan"`
	ResetsAt   time.Time       `json:"resets_at"`
	Allowances []allowanceView `json:"allowances"`
}

// GetMyPlan returns the caller's plan and today's usage across every metered feature,
// without consuming anything. Cookie or API key; never calls the LLM.
//
// Every feature is listed, including ones the caller has not touched — the surface shows
// what the plan IS, and a feature missing from the list because no row exists yet would
// read as a feature they do not have.
func (h *planHandlers) GetMyPlan(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	tier, usage, resets, err := h.plans.Usage(c.Context(), userID)
	if err != nil {
		return err
	}
	out := planResponse{Plan: string(tier), ResetsAt: resets, Allowances: make([]allowanceView, 0, len(usage))}
	for _, u := range usage {
		out.Allowances = append(out.Allowances, view(u.Feature, u.Used, u.Limit, u.Unlimited, u.Enforced, resets))
	}
	return c.JSON(fiber.Map{"data": out})
}
