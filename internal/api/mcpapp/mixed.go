package mcpapp

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/strelov1/freehire/internal/ai/assistant"
)

// MixedResourceMetadataPath identifies the canonical endpoint without changing
// the root metadata used by existing /mcp/account connections.
const MixedResourceMetadataPath = "/.well-known/oauth-protected-resource/api/v1/mcp"

// accountMetaKey tells the job-card widget whether this connection is signed in,
// so it can offer Connect instead of buttons that would fail.
const accountMetaKey = "freehire.me/account"

func mixedChallenge(origin string) string {
	return `Bearer resource_metadata="` + strings.TrimSuffix(origin, "/") + MixedResourceMetadataPath +
		`", error="insufficient_scope", error_description="Sign in to Freehire to use personal tools"`
}

// stampsAccount adds the signed-in flag to the results the widget draws.
func stampsAccount(signedIn bool) mcp.Middleware {
	return func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
			result, err := next(ctx, method, req)
			call, _ := req.(*mcp.CallToolRequest)
			res, _ := result.(*mcp.CallToolResult)
			if call == nil || res == nil || (call.Params.Name != toolSearchJobs && call.Params.Name != toolGetJob) {
				return result, err
			}
			if res.Meta == nil {
				res.Meta = mcp.Meta{}
			}
			res.Meta[accountMetaKey] = map[string]any{"signed_in": signedIn}
			return result, err
		}
	}
}

func newMixedServer(tools []assistant.Tool, userID int64, origin string, reader Reader) *mcp.Server {
	instructions := "Public catalogue tools work without signing in. Personal tools require OAuth for this account. " +
		"Geography filters form a union. "
	// An anonymous connection must not be told to call get_profile first: the call would
	// answer with a sign-in challenge before an otherwise public search. It hears about the
	// account through the result's next hint instead.
	described := tools
	if userID == 0 {
		described = slices.DeleteFunc(slices.Clone(tools), func(t assistant.Tool) bool { return t.Name == "get_profile" })
	}
	account := accountInstructions(described)
	instructions += strings.TrimPrefix(account, accountServerInstructions)
	if userID == 0 {
		reader = forAnonymous(reader)
	}
	next := signedOutNext(origin)
	if userID != 0 {
		next = signedInNext(tools)
	}
	server := newCatalogueServer(reader, origin, instructions, "mixed", next)
	server.AddReceivingMiddleware(stampsAccount(userID != 0))
	if userID != 0 {
		addPromptSpecs(server, accountPrompts(tools))
	}
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
				result.Meta = mcp.Meta{
					"mcp/www_authenticate": []string{mixedChallenge(origin)},
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
//
// An anonymous call to a personal tool is answered two ways. ChatGPT (User-Agent
// openai-mcp) opens sign-in from the in-band mcp/www_authenticate result, as its
// Apps SDK documents. Every other host (Grok, Claude.ai) opens sign-in only on the
// MCP specification's HTTP 401 with WWW-Authenticate, and ignores the in-band one.
//
// Grok and Claude.ai go further: they offer sign-in only while a connector is being
// added, and a 401 later in a conversation opens nothing (measured on Grok, 2026-11-07).
// An anonymous request from them is therefore refused outright, so the connector asks
// for sign-in when it is added; without that it could never reach a personal tool.
func MixedHandler(tools []assistant.Tool, origin string, reader Reader) http.Handler {
	personal := map[string]bool{}
	for _, tool := range tools {
		personal[tool.Name] = true
	}
	// These names are served by the public catalogue tools instead.
	delete(personal, toolSearchJobs)
	delete(personal, toolGetJob)
	delete(personal, toolGetCompany)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get(AccountUserIDHeader) == "" && signsInOnlyAtConnect(r.Header.Get("User-Agent")) {
			w.Header().Set("WWW-Authenticate", mixedChallenge(origin))
			http.Error(w, "sign in to freehire to connect", http.StatusUnauthorized)
			return
		}
		if r.Header.Get(AccountUserIDHeader) == "" && r.Method == http.MethodPost &&
			!strings.HasPrefix(r.Header.Get("User-Agent"), "openai-mcp") {
			body, err := io.ReadAll(r.Body)
			if err != nil {
				http.Error(w, "unreadable request body", http.StatusBadRequest)
				return
			}
			r.Body = io.NopCloser(bytes.NewReader(body))
			var call struct {
				Method string `json:"method"`
				Params struct {
					Name string `json:"name"`
				} `json:"params"`
			}
			if json.Unmarshal(body, &call) == nil && call.Method == "tools/call" && personal[call.Params.Name] {
				w.Header().Set("WWW-Authenticate", mixedChallenge(origin))
				http.Error(w, "sign in to freehire to use personal tools", http.StatusUnauthorized)
				return
			}
		}
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

// signsInOnlyAtConnect names the hosts whose sign-in exists only while a connector is
// being added (see MixedHandler). Read from production access logs.
func signsInOnlyAtConnect(userAgent string) bool {
	return strings.HasPrefix(userAgent, "grok-connectors-manager") || strings.HasPrefix(userAgent, "Claude-User")
}
