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
// server (so a resource binding has nothing to be carried across) — the issue asks for grants that "expire after 30 days," not ones that
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
	if !isURLEncodedForm(c) {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "invalid_request", "error_description": "the token request must be application/x-www-form-urlencoded",
		})
	}
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

	// Validated before the code is consumed: a malformed or foreign resource is
	// a request error, not a reason to burn a code the client could re-send.
	tokenResource, err := oauth2server.CanonicalResource(h.frontendOrigin, bytesToStrings(c.Context().PostArgs().PeekMulti("resource")))
	if err != nil {
		return invalidTarget(c, "resource is not a freehire MCP endpoint")
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
	// The grant is bound to the resource the code was authorized for; a resource
	// sent only here binds a grant whose code carried none, but cannot contradict
	// one it did (RFC 8707 2.2).
	resource := pgconv.TextString(row.Resource)
	if resource != "" && tokenResource != "" && tokenResource != resource {
		return invalidTarget(c, "resource does not match the authorization request")
	}
	if resource == "" {
		resource = tokenResource
	}

	// The grant is stamped with the generation the CONSUMED code was bound to —
	// the consent-time session generation, already re-validated against the
	// account's CURRENT one by the consume's latch — never a fresh read of
	// users.token_version. A revocation landing between this consume and the
	// grant INSERT below therefore leaves the grant on the dead generation,
	// where AuthenticateOAuthGrant refuses it: a consumed code can never be
	// upgraded to the generation that revocation minted.
	version := row.IssuedTokenVersion.Int32

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
		Resource:           pgconv.Text(resource),
	}); err != nil {
		return fmt.Errorf("persist oauth grant for user %d: %w", row.UserID, err)
	}

	return c.JSON(tokenResponse{
		AccessToken: token,
		TokenType:   "Bearer",
		ExpiresIn:   int(accessTokenTTL.Seconds()),
	})
}

// invalidTarget is RFC 8707's token-endpoint refusal: a 400 carrying the OAuth
// error code a client library keys on, not a bare message.
func invalidTarget(c *fiber.Ctx, description string) error {
	return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid_target", "error_description": description})
}
