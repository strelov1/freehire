package handler

import (
	"strings"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/identity/auth/oauth2server"
)

// authorizationServerMetadata is RFC 8414's document: where the OAuth endpoints live and
// what this server supports.
type authorizationServerMetadata struct {
	Issuer                            string   `json:"issuer"`
	AuthorizationEndpoint             string   `json:"authorization_endpoint"`
	TokenEndpoint                     string   `json:"token_endpoint"`
	RegistrationEndpoint              string   `json:"registration_endpoint"`
	ResponseTypesSupported            []string `json:"response_types_supported"`
	GrantTypesSupported               []string `json:"grant_types_supported"`
	CodeChallengeMethodsSupported     []string `json:"code_challenge_methods_supported"`
	TokenEndpointAuthMethodsSupported []string `json:"token_endpoint_auth_methods_supported"`
}

// OAuthAuthorizationServerMetadata renders RFC 8414's document. Public, identical for every
// caller.
//
// Mounted at an internal /api/v1/oauth/metadata/... path, not /.well-known/... directly:
// nginx sends /api/ to Go and everything else to SvelteKit, so a Go route at the well-known
// path would never be reached (same as the OJCP manifest, internal/api/ojcp/manifest.go).
// SvelteKit proxies the well-known path here.
func (h *authHandlers) OAuthAuthorizationServerMetadata(c *fiber.Ctx) error {
	origin := h.frontendOrigin
	return c.JSON(authorizationServerMetadata{
		Issuer:                            origin,
		AuthorizationEndpoint:             origin + "/api/v1/oauth/authorize",
		TokenEndpoint:                     origin + "/api/v1/oauth/token",
		RegistrationEndpoint:              origin + "/api/v1/oauth/register",
		ResponseTypesSupported:            []string{"code"},
		GrantTypesSupported:               []string{"authorization_code"},
		CodeChallengeMethodsSupported:     []string{"S256"},
		TokenEndpointAuthMethodsSupported: []string{"none"},
	})
}

// protectedResourceMetadata is RFC 9728's document: which authorization server
// protects this resource.
type protectedResourceMetadata struct {
	Resource             string   `json:"resource"`
	AuthorizationServers []string `json:"authorization_servers"`
}

// OAuthProtectedResourceMetadata renders RFC 9728's document, naming /api/v1/mcp as the
// protected resource and this deployment as its authorization server. Public. The root and
// path-inserted (mcpapp.ResourceMetadataPath) well-known paths both proxy here. Mounted
// internally like the document above.
func (h *authHandlers) OAuthProtectedResourceMetadata(c *fiber.Ctx) error {
	return h.protectedResource(c, oauth2server.MCPResourcePath)
}

// OAuthAccountProtectedResourceMetadata is the same document for the saved
// /api/v1/mcp/account URL. RFC 9728 §3.3 has the client compare `resource` with the URL it
// derived the metadata URL from, so a client connected with that address is told that
// address, not /api/v1/mcp. The path-inserted form proxies here from SvelteKit.
func (h *authHandlers) OAuthAccountProtectedResourceMetadata(c *fiber.Ctx) error {
	return h.protectedResource(c, oauth2server.MCPAccountResourcePath)
}

// protectedResource serves the document for one resource path. Each path used is one
// CanonicalResource accepts, so the client can send the resource back on /authorize.
func (h *authHandlers) protectedResource(c *fiber.Ctx, path string) error {
	origin := h.frontendOrigin
	return c.JSON(protectedResourceMetadata{
		Resource:             strings.TrimSuffix(origin, "/") + path,
		AuthorizationServers: []string{origin},
	})
}
