package handler

import (
	"context"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/ai/plan"
	"github.com/strelov1/freehire/internal/api/apigen"
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

func (h *plansHandlers) register(api fiber.Router, contract fiber.Handler) {
	api.Get("/plans", contract)
}

// GetPlans returns what each plan allows and what Pro costs.
//
// The allowances come from the SAME configuration the metering path reads. Hard-coding them
// into the page would create a second source of truth about what a plan gives, and it would
// drift the first time a number moved — silently, and in the most expensive place: a
// promise on a pricing page is one a customer can hold us to.
//
// Each tier gets its own entry rather than a `free_daily` plus `pro_unlimited` pair: auto-apply
// is where pro stopped being unlimited, and the pair had no way to say so. The field names are
// the allowance's (`limit`, `unlimited`), so a page reads one vocabulary for both. `enforced`
// names the features whose ceiling refuses today; a page claiming more would sell a limit
// nobody meets.
func (h *plansHandlers) GetPlans(ctx context.Context) (*apigen.GetPlansOK, error) {
	out := apigen.Plans{
		Features: make([]apigen.PlanFeature, 0, len(plan.AllFeatures())),
		Prices:   []apigen.PlanPrice{},
		Enforced: []string{},
	}
	for _, f := range plan.AllFeatures() {
		out.Features = append(out.Features, apigen.PlanFeature{
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
		ctx, cancel := context.WithTimeout(ctx, applyTimeout)
		defer cancel()
		for _, p := range h.billing.PublicPrices(ctx) {
			out.Prices = append(out.Prices, apigen.PlanPrice{
				ID: p.ID, AmountCents: p.AmountCents, Currency: p.Currency,
				Interval: apigen.PlanPriceInterval(p.Interval), Default: p.Default, Tier: apigen.PlanPriceTier(p.Tier),
			})
		}
	}
	return &apigen.GetPlansOK{Data: out}, nil
}

// tierView reads one tier's allowance for one feature out of the SAME configuration the
// metering path reads.
func tierView(cfg plan.Config, tier plan.Tier, f plan.Feature) apigen.PlanTierAllowance {
	a := cfg.Allowance(tier, f)
	return apigen.PlanTierAllowance{Limit: int64(a.Limit), Unlimited: a.Unlimited}
}
