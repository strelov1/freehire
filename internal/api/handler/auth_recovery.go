package handler

import (
	"context"
	"errors"
	"log"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/api/apigen"
	"github.com/strelov1/freehire/internal/identity/accounts"
)

// maskEmail redacts an email address for logging to avoid PII leakage (e.g., user@example.com -> u***@e***.com).
func maskEmail(email string) string {
	parts := strings.Split(email, "@")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "***"
	}
	localRunes := []rune(parts[0])
	localMasked := string(localRunes[0]) + "***"

	domainParts := strings.SplitN(parts[1], ".", 2)
	domainRunes := []rune(domainParts[0])
	if len(domainRunes) == 0 {
		return localMasked + "@***"
	}
	domainMasked := string(domainRunes[0]) + "***"
	if len(domainParts) > 1 {
		domainMasked += "." + domainParts[1]
	}

	return localMasked + "@" + domainMasked
}

// forgotPasswordTimeout bounds the background work a forgot-password request spawns —
// the account lookup, the bcrypt of the code, and the outbound mail — so a stuck goroutine
// cannot leak after the request has already been answered.
const forgotPasswordTimeout = 30 * time.Second

// recoveryError maps the code-flow sentinels to HTTP errors. Wrong, absent, and burnt
// codes share one 400 with one message: a caller learns only that this code does not work.
func recoveryError(err error) error {
	switch {
	case errors.Is(err, accounts.ErrInvalidCode):
		return fiber.NewError(fiber.StatusBadRequest, "invalid or already used code")
	case errors.Is(err, accounts.ErrCodeExpired):
		return fiber.NewError(fiber.StatusBadRequest, "this code has expired — request a new one")
	case errors.Is(err, accounts.ErrResendTooSoon):
		return fiber.NewError(fiber.StatusTooManyRequests, "a code was just sent — check your inbox")
	case errors.Is(err, accounts.ErrMailUnavailable):
		return fiber.NewError(fiber.StatusServiceUnavailable, "email delivery is not configured")
	case errors.Is(err, accounts.ErrAlreadyVerified):
		return fiber.NewError(fiber.StatusConflict, "this email is already confirmed")
	case errors.Is(err, accounts.ErrSameEmail):
		return fiber.NewError(fiber.StatusBadRequest, "that is already your email")
	case errors.Is(err, accounts.ErrEmailTaken):
		// Worded for the change flow, the only code flow that can meet it.
		return fiber.NewError(fiber.StatusConflict, "another account already uses this email")
	default:
		return accountsError(err)
	}
}

// RequestEmailVerification mails the caller a fresh verification code. Cookie-only: the
// account is identified by the session, never by a body field, so the endpoint cannot be
// pointed at someone else's address.
func (h *authHandlers) RequestEmailVerification(ctx context.Context) error {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return err
	}
	user, err := h.accounts.UserByID(ctx, userID)
	if err != nil {
		return accountsError(err)
	}
	if user.EmailVerified {
		return recoveryError(accounts.ErrAlreadyVerified)
	}
	if err := h.accounts.IssueVerificationCode(ctx, userID, user.Email); err != nil {
		return recoveryError(err)
	}
	return nil
}

type verifyCodeRequest struct {
	Code string `json:"code"`
}

// ConfirmEmailVerification marks the caller's address verified when the presented code
// matches. Cookie-only, same reasoning as the request side.
func (h *authHandlers) ConfirmEmailVerification(ctx context.Context, req *apigen.AuthCode) (*apigen.ConfirmEmailVerificationOK, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	if err := h.accounts.ConfirmVerification(ctx, userID, req.Code.Value); err != nil {
		return nil, recoveryError(err)
	}
	user, err := h.accounts.UserByID(ctx, userID)
	if err != nil {
		return nil, accountsError(err)
	}
	return &apigen.ConfirmEmailVerificationOK{Data: authUserOut(h.toUserResponseWithTier(ctx, user))}, nil
}

// ForgotPassword mails a reset code, if the address has a password-backed account. It
// ALWAYS answers 202 and does the work on a bounded background context: the lookup, a
// bcrypt hash, and an SES round-trip take wildly different times for a known and an
// unknown address, so answering before any of it happens removes the timing side-channel
// instead of trying to mask it with dummy work.
func (h *authHandlers) ForgotPassword(_ context.Context, req *apigen.AuthPasswordForgot) error {
	go h.sendPasswordResetCode(req.Email.Value)
	return nil
}

// sendPasswordResetCode runs the forgot-password work off the request path. Failures are
// logged, never surfaced — the response was already sent, and any difference a caller
// could observe would be the enumeration oracle this design removes.
func (h *authHandlers) sendPasswordResetCode(email string) {
	ctx, cancel := context.WithTimeout(context.Background(), forgotPasswordTimeout)
	defer cancel()

	if err := h.accounts.RequestPasswordReset(ctx, email); err != nil {
		log.Printf("auth: password reset for %q: %v", maskEmail(email), err)
	}
}

// ResetPassword sets a new password against a mailed code. Public — the code is the
// credential. Success revokes every session, so the user signs in fresh with the new
// password rather than being handed a session by the reset itself.
func (h *authHandlers) ResetPassword(ctx context.Context, req *apigen.AuthPasswordReset) (*apigen.ResetPasswordOK, error) {
	if err := h.accounts.ResetPassword(ctx, req.Email.Value, req.Code.Value, req.Password.Value); err != nil {
		return nil, recoveryError(err)
	}
	return &apigen.ResetPasswordOK{Data: apigen.AuthPasswordResetResult{Reset: true}}, nil
}

type changePasswordRequest struct {
	CurrentPassword string `json:"current_password"`
	Password        string `json:"password"`
}

// ChangePassword replaces a known password. Cookie-only: an API key must not be able to
// change the credential it would then outlive. The change revokes every session, and the
// caller's own cookie is re-issued at the new generation so they stay signed in.
func (h *authHandlers) ChangePassword(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	var in changePasswordRequest
	if err := c.BodyParser(&in); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}
	version, err := h.accounts.ChangePassword(c.Context(), userID, in.CurrentPassword, in.Password)
	if err != nil {
		return recoveryError(err)
	}
	if err := h.setSessionAt(c, userID, version); err != nil {
		return err
	}
	return c.JSON(fiber.Map{"data": fiber.Map{"changed": true}})
}

// RequestEmailChange mails a confirmation code to the address the caller wants to move
// the account to. Nothing changes until ConfirmEmailChange spends that code.
func (h *authHandlers) RequestEmailChange(ctx context.Context, req *apigen.AuthEmailChange) error {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return err
	}
	if err := h.accounts.RequestEmailChange(ctx, userID, req.Email.Value); err != nil {
		return recoveryError(err)
	}
	return nil
}

// ConfirmEmailChange moves the caller's account to the address the code was mailed to.
// The change revokes every session, and the caller's own cookie is re-issued at the new
// generation so they stay signed in — the same shape as ChangePassword.
func (h *authHandlers) ConfirmEmailChange(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	var in verifyCodeRequest
	if err := c.BodyParser(&in); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}
	version, err := h.accounts.ConfirmEmailChange(c.Context(), userID, in.Code)
	if err != nil {
		return recoveryError(err)
	}
	if err := h.setSessionAt(c, userID, version); err != nil {
		return err
	}
	user, err := h.accounts.UserByID(c.Context(), userID)
	if err != nil {
		return accountsError(err)
	}
	return c.JSON(fiber.Map{"data": h.toUserResponseWithTier(c.Context(), user)})
}
