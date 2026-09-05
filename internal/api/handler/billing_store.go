package handler

import (
	"context"
	"errors"
	"log"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/api/ratelimit"
	"github.com/strelov1/freehire/internal/identity/billing"
)

// storeSyncTimeout bounds the one provider call the sync route makes. Shorter than the
// webhook's, because a person is waiting on this one: a paywall that has already taken their
// money and is now spinning is worse than one that says "a moment" and lets them retry.
const storeSyncTimeout = 8 * time.Second

// storeSyncLimit and storeSyncWindow bound how often one caller can make us call RevenueCat.
//
// Generous enough for the real pattern — buy, land back in the app, ask once, maybe retry
// twice while the store settles — and far too tight to be worth pointing at us. The route is
// authenticated, so the budget is per account rather than per address.
const (
	storeSyncLimit  = 10
	storeSyncWindow = time.Minute
)

// registerStore mounts the App Store and Google Play routes, or mounts nothing.
//
// Independent of Stripe's block above it: the two providers are configured separately, and a
// deployment may have either, both or neither.
func (h *billingHandlers) registerStore(api fiber.Router, mw middleware) {
	if h.store == nil || !h.store.Enabled() {
		return
	}
	// Unauthenticated, and authenticated by RevenueCat's HMAC instead — the same arrangement
	// as Stripe's webhook, over a different header with a different window.
	api.Post("/billing/revenuecat/webhook", webhookFor(h.store))
	// Cookie only, and rate-limited: this is the one route a caller can use to make the
	// server call a third party on demand.
	api.Post("/billing/revenuecat/sync", mw.cookie,
		ratelimit.Middleware(mw.throttler, ratelimit.KeyByUserOrIP("billing_sync"), storeSyncLimit, storeSyncWindow),
		h.SyncStore)
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
func (h *billingHandlers) SyncStore(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(c.Context(), storeSyncTimeout)
	defer cancel()

	switch err := h.store.SyncCaller(ctx, userID); {
	case err == nil:
		return c.JSON(fiber.Map{"data": fiber.Map{"status": "synced"}})

	case errors.Is(err, billing.ErrNoSubscription):
		// RevenueCat was ASKED and holds nothing for this account — it has never bought through
		// an app. Not an error: the honest answer is "nothing of yours to read".
		//
		// SyncCaller rather than SyncUser is what makes that sentence true. The guarded path
		// would have answered this without asking anybody, which for a first-time buyer whose
		// webhook was lost is the difference between "you are Pro" and "you are not a
		// subscriber" — and no reconciler pass would ever have found them either.
		return c.JSON(fiber.Map{"data": fiber.Map{"status": "no_subscription"}})

	default:
		// The provider is slow or down. The purchase is not lost — RevenueCat holds it and the
		// reconciler will find it — so this is a "try again", not a failure of the payment.
		log.Printf("billing: store sync for user %d failed: %v", userID, err)
		return fiber.NewError(fiber.StatusServiceUnavailable, "could not reach the store provider")
	}
}
