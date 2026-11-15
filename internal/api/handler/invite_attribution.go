package handler

import (
	"log"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/identity/promo"
)

// withInvites gives the auth handlers the discount service, so a registration can be
// attributed to whoever's link brought the person.
//
// A setter, not another constructor parameter. Optional: attribution must never fail a
// sign-up, and a nil service means nothing is attributed.
func (h *authHandlers) withInvites(svc *promo.Service) *authHandlers {
	h.invites = svc
	return h
}

// attributeInvite records that a newly created account arrived through an invite link, and
// clears the cookie once it has.
//
// Best-effort by design: it runs after the account and session exist, returns nothing and
// only logs, since a referral must never cost somebody their account. The freshness rule
// against attributing old accounts lives in the SQL (see AttributeInvite).
func (h *authHandlers) attributeInvite(c *fiber.Ctx, userID int64) {
	if h.invites == nil {
		return
	}
	code := c.Cookies(promo.AttributionCookie)
	if code == "" {
		return
	}

	if err := h.invites.Attribute(c.Context(), userID, code); err != nil {
		// Left in place on failure: clearing it would lose the only record of the inviter.
		log.Printf("promo: attributing user %d: %v", userID, err)
		return
	}
	h.clearAttributionCookie(c)
}

// clearAttributionCookie expires the invite cookie.
//
// The attributes must match those it was set with (a browser matches deletion on name, path
// and domain), so the path is stated explicitly.
func (h *authHandlers) clearAttributionCookie(c *fiber.Ctx) {
	c.Cookie(&fiber.Cookie{
		Name:     promo.AttributionCookie,
		Value:    "",
		Path:     "/",
		Expires:  time.Unix(0, 0),
		MaxAge:   -1,
		HTTPOnly: true,
		Secure:   h.cookieSecure,
		SameSite: "Lax",
	})
}
