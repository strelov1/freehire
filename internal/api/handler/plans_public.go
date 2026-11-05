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

// planTierView is what one tier allows for one feature in a day. Unlimited makes Limit the
// fair-use guard behind it rather than a ceiling, exactly as the allowance itself does.
//
// The field names are allowanceView's, deliberately. This endpoint and every allowance
// response describe the same idea, and shipping it as `daily` here and `limit` there would
// make a page learn a second vocabulary for one thing — the objection env.go raises about
// two names for one field, in the place a customer reads.
type planTierView struct {
	Limit     int  `json:"limit"`
	Unlimited bool `json:"unlimited"`
}

// planFeatureView is one metered feature as the comparison shows it.
//
// One entry per tier rather than the earlier `free_daily` plus `pro_unlimited`. That pair
// carried an assumption its own comment predicted would not last — "pro_unlimited is always
// true today" — and auto-apply is where it stopped being true: pro has a real ceiling there.
// The page had no way to render that, and would have shown the FREE number in pro's column.
type planFeatureView struct {
	Feature string       `json:"feature"`
	Free    planTierView `json:"free"`
	Pro     planTierView `json:"pro"`
	Ultra   planTierView `json:"ultra"`
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
			Feature: string(f),
			Free:    tierView(h.plans, plan.TierFree, f),
			Pro:     tierView(h.plans, plan.TierPro, f),
			Ultra:   tierView(h.plans, plan.TierUltra, f),
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

// tierView reads one tier's allowance for one feature out of the SAME configuration the
// metering path reads. Hard-coding any of it into the page would create a second source of
// truth about what a plan gives, and it would drift the first time a number moved — in the
// most expensive place, since a promise on a pricing page is one a customer can hold us to.
func tierView(cfg plan.Config, tier plan.Tier, f plan.Feature) planTierView {
	a := cfg.Allowance(tier, f)
	return planTierView{Limit: a.Limit, Unlimited: a.Unlimited}
}
