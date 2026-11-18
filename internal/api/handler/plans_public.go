package handler

import (
	"context"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/ai/plan"
	"github.com/strelov1/freehire/internal/api/apigen"
	"github.com/strelov1/freehire/internal/identity/billing"
)

// plansHandlers serve the public plan comparison: what each plan allows and what buying
// costs. PUBLIC and unauthenticated, since a pricing page behind an account cannot do its job.
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
// Allowances come from the SAME configuration the metering path reads; hard-coding them would
// add a second source of truth that drifts, on a pricing page a customer can hold us to.
//
// Each tier gets its own entry rather than a `free_daily`/`pro_unlimited` pair, since
// auto-apply is where pro stopped being unlimited. Field names are the allowance's (`limit`,
// `unlimited`). `enforced` names the features whose ceiling refuses today; claiming more
// would sell a limit nobody meets.
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
