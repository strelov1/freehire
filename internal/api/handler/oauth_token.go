package handler

import (
	"errors"
	"fmt"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5"

	"github.com/strelov1/freehire/internal/identity/auth/oauth2server"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/pgconv"
)

// accessTokenTTL is the grant's fixed lifetime. No refresh token exists in this
// server — the issue asks for grants that "expire after 30 days," not ones that
// renew themselves; re-approving the consent screen mints a fresh one.
const accessTokenTTL = 30 * 24 * time.Hour

type tokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int    `json:"expires_in"`
}

// OAuthToken implements the authorization_code grant's token exchange. Public —
// the caller authenticates with the code and PKCE verifier, not a session; this is
// the one step a public, non-browser client makes directly, no cookie involved.
func (h *authHandlers) OAuthToken(c *fiber.Ctx) error {
	if c.FormValue("grant_type") != "authorization_code" {
		return fiber.NewError(fiber.StatusBadRequest, "unsupported grant_type")
	}
	code := c.FormValue("code")
	clientID := c.FormValue("client_id")
	redirectURI := c.FormValue("redirect_uri")
	verifier := c.FormValue("code_verifier")
	if code == "" || clientID == "" || redirectURI == "" || verifier == "" {
		return fiber.NewError(fiber.StatusBadRequest, "code, client_id, redirect_uri and code_verifier are required")
	}

	row, err := h.queries.ConsumeOAuthAuthorizationCode(c.Context(), oauth2server.HashToken(code))
	if errors.Is(err, pgx.ErrNoRows) {
		return fiber.NewError(fiber.StatusBadRequest, "code is invalid, expired, or already used")
	}
	if err != nil {
		return err
	}
	// The code is already consumed (deleted) at this point — every check below
	// that fails must still leave it spent, which is the right failure mode: a
	// mismatched client/redirect/verifier means something is wrong with this
	// exchange, and the client must restart the authorize step rather than retry
	// the same code.
	if row.ClientID != clientID || row.RedirectUri != redirectURI {
		return fiber.NewError(fiber.StatusBadRequest, "client_id or redirect_uri does not match the authorization request")
	}
	if !oauth2server.VerifyPKCE(row.CodeChallenge, verifier) {
		return fiber.NewError(fiber.StatusBadRequest, "code_verifier does not match code_challenge")
	}

	version, err := h.queries.GetUserTokenVersion(c.Context(), row.UserID)
	if err != nil {
		return fmt.Errorf("read token version for user %d: %w", row.UserID, err)
	}

	token, hash, err := oauth2server.GenerateAccessToken()
	if err != nil {
		return fmt.Errorf("generate access token for user %d: %w", row.UserID, err)
	}
	expiresAt := time.Now().Add(accessTokenTTL)
	if _, err := h.queries.CreateOAuthGrant(c.Context(), db.CreateOAuthGrantParams{
		UserID:             row.UserID,
		ClientID:           row.ClientID,
		AccessTokenHash:    hash,
		IssuedTokenVersion: version,
		ExpiresAt:          pgconv.Timestamptz(&expiresAt),
	}); err != nil {
		return fmt.Errorf("persist oauth grant for user %d: %w", row.UserID, err)
	}

	return c.JSON(tokenResponse{
		AccessToken: token,
		TokenType:   "Bearer",
		ExpiresIn:   int(accessTokenTTL.Seconds()),
	})
}
