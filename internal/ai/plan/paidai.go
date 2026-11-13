package plan

import (
	"context"
	"errors"
)

// ErrPaymentRequired is RequirePaid's answer for a caller on no paid plan. It is distinct
// from ErrRefused, which is a spent allowance: this one means the plan never included any
// of it, and that no amount of waiting for midnight changes the answer.
var ErrPaymentRequired = errors.New("plan: a paid plan is required")

// RequirePaid answers whether this caller may have a model call made for them at all.
//
// Every user-initiated model call needs a live Pro or Ultra entitlement — a product
// invariant, not an allowance: there is no free figure to raise and no environment variable
// that reopens one. A metered action still draws on its own daily allowance afterwards;
// this is the question in front of all of them, including the AI actions that have no
// allowance of their own (extraction, the ATS review, search intent, mail recall, autofill).
//
// A failed read returns the raw error and no tier. The caller must refuse on it (503), never
// authorise: an entitlement nobody could read is not an entitlement, and the fail-open rule
// the meters follow does not apply here — what this protects is model spend, not
// bookkeeping.
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

// WithPaidAI marks ctx as carrying a verified paid entitlement for the request it serves.
//
// Call it ONLY after RequirePaid answered yes for the user the work belongs to. The mark is
// what the user-call binding in internal/api/handler checks before it hands out a model
// client, so a route that makes a model call without having been through the gate fails
// closed on its first request — and in its first test — instead of quietly spending on
// somebody who never paid.
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
//
// It is the one named exception to RequirePaid, and it is a mark rather than a missing check
// so the user-call binding still fails closed for everything else: a call reaches a model
// client only through WithPaidAI or here. Today four calls take it. Parsing a stored CV,
// without which the experience bank stays empty and every paid feature that reads it has
// nothing to work with, including for somebody who uploaded before they paid. And turning a
// written search description into filters ("Describe with AI"): one short call on a cheap
// model, throttled per user, and the way a visitor first finds the catalogue. And the support
// agent (FeatureSupport): a person who cannot reach support about a payment is the one who
// most needs it, so it is bounded by its own flat allowance, a per-session ceiling and a
// global daily budget instead of by a plan. And the CV-first onboarding's read of a CV
// (POST /onboarding/cv), the one model call an anonymous visitor gets: it is what the
// visitor sees before deciding to sign up, it stores nothing, and the operations gateway
// bounds guests per address.
func WithFreeAI(ctx context.Context) context.Context {
	return context.WithValue(ctx, freeAIKey{}, true)
}

// FreeAI reports whether ctx was marked by WithFreeAI.
func FreeAI(ctx context.Context) bool {
	v, _ := ctx.Value(freeAIKey{}).(bool)
	return v
}
