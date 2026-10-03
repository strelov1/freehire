package mcpapp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/strelov1/freehire/internal/ai/assistant"
)

// accountServerInstructions tells the model this surface acts as a specific,
// already-authenticated person — the opposite framing from serverInstructions
// above, which warns an anonymous aggregator view apart from the catalogue.
const accountServerInstructions = "This server acts as the signed-in freehire account that " +
	"approved this connection. Tools read and change that account's own saved jobs, " +
	"applications, and tracking board — never another account's."

// NewAccountServer builds the per-request signed-in MCP server for one resolved
// user. tools is the caller's assistantDiscoveryTools() + assistantTrackingTools()
// — the same capabilities the in-app assistant already has, reused rather than
// reimplemented so a tool result here and in the assistant can never disagree.
func NewAccountServer(tools []assistant.Tool, userID int64) *mcp.Server {
	server := mcp.NewServer(&mcp.Implementation{
		Name:    "freehire-account",
		Version: Version,
	}, &mcp.ServerOptions{Instructions: accountServerInstructions})

	for _, t := range tools {
		server.AddTool(&mcp.Tool{
			Name:        t.Name,
			Description: t.Description,
			InputSchema: t.Schema,
		}, accountToolHandler(t, userID))
	}
	return server
}

// accountToolHandler closes over the request's resolved userID — set once, when
// NewAccountServer is built per-request in AccountHandler's factory below — so
// every call this session makes runs as that user and no other. The JSON error
// envelope mirrors assistant.Registry.Call's own failure rendering, reproduced
// here rather than reused directly since the SDK's CallToolResult shape differs
// from assistant.Result.
func accountToolHandler(t assistant.Tool, userID int64) mcp.ToolHandler {
	return func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		out, err := t.Run(ctx, userID, req.Params.Arguments)
		if err != nil {
			return &mcp.CallToolResult{
				IsError: true,
				Content: []mcp.Content{&mcp.TextContent{Text: err.Error()}},
			}, nil
		}
		payload, err := json.Marshal(out)
		if err != nil {
			return &mcp.CallToolResult{
				IsError: true,
				Content: []mcp.Content{&mcp.TextContent{Text: "could not encode the result"}},
			}, nil
		}
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: string(payload)}}}, nil
	}
}

// ErrUnauthorized is what AccountHandler's factory reports for a missing,
// malformed, or unrecognized bearer token.
var ErrUnauthorized = errors.New("mcpapp: unauthorized")

// AccountHandler builds the HTTP handler for the signed-in MCP server. Unlike
// Handler above, every request must authenticate: authenticate resolves the
// Authorization: Bearer token to a user id, and a failure answers with no
// server at all — the stdlib handler 401s before any MCP framing is attempted,
// the same posture as every other bearer-gated surface.
func AccountHandler(tools []assistant.Tool, authenticate func(ctx context.Context, bearerToken string) (int64, error)) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token, ok := bearerToken(r)
		if !ok {
			http.Error(w, "missing bearer token", http.StatusUnauthorized)
			return
		}
		userID, err := authenticate(r.Context(), token)
		if err != nil {
			http.Error(w, "invalid or expired token", http.StatusUnauthorized)
			return
		}
		mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server {
			return NewAccountServer(tools, userID)
		}, &mcp.StreamableHTTPOptions{Stateless: true}).ServeHTTP(w, r)
	})
}

func bearerToken(r *http.Request) (string, bool) {
	h := r.Header.Get("Authorization")
	const prefix = "Bearer "
	if !strings.HasPrefix(h, prefix) {
		return "", false
	}
	return strings.TrimPrefix(h, prefix), true
}
