package handler

import (
	"context"
	"errors"
	"testing"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/candidate/experience"

	"github.com/strelov1/freehire/internal/ai/plan"
	"github.com/strelov1/freehire/internal/identity/auth"
)

type paidStub struct{ err error }

func (p paidStub) RequirePaid(context.Context, int64) (plan.Tier, error) { return plan.TierFree, p.err }
func (paidStub) Standing(context.Context, int64, plan.Feature) (plan.Standing, error) {
	return plan.Standing{}, nil
}
func (paidStub) RecordPaywallHit(context.Context, int64, string, plan.PaywallJob) error { return nil }

// cv_tailor's gate: a free account is a 402, and a plan that cannot be read never authorises.
func TestRequirePaidAI(t *testing.T) {
	for name, tc := range map[string]struct {
		plans paidEntitlement
		want  int
	}{
		"paid":       {paidStub{}, 0},
		"free":       {paidStub{plan.ErrPaymentRequired}, fiber.StatusPaymentRequired},
		"read fails": {paidStub{errors.New("db down")}, fiber.StatusServiceUnavailable},
		"no store":   {nil, fiber.StatusServiceUnavailable},
	} {
		err := requirePaidAI(t.Context(), tc.plans, 7, plan.FeatureTailor)
		var fe *fiber.Error
		switch {
		case tc.want == 0 && err != nil:
			t.Errorf("%s: %v, want authorised", name, err)
		case tc.want != 0 && (!errors.As(err, &fe) || fe.Code != tc.want):
			t.Errorf("%s: %v, want %d", name, err, tc.want)
		}
	}
}

// A tool's handler sees the caller as a keyed request for that user.
func TestAgentContext(t *testing.T) {
	ctx := auth.AgentContext(t.Context(), 7)
	if id, ok := auth.UserIDFromContext(ctx); !ok || id != 7 || !auth.ViaAPIKeyFromContext(ctx) {
		t.Fatalf("AgentContext = user %d (%v), via key %v", id, ok, auth.ViaAPIKeyFromContext(ctx))
	}
}

// A partial correction keeps a project's label through a kind change, and a passed company
// replaces it.
func TestOverlayKeepsAProjectsLabel(t *testing.T) {
	project := experience.Employment{Kind: experience.KindProject, Company: "Acme", Role: "Engineer"}
	for name, tc := range map[string]struct {
		changes employmentArgs
		want    experience.Employment
	}{
		"kind only": {employmentArgs{Kind: new(experience.KindJob)}, experience.Employment{Kind: experience.KindJob, Company: "Acme", Role: "Engineer"}},
		"rename":    {employmentArgs{Company: new("Beta")}, experience.Employment{Kind: experience.KindProject, Company: "Beta", Role: "Engineer"}},
	} {
		raw, err := overlay(project, tc.changes)
		if err != nil {
			t.Fatal(err)
		}
		var got experience.Employment
		if err := got.UnmarshalJSON(raw); err != nil {
			t.Fatal(err)
		}
		if got.Kind != tc.want.Kind || got.Company != tc.want.Company || got.Role != tc.want.Role {
			t.Errorf("%s: %+v, want %+v", name, got, tc.want)
		}
	}
}
