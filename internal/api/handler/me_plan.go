package handler

import (
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgtype"

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
	Plan     string    `json:"plan"`
	ResetsAt time.Time `json:"resets_at"`
	// ProUntil is when the Pro plan lapses, absent on the free plan. It is read from the
	// stored column, never from the billing provider — this endpoint must keep answering
	// when the provider does not.
	ProUntil *time.Time `json:"pro_until,omitempty"`
	// ProSource is where the plan was bought: "stripe", "revenuecat" or "granted". Absent on
	// the free plan, alongside ProUntil.
	//
	// It is behavioural rather than informational, which is why it is here at all. Apple
	// forbids directing an in-app subscriber to a web page to cancel, so a client has to know
	// which surface to send them to; and offering an in-app purchase to somebody already
	// paying through Stripe sells them the same plan twice, which RevenueCat would happily
	// take the money for.
	ProSource  string          `json:"pro_source,omitempty"`
	Allowances []allowanceView `json:"allowances"`
}

// proSourceOf names which origin conferred the plan: the source column equal to the derived
// pro_until.
//
// The order is the tie-break and it is stated rather than left to whichever column is read
// first, so the answer is stable across deployments. A tie means two origins reach the same
// instant, which is rare and harmless; naming Stripe first points a client's cancellation
// advice at the surface the subscriber most likely bought through.
func proSourceOf(row db.GetProUntilSourcesRow) string {
	if !row.ProUntil.Valid {
		return ""
	}
	for _, c := range []struct {
		name  string
		value pgtype.Timestamptz
	}{
		{"stripe", row.ProUntilStripe},
		{"revenuecat", row.ProUntilRevenuecat},
		{"granted", row.ProUntilGranted},
	} {
		if c.value.Valid && c.value.Time.Equal(row.ProUntil.Time) {
			return c.name
		}
	}
	// Unreachable while pro_until is GREATEST of the three: something equal to it must exist.
	// Answering "" rather than guessing keeps a schema change from inventing an origin.
	return ""
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
	// One extra row read, not a call to the provider. A lapsed or absent value simply leaves
	// both fields out, which is what a free-plan caller should see.
	//
	// A FAILED read FAILS THE RESPONSE. Logging it and answering 200 anyway was the first
	// draft, and it is the wrong direction: the tier above came from the same column through
	// plans.Usage, so the answer would say "pro" while carrying no pro_source — and a client
	// that decides whether to offer an in-app purchase from pro_source would then sell Pro to
	// somebody already paying for it. That is the exact double-charge the field exists to
	// prevent, produced by the endpoint meant to prevent it.
	//
	// A partial answer nobody can tell is partial is worse than no answer: a 500 is retried,
	// a wrong 200 is believed.
	sources, err := h.queries.GetProUntilSources(c.Context(), userID)
	if err != nil {
		return err
	}
	if sources.ProUntil.Valid && sources.ProUntil.Time.After(time.Now()) {
		when := sources.ProUntil.Time
		out.ProUntil = &when
		out.ProSource = proSourceOf(sources)
	}
	for _, u := range usage {
		out.Allowances = append(out.Allowances, view(u.Feature, u.Used, u.Limit, u.Unlimited, u.Enforced, resets))
	}
	return c.JSON(fiber.Map{"data": out})
}
