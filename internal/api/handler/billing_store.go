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

// storeSyncTimeout bounds the one provider call the sync route makes. Shorter than the webhook's: a person is waiting on it.
const storeSyncTimeout = 8 * time.Second

// registerStore mounts the App Store and Google Play routes, or nothing. Configured independently of Stripe.
func (h *billingHandlers) registerStore(api fiber.Router, mw middleware, contract fiber.Handler) {
	if h.store == nil || !h.store.Enabled() {
		return
	}
	// Unauthenticated; authenticated by RevenueCat's HMAC, like Stripe's webhook.
	api.Post("/billing/revenuecat/webhook", webhookFor(h.store))
	// Cookie only: the one route a caller can use to make the server call a third party on demand.
	api.Post("/billing/revenuecat/sync", mw.cookie, contract)
}

// SyncStore re-reads the caller's own store subscription and writes their source column. It closes the gap between a purchase
// completing on the device and a webhook that never lands (RevenueCat gives up after 80 minutes).
// IT NAMES NOBODY: the account is the session's, and a user id in the body or query is ignored. Honouring one would let
// callers trigger a third-party call per name and write another account's plan column.
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
		// RevenueCat was ASKED and holds nothing for this account. Not an error: "nothing of yours to read".
		// SyncCaller, not SyncUser, makes that true: the guarded path would answer without asking, hiding a first-time buyer
		// whose webhook was lost, and no reconciler pass would find them either.
		return &apigen.SyncBillingStoreOK{Data: apigen.BillingStoreSync{Status: "no_subscription"}}, nil

	default:
		// The provider is slow or down. The purchase is not lost (RevenueCat holds it, the reconciler will find it): try again.
		log.Printf("billing: store sync for user %d failed: %v", userID, err)
		return nil, fiber.NewError(fiber.StatusServiceUnavailable, "could not reach the store provider")
	}
}
