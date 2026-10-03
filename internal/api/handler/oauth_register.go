package handler

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/platform/db"
)

// registerClientRequest is the RFC 7591 request body. Only the two fields this
// server reads are declared; an MCP client's request may carry others
// (grant_types, scope, …) which are accepted and ignored rather than rejected —
// this server always answers with the one grant type and scope it supports.
type registerClientRequest struct {
	ClientName   string   `json:"client_name"`
	RedirectURIs []string `json:"redirect_uris"`
}

// registerClientResponse is the RFC 7591 response: the client's own metadata
// echoed back, plus the server-assigned client_id and the fixed values this
// server always answers with (public client, authorization_code only).
type registerClientResponse struct {
	ClientID                string   `json:"client_id"`
	ClientName              string   `json:"client_name"`
	RedirectURIs            []string `json:"redirect_uris"`
	TokenEndpointAuthMethod string   `json:"token_endpoint_auth_method"`
	GrantTypes              []string `json:"grant_types"`
	ResponseTypes           []string `json:"response_types"`
}

// RegisterOAuthClient implements RFC 7591 dynamic client registration. Public and
// unauthenticated by design: an MCP client (Claude, Cursor, ChatGPT) self-registers
// before it has ever talked to a freehire account, so there is no caller to gate
// on yet. The security boundary is the consent screen a human approves later, not
// who may mint a client_id here.
func (h *authHandlers) RegisterOAuthClient(c *fiber.Ctx) error {
	var in registerClientRequest
	if err := c.BodyParser(&in); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}
	in.ClientName = strings.TrimSpace(in.ClientName)
	if in.ClientName == "" {
		return fiber.NewError(fiber.StatusBadRequest, "client_name is required")
	}
	if len(in.RedirectURIs) == 0 {
		return fiber.NewError(fiber.StatusBadRequest, "redirect_uris must contain at least one URI")
	}

	clientID, err := generateClientID()
	if err != nil {
		return fmt.Errorf("generate oauth client id: %w", err)
	}

	row, err := h.queries.RegisterOAuthClient(c.Context(), db.RegisterOAuthClientParams{
		ClientID:     clientID,
		ClientName:   in.ClientName,
		RedirectUris: in.RedirectURIs,
	})
	if err != nil {
		return err
	}

	return c.Status(fiber.StatusCreated).JSON(registerClientResponse{
		ClientID:                row.ClientID,
		ClientName:              row.ClientName,
		RedirectURIs:            row.RedirectUris,
		TokenEndpointAuthMethod: "none",
		GrantTypes:              []string{"authorization_code"},
		ResponseTypes:           []string{"code"},
	})
}

// generateClientID mints a random, non-secret client identifier. It is not a
// credential (PKCE is), so it is not hashed at rest — it needs to be looked up by
// value at /authorize and /token, which a hash would prevent.
func generateClientID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return "fhcl_" + base64.RawURLEncoding.EncodeToString(b[:]), nil
}
