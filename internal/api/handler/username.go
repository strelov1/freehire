package handler

import (
	"time"

	"github.com/gofiber/fiber/v2"
)

// usernameCheckResponse is the wire shape for GET /username/check.
type usernameCheckResponse struct {
	Available bool `json:"available"`
}

// CheckUsername reports whether a candidate username could be claimed right
// now (valid format, not reserved, not already held). Public — usable while
// picking a name before committing to a claim, or before the caller is even
// signed in.
func (h *authHandlers) CheckUsername(c *fiber.Ctx) error {
	available, err := h.accounts.UsernameAvailable(c.Context(), c.Query("value"))
	if err != nil {
		return err
	}
	return c.JSON(fiber.Map{"data": usernameCheckResponse{Available: available}})
}

// usernameResponse is the wire shape for GET/PUT /me/username. Username is
// null until the caller has claimed or been lazily allocated one; UpdatedAt is
// null for a lazy default that was never explicitly claimed.
type usernameResponse struct {
	Username  *string    `json:"username"`
	UpdatedAt *time.Time `json:"updated_at"`
}

// respondUsername writes the caller's current username state.
func (h *authHandlers) respondUsername(c *fiber.Ctx, userID int64) error {
	name, updatedAt, ok, err := h.accounts.Username(c.Context(), userID)
	if err != nil {
		return err
	}
	resp := usernameResponse{UpdatedAt: updatedAt}
	if ok {
		resp.Username = &name
	}
	return c.JSON(fiber.Map{"data": resp})
}

// GetUsername returns the caller's own username (null until claimed/allocated).
// Cookie-only, same reasoning as the other /me reads: it is the caller's own
// account state.
func (h *authHandlers) GetUsername(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	return h.respondUsername(c, userID)
}

// claimUsernameRequest is the PUT /me/username body.
type claimUsernameRequest struct {
	Username string `json:"username"`
}

// ClaimUsername claims or changes the caller's username. Cookie-only.
func (h *authHandlers) ClaimUsername(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	var in claimUsernameRequest
	if err := c.BodyParser(&in); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}
	if err := h.accounts.ClaimUsername(c.Context(), userID, in.Username); err != nil {
		return accountsError(err)
	}
	return h.respondUsername(c, userID)
}
