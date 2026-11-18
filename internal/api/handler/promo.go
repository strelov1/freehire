package handler

import (
	"context"
	"errors"
	"log"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/api/apigen"
	"github.com/strelov1/freehire/internal/identity/promo"
)

// promoReadTimeout bounds one database read on these routes (small indexed queries).
const promoReadTimeout = 5 * time.Second

// promoHandlers serve the discount surfaces: checking a code, and an account's own invite
// link. The invite routes are mounted whatever billing is doing; only checkout needs a
// configured provider.
type promoHandlers struct {
	promo *promo.Service
}

func newPromoHandlers(svc *promo.Service) *promoHandlers {
	return &promoHandlers{promo: svc}
}

// register mounts the referral and promo routes.
func (h *promoHandlers) register(api fiber.Router, mw middleware, contract fiber.Handler) {
	// Cookie only, like the rest of /me: the invite link is minted for a browser session, not a key.
	api.Get("/me/invite", mw.cookie, contract)

	api.Post("/me/promo/preview", mw.cookie, contract)

	// POST is a security property: redeeming spends the account's one lifetime redemption, and a
	// GET would carry the `SameSite=Lax` cookie on a cross-site top-level navigation, letting any
	// page burn it. A GET here must stay read-only.
	//
	// Rate limited with preview: a refusing redemption leaks as much to a guesser as a refusing
	// preview, so bounding only one bounds neither.
	api.Post("/me/promo/redeem", mw.cookie, contract)
}

// GetInvite returns this account's invite link and what it has earned.
//
// The counts name nobody: telling a referrer which contact signed up would disclose that
// person is looking for work, so the service returns aggregates only.
func (h *promoHandlers) GetInvite(ctx context.Context) (*apigen.GetInviteOK, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(ctx, promoReadTimeout)
	defer cancel()

	link, err := h.promo.Link(ctx, userID)
	if err != nil {
		log.Printf("promo: minting an invite link for user %d: %v", userID, err)
		return nil, fiber.NewError(fiber.StatusInternalServerError, "could not read your invite link")
	}

	stats, err := h.promo.Stats(ctx, userID)
	if err != nil {
		log.Printf("promo: reading invite stats for user %d: %v", userID, err)
		return nil, fiber.NewError(fiber.StatusInternalServerError, "could not read your invite link")
	}

	return &apigen.GetInviteOK{Data: apigen.Invite{
		Link:        link,
		Invitees:    stats.Invitees,
		Rewarded:    stats.Rewarded,
		CreditCents: stats.CreditCents,
		PercentOff:  int64(promo.InvitePercent),
	}}, nil
}

// PreviewPromoCode reports what a code is worth to this caller, without consuming a seat.
//
// Every refusal about the CODE is the same refusal: telling "out of seats" from "no such
// code" would make the route a guessing oracle.
func (h *promoHandlers) PreviewPromoCode(ctx context.Context, req *apigen.PromoCode) (*apigen.PreviewPromoCodeOK, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(ctx, promoReadTimeout)
	defer cancel()

	percent, err := h.promo.Preview(ctx, userID, req.Code.Value)
	if err != nil {
		return nil, promoError(err, "could not check that code", userID)
	}
	return &apigen.PreviewPromoCodeOK{Data: apigen.PromoDiscount{PercentOff: int64(percent)}}, nil
}

// RedeemPromoCode spends this account's one lifetime redemption on a code.
//
// Separate from checkout on purpose: the redemption is durable (later checkouts read it back
// through `promo.Discount`), so a provider failure costs a retry, not the offer.
func (h *promoHandlers) RedeemPromoCode(ctx context.Context, req *apigen.PromoCode) (*apigen.RedeemPromoCodeOK, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(ctx, promoReadTimeout)
	defer cancel()

	percent, err := h.promo.Redeem(ctx, userID, req.Code.Value)
	if err != nil {
		return nil, promoError(err, "could not apply that code", userID)
	}
	return &apigen.RedeemPromoCodeOK{Data: apigen.PromoDiscount{PercentOff: int64(percent)}}, nil
}

// promoError renders the two refusals this surface distinguishes, and nothing else.
//
// Every reason a CODE can be refused is one status, deliberately, so a guesser learns nothing.
func promoError(err error, fallback string, userID int64) error {
	switch {
	case errors.Is(err, promo.ErrAlreadyRedeemed):
		return fiber.NewError(fiber.StatusConflict, "you have already used a promo code")
	case errors.Is(err, promo.ErrNotUsable):
		return fiber.NewError(fiber.StatusNotFound, "that code is not available")
	default:
		log.Printf("promo: %s for user %d: %v", fallback, userID, err)
		return fiber.NewError(fiber.StatusInternalServerError, fallback)
	}
}
