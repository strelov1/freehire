package handler

import (
	"context"
	"errors"
	"log"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/api/ratelimit"
	"github.com/strelov1/freehire/internal/identity/promo"
)

// promoReadTimeout bounds one database read on these routes. Everything here is a small
// indexed query; a longer budget would only hold a connection open for a page that has
// already given up.
const promoReadTimeout = 5 * time.Second

// promoPreviewPerMinute is how often one account may ask whether a code is valid, and
// promoPreviewPerAddressPerMinute how often one client address may, across every account it
// signs in as.
//
// Both, because either alone is a hole. An account is free to create, so a per-account
// bound is a bound on nothing — the cheap attack is a hundred accounts from one machine.
// And a per-address bound alone would punish everyone behind one office NAT for the first
// person to mistype a code. The address budget is deliberately looser than a multiple of
// the account one: several colleagues redeeming a launch offer on the same day is ordinary,
// and thousands of guesses an hour is not.
//
// `KeyByUserOrIP` cannot serve both. It resolves to the account id whenever there IS one,
// and both these routes sit behind the cookie gate — so its address half is unreachable
// here, and a second limiter keyed by address is the only way to have one.
const (
	promoPreviewPerMinute           = 10
	promoPreviewPerAddressPerMinute = 30
)

// promoHandlers serve the discount surfaces: checking a code, and an account's own invite
// link.
//
// The invite routes are mounted whatever billing is doing, because sharing a link and
// counting who came is not a purchase. Only the checkout, which lives with the other
// billing routes, needs a configured provider.
type promoHandlers struct {
	promo *promo.Service
}

func newPromoHandlers(svc *promo.Service) *promoHandlers {
	return &promoHandlers{promo: svc}
}

func (h *promoHandlers) register(api fiber.Router, mw middleware, throttler ratelimit.Throttler) {
	// Cookie only, like the rest of /me. A link that decides who gets credited for a
	// referral is minted for a browser session, not for a script holding a key.
	api.Get("/me/invite", mw.cookie, h.Invite)

	perAccount := ratelimit.Middleware(throttler,
		ratelimit.KeyByUserOrIP("promo"), promoPreviewPerMinute, time.Minute)
	perAddress := ratelimit.Middleware(throttler,
		ratelimit.KeyByIP("promoaddr"), promoPreviewPerAddressPerMinute, time.Minute)

	api.Post("/me/promo/preview", mw.cookie, perAccount, perAddress, h.PreviewCode)

	// POST, and that is a security property rather than a style choice. Redeeming spends
	// the account's one lifetime redemption, and the first draft did it on the checkout
	// GET — which `SameSite=Lax` sends the session cookie along with on a cross-site
	// top-level navigation. Any page could then burn a visitor's redemption by linking to
	// it. A GET here must stay read-only; a state change goes through a method the browser
	// will not issue cross-site without a preflight this deployment does not answer.
	//
	// Limited by the same pair: a redemption that refuses tells a guesser as much as a
	// preview that refuses, so bounding one and not the other bounds neither.
	api.Post("/me/promo/redeem", mw.cookie, perAccount, perAddress, h.RedeemCode)
}

// Invite returns this account's invite link and what it has earned.
//
// The counts name nobody. Telling a referrer which of their contacts signed up would
// disclose that a particular person is looking for work, which is not theirs to know — so
// the service returns aggregates and there is no query that could return more.
func (h *promoHandlers) Invite(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(c.Context(), promoReadTimeout)
	defer cancel()

	link, err := h.promo.Link(ctx, userID)
	if err != nil {
		log.Printf("promo: minting an invite link for user %d: %v", userID, err)
		return fiber.NewError(fiber.StatusInternalServerError, "could not read your invite link")
	}

	stats, err := h.promo.Stats(ctx, userID)
	if err != nil {
		log.Printf("promo: reading invite stats for user %d: %v", userID, err)
		return fiber.NewError(fiber.StatusInternalServerError, "could not read your invite link")
	}

	return c.JSON(fiber.Map{"data": fiber.Map{
		"link":         link,
		"invitees":     stats.Invitees,
		"rewarded":     stats.Rewarded,
		"credit_cents": stats.CreditCents,
		"percent_off":  promo.InvitePercent,
	}})
}

// PreviewCode reports what a code is worth to this caller, without consuming a seat.
//
// Rate limited above, and every refusal about the CODE is the same refusal: the route is
// reachable by anyone who can create an account, and answering "that code exists but is out
// of seats" differently from "no such code" would turn it into an oracle for guessing.
func (h *promoHandlers) PreviewCode(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}

	var body struct {
		Code string `json:"code"`
	}
	if err := c.BodyParser(&body); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "expected a JSON body with a code")
	}

	ctx, cancel := context.WithTimeout(c.Context(), promoReadTimeout)
	defer cancel()

	percent, err := h.promo.Preview(ctx, userID, body.Code)
	if err != nil {
		return promoError(err, "could not check that code", userID)
	}
	return c.JSON(fiber.Map{"data": fiber.Map{"percent_off": percent}})
}

// RedeemCode spends this account's one lifetime redemption on a code.
//
// Separate from checkout on purpose. The redemption is DURABLE: once recorded, every later
// checkout reads the percentage back through `promo.Discount`, so a provider failure while
// opening the payment page costs a retry rather than the offer. Making the checkout GET do
// this instead — as the first draft did — was both a CSRF hole and a way to burn somebody's
// one code on a call that then failed.
func (h *promoHandlers) RedeemCode(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}

	var body struct {
		Code string `json:"code"`
	}
	if err := c.BodyParser(&body); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "expected a JSON body with a code")
	}

	ctx, cancel := context.WithTimeout(c.Context(), promoReadTimeout)
	defer cancel()

	percent, err := h.promo.Redeem(ctx, userID, body.Code)
	if err != nil {
		return promoError(err, "could not apply that code", userID)
	}
	return c.JSON(fiber.Map{"data": fiber.Map{"percent_off": percent}})
}

// promoError renders the two refusals this surface distinguishes, and nothing else.
//
// Every reason a CODE can be refused is one status, deliberately: these routes are
// reachable by anyone who can create an account, and separating "no such code" from "out of
// seats" would answer the question a guesser is actually asking.
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
