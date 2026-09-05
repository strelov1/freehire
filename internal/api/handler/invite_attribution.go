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
// A setter rather than another parameter on a constructor that already takes ten. It is
// also honestly optional: attribution is the one thing here that must never be able to fail
// a sign-up, and a nil service simply means nothing is attributed.
func (h *authHandlers) withInvites(svc *promo.Service) *authHandlers {
	h.invites = svc
	return h
}

// attributeInvite records that a newly created account arrived through an invite link, and
// clears the cookie once it has.
//
// Everything about it is best-effort by design. It runs after the account exists and after
// the session is set, it returns nothing, and it logs rather than surfacing: a referral must
// never cost somebody their account. The freshness rule that keeps this from attributing an
// account that has existed for years lives in the SQL, not here — see AttributeInvite.
func (h *authHandlers) attributeInvite(c *fiber.Ctx, userID int64) {
	if h.invites == nil {
		return
	}
	code := c.Cookies(promo.AttributionCookie)
	if code == "" {
		return
	}

	if err := h.invites.Attribute(c.Context(), userID, code); err != nil {
		// Left in place on failure. The cookie costs nothing to carry, and clearing it here
		// would throw away the only record of who brought this person.
		log.Printf("promo: attributing user %d: %v", userID, err)
		return
	}
	h.clearAttributionCookie(c)
}

// clearAttributionCookie expires the invite cookie.
//
// The attributes must match the ones the cookie was set with — a browser matches a deletion
// on name, path and domain — which is why the path is stated rather than left to default.
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
