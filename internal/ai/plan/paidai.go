package plan

import (
	"context"
	"errors"
)

// ErrPaymentRequired is RequirePaid's answer for a caller on no paid plan. Unlike ErrRefused (a
// spent allowance), waiting for midnight does not change it.
var ErrPaymentRequired = errors.New("plan: a paid plan is required")

// RequirePaid answers whether this caller may have a model call made for them at all. Every
// user-initiated model call needs a live Pro or Ultra entitlement: a product invariant, not an
// allowance. A metered action still draws on its own daily allowance afterwards.
//
// A failed read returns the raw error and no tier. The caller must refuse (503), never
// authorise: unlike the meters, this fails closed because it protects model spend.
func (s *Store) RequirePaid(ctx context.Context, userID int64) (Tier, error) {
	tier, err := s.Tier(ctx, userID)
	if err != nil {
		return "", err
	}
	if !tier.Paid() {
		return tier, ErrPaymentRequired
	}
	return tier, nil
}

// paidAIKey is the context key WithPaidAI sets. Unexported, so the only way to mark a
// context is the function below.
type paidAIKey struct{}

// WithPaidAI marks ctx as carrying a verified paid entitlement. Call it ONLY after RequirePaid
// answered yes for the user the work belongs to. The handler's user-call binding checks the
// mark before handing out a model client, so an ungated route fails closed.
func WithPaidAI(ctx context.Context) context.Context {
	return context.WithValue(ctx, paidAIKey{}, true)
}

// PaidAI reports whether ctx was marked by WithPaidAI.
func PaidAI(ctx context.Context) bool {
	v, _ := ctx.Value(paidAIKey{}).(bool)
	return v
}

// freeAIKey is the context key WithFreeAI sets, unexported for the same reason as paidAIKey.
type freeAIKey struct{}

// WithFreeAI marks ctx as carrying a model call the product gives every account, paid or not.
// It is the one named exception to RequirePaid, and a mark rather than a missing check so the
// user-call binding still fails closed for everything else. Current uses:
//   - parsing a stored CV (the experience bank is empty without it);
//   - turning a written search description into filters ("Describe with AI");
//   - the support agent (FeatureSupport), bounded by its own allowance and a global daily budget;
//   - the CV-first onboarding read (POST /onboarding/cv), the one call an anonymous visitor gets:
//     it stores nothing and the gateway bounds guests per address.
func WithFreeAI(ctx context.Context) context.Context {
	return context.WithValue(ctx, freeAIKey{}, true)
}

// FreeAI reports whether ctx was marked by WithFreeAI.
func FreeAI(ctx context.Context) bool {
	v, _ := ctx.Value(freeAIKey{}).(bool)
	return v
}
