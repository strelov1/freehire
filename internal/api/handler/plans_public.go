package handler

import (
	"context"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/ai/plan"
	"github.com/strelov1/freehire/internal/identity/billing"
)

// plansHandlers serve the public plan comparison: what each plan allows, and what buying
// costs.
//
// It is PUBLIC and unauthenticated, because a pricing page that requires an account cannot
// do the one job a pricing page has.
type plansHandlers struct {
	plans   plan.Config
	billing *billing.Service
}

func newPlansHandlers(cfg plan.Config, svc *billing.Service) *plansHandlers {
	return &plansHandlers{plans: cfg, billing: svc}
}

func (h *plansHandlers) register(api fiber.Router) {
	api.Get("/plans", h.GetPlans)
}

// planFeatureView is one metered feature as the comparison shows it.
type planFeatureView struct {
	Feature   string `json:"feature"`
	FreeDaily int    `json:"free_daily"`
	// ProUnlimited is always true today and is sent anyway, so the page renders from the
	// answer rather than from an assumption that will outlive it.
	ProUnlimited bool `json:"pro_unlimited"`
}

type plansResponse struct {
	Features []planFeatureView     `json:"features"`
	Prices   []billing.PublicPrice `json:"prices"`
	// Enforced names the features whose ceiling actually refuses today. While the shadow run
	// is on this is empty, and a page that claimed otherwise would be selling a limit nobody
	// meets.
	Enforced []string `json:"enforced"`
}

// GetPlans returns what each plan allows and what Pro costs.
//
// The allowances come from the SAME configuration the metering path reads. Hard-coding them
// into the page would create a second source of truth about what a plan gives, and it would
// drift the first time a number moved — silently, and in the most expensive place: a
// promise on a pricing page is one a customer can hold us to.
func (h *plansHandlers) GetPlans(c *fiber.Ctx) error {
	out := plansResponse{
		Features: make([]planFeatureView, 0, len(plan.AllFeatures())),
		Enforced: []string{},
	}
	for _, f := range plan.AllFeatures() {
		out.Features = append(out.Features, planFeatureView{
			Feature:      string(f),
			FreeDaily:    h.plans.FreeDaily(f),
			ProUnlimited: h.plans.Allowance(plan.TierPro, f).Unlimited,
		})
		if h.plans.Enforced(f) {
			out.Enforced = append(out.Enforced, string(f))
		}
	}
	if h.billing != nil {
		ctx, cancel := context.WithTimeout(c.Context(), applyTimeout)
		defer cancel()
		out.Prices = h.billing.PublicPrices(ctx)
	}
	if out.Prices == nil {
		out.Prices = []billing.PublicPrice{}
	}
	return c.JSON(fiber.Map{"data": out})
}
