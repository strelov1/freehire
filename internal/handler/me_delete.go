package handler

import (
	"context"
	"errors"
	"strings"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/accountdelete"
	"github.com/strelov1/freehire/internal/auth"
)

// accountEraser erases an account and everything it owns. Satisfied by
// *accountdelete.Service.
type accountEraser interface {
	Delete(ctx context.Context, userID int64) error
}

// accountEmailLookup resolves the caller's own email, which the typed confirmation is
// checked against. Satisfied directly by *db.Queries (UserEmail).
type accountEmailLookup interface {
	UserEmail(ctx context.Context, userID int64) (string, error)
}

// deleteAccountBody carries the confirmation: the member types their own email
// address. It is deliberately not a yes/no flag — this is the one request in the API
// that cannot be undone.
type deleteAccountBody struct {
	Email string `json:"email"`
}

// DeleteAccount permanently erases the caller's account: every user-owned row, every
// object in storage, and the Gmail grant at Google. There is no grace period and no
// restore path.
//
// Cookie-only, deliberately: an API key must not be able to destroy the account that
// issued it (the same reasoning that keeps key management off the key path). The
// session cookie is expired in the response, and every other device's cookie stops
// authenticating as soon as the row is gone — RequireAuth checks the subject exists.
func (h *authHandlers) DeleteAccount(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	var in deleteAccountBody
	if err := c.BodyParser(&in); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}
	email, err := h.accountEmails.UserEmail(c.Context(), userID)
	if err != nil {
		return err
	}
	// Case-insensitive, matching how an account is looked up at login: a member who
	// types their address in a different case is still confirming their own account.
	if in.Email == "" || !strings.EqualFold(strings.TrimSpace(in.Email), email) {
		return fiber.NewError(fiber.StatusBadRequest, "confirm deletion by entering your account email")
	}
	if err := h.accountDelete.Delete(c.Context(), userID); err != nil {
		if errors.Is(err, accountdelete.ErrStorageUnavailable) {
			// Nothing was erased — the account is intact and the request can be retried.
			return fiber.NewError(fiber.StatusServiceUnavailable, "could not erase your stored files; nothing was deleted, please try again")
		}
		return err
	}
	auth.ClearTokenCookie(c, h.cookieSecure, auth.CookieDomainForHost(c.Hostname(), h.cookieDomains))
	return c.SendStatus(fiber.StatusNoContent)
}
