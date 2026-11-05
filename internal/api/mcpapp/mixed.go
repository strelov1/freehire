package mcpapp

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/strelov1/freehire/internal/ai/assistant"
)

// MixedResourceMetadataPath identifies the canonical endpoint without changing
// the root metadata used by existing /mcp/account connections.
const MixedResourceMetadataPath = "/.well-known/oauth-protected-resource/api/v1/mcp"

func newMixedServer(tools []assistant.Tool, userID int64, origin string, reader Reader) *mcp.Server {
	instructions := "Public catalogue tools work without signing in. Personal tools require OAuth for this account. " +
		"Geography filters form a union. "
	account := accountInstructions(tools)
	instructions += strings.TrimPrefix(account, accountServerInstructions)
	server := newCatalogueServer(reader, origin, instructions, "mixed")
	for _, tool := range tools {
		// Preserve the public schemas, previews and widgets on the canonical endpoint.
		switch tool.Name {
		case toolSearchJobs, toolGetJob, toolGetCompany:
			continue
		}
		handler := accountToolHandler(tool, userID)
		if userID == 0 {
			handler = func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				result := errorResult("Sign in to Freehire to use this personal tool.")
				challenge := `Bearer resource_metadata="` + strings.TrimSuffix(origin, "/") + MixedResourceMetadataPath +
					`", error="insufficient_scope", error_description="Sign in to Freehire to use personal tools"`
				result.Meta = mcp.Meta{
					"mcp/www_authenticate": []string{challenge},
					diagnosticsMetaKey:     map[string]any{"error_code": "unauthenticated"},
				}
				return result, nil
			}
		}
		server.AddTool(&mcp.Tool{
			Name: tool.Name, Title: tool.Title, Description: tool.Description,
			InputSchema: tool.Schema, Annotations: accountAnnotations(tool),
			Meta: mcp.Meta{"securitySchemes": []map[string]any{{"type": "oauth2", "scopes": []string{}}}},
		}, loggedTool("mixed", tool.Name, handler))
	}
	return server
}

// MixedHandler trusts only the identity stamped by the authentication middleware.
// An absent identity is anonymous; malformed internal identity is a wiring error.
func MixedHandler(tools []assistant.Tool, origin string, reader Reader) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var userID int64
		if value := r.Header.Get(AccountUserIDHeader); value != "" {
			parsed, err := strconv.ParseInt(value, 10, 64)
			if err != nil || parsed <= 0 {
				http.Error(w, "internal error: invalid authenticated user", http.StatusInternalServerError)
				return
			}
			userID = parsed
		}
		mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server {
			return newMixedServer(tools, userID, origin, reader)
		}, &mcp.StreamableHTTPOptions{Stateless: true}).ServeHTTP(w, r)
	})
}
