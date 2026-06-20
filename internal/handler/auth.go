package handler

import (
	"errors"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/accounts"
	"github.com/strelov1/freehire/internal/auth"
)

// userResponse is the public shape of a user. It deliberately omits
// password_hash so the hash never reaches a response. role is included so the SPA can
// decide whether to surface moderator-only UI; it is an affordance only, as RequireRole
// re-checks the DB-stored role on every privileged request.
type userResponse struct {
	ID        int64      `json:"id"`
	Email     string     `json:"email"`
	Role      string     `json:"role"`
	CreatedAt *time.Time `json:"created_at"`
}

type credentials struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// toUserResponse maps an accounts.User to its public response shape.
func toUserResponse(u accounts.User) userResponse {
	return userResponse{ID: u.ID, Email: u.Email, Role: u.Role, CreatedAt: u.CreatedAt}
}

// accountsError maps the accounts service sentinels to HTTP errors, preserving
// the statuses and generic messages the handlers used before delegation.
func accountsError(err error) error {
	switch {
	case errors.Is(err, accounts.ErrInvalidEmail):
		return fiber.NewError(fiber.StatusBadRequest, "invalid email")
	case errors.Is(err, accounts.ErrPasswordTooShort):
		return fiber.NewError(fiber.StatusBadRequest, "password must be at least 8 characters")
	case errors.Is(err, accounts.ErrPasswordTooLong):
		return fiber.NewError(fiber.StatusBadRequest, "password must be at most 72 characters")
	case errors.Is(err, accounts.ErrEmailTaken):
		return fiber.NewError(fiber.StatusConflict, "email already registered")
	case errors.Is(err, accounts.ErrInvalidCredentials):
		return fiber.NewError(fiber.StatusUnauthorized, "invalid credentials")
	case errors.Is(err, accounts.ErrUserNotFound):
		return fiber.NewError(fiber.StatusUnauthorized, "unauthorized")
	default:
		return err
	}
}

// Register creates an account, starts a session (auth cookie), and returns the
// user.
func (a *API) Register(c *fiber.Ctx) error {
	var in credentials
	if err := c.BodyParser(&in); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}
	user, err := a.accounts.Register(c.Context(), in.Email, in.Password)
	if err != nil {
		return accountsError(err)
	}
	if err := a.setSession(c, user.ID); err != nil {
		return err
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"data": toUserResponse(user)})
}

// Login verifies credentials, starts a session (auth cookie), and returns the
// user. Unknown email, wrong password, and passwordless accounts all yield the
// same generic 401 so the response never reveals which factor failed.
func (a *API) Login(c *fiber.Ctx) error {
	var in credentials
	if err := c.BodyParser(&in); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}
	user, err := a.accounts.Login(c.Context(), in.Email, in.Password)
	if err != nil {
		return accountsError(err)
	}
	if err := a.setSession(c, user.ID); err != nil {
		return err
	}
	return c.JSON(fiber.Map{"data": toUserResponse(user)})
}

// Logout clears the auth cookie. It is public and idempotent: clearing an
// absent or already-expired cookie is a no-op.
func (a *API) Logout(c *fiber.Ctx) error {
	auth.ClearTokenCookie(c, a.cookieSecure)
	return c.SendStatus(fiber.StatusNoContent)
}

// setSession issues a token for userID and writes it as the auth cookie.
func (a *API) setSession(c *fiber.Ctx, userID int64) error {
	token, err := a.issuer.Issue(userID)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to start session")
	}
	auth.SetTokenCookie(c, token, a.issuer.TTL(), a.cookieSecure)
	return nil
}

// Me returns the authenticated user. It runs behind auth.RequireAuth, which has
// already resolved and stored the user id.
func (a *API) Me(c *fiber.Ctx) error {
	id, err := requireUserID(c)
	if err != nil {
		return err
	}
	user, err := a.accounts.UserByID(c.Context(), id)
	if err != nil {
		return accountsError(err)
	}
	return c.JSON(fiber.Map{"data": toUserResponse(user)})
}

// authHasher adapts the auth package's bcrypt helpers to the accounts.PasswordHasher
// interface, keeping the accounts package free of the auth/fiber dependency graph.
type authHasher struct{}

func (authHasher) Hash(plain string) (string, error) { return auth.HashPassword(plain) }
func (authHasher) Check(hash, plain string) error    { return auth.CheckPassword(hash, plain) }
