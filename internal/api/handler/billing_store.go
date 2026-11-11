package handler

import (
	"context"
	"errors"
	"log"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/api/apigen"
	"github.com/strelov1/freehire/internal/identity/billing"
)

// storeSyncTimeout bounds the one provider call the sync route makes. Shorter than the
// webhook's, because a person is waiting on this one: a paywall that has already taken their
// money and is now spinning is worse than one that says "a moment" and lets them retry.
const storeSyncTimeout = 8 * time.Second

// registerStore mounts the App Store and Google Play routes, or mounts nothing.
//
// Independent of Stripe's block above it: the two providers are configured separately, and a
// deployment may have either, both or neither.
func (h *billingHandlers) registerStore(api fiber.Router, mw middleware, contract fiber.Handler) {
	if h.store == nil || !h.store.Enabled() {
		return
	}
	// Unauthenticated, and authenticated by RevenueCat's HMAC instead — the same arrangement
	// as Stripe's webhook, over a different header with a different window.
	api.Post("/billing/revenuecat/webhook", webhookFor(h.store))
	// Cookie only: this is the one route a caller can use to make the
	// server call a third party on demand.
	api.Post("/billing/revenuecat/sync", mw.cookie, contract)
}

// SyncStore re-reads the caller's own store subscription and writes their source column.
//
// It exists because of a gap nothing else closes. A store purchase completes on the device
// and the webhook arrives afterwards; if that delivery is one of the ones that never lands,
// RevenueCat gives up 80 minutes later and the reconciler is the next chance. Between those
// two facts sits somebody who has just paid and is looking at a paywall.
//
// IT NAMES NOBODY. The account is the session's, and a user id in the body or the query is
// ignored rather than honoured — a route that accepted one would be a way to ask us to call a
// third party once per name, and a way to write another account's plan column.
func (h *billingHandlers) SyncBillingStore(ctx context.Context) (*apigen.SyncBillingStoreOK, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(ctx, storeSyncTimeout)
	defer cancel()

	switch err := h.store.SyncCaller(ctx, userID); {
	case err == nil:
		return &apigen.SyncBillingStoreOK{Data: apigen.BillingStoreSync{Status: "synced"}}, nil

	case errors.Is(err, billing.ErrNoSubscription):
		// RevenueCat was ASKED and holds nothing for this account — it has never bought through
		// an app. Not an error: the honest answer is "nothing of yours to read".
		//
		// SyncCaller rather than SyncUser is what makes that sentence true. The guarded path
		// would have answered this without asking anybody, which for a first-time buyer whose
		// webhook was lost is the difference between "you are Pro" and "you are not a
		// subscriber" — and no reconciler pass would ever have found them either.
		return &apigen.SyncBillingStoreOK{Data: apigen.BillingStoreSync{Status: "no_subscription"}}, nil

	default:
		// The provider is slow or down. The purchase is not lost — RevenueCat holds it and the
		// reconciler will find it — so this is a "try again", not a failure of the payment.
		log.Printf("billing: store sync for user %d failed: %v", userID, err)
		return nil, fiber.NewError(fiber.StatusServiceUnavailable, "could not reach the store provider")
	}
}
