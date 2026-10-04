package mcpapp

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"

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
			return errorResult(err.Error()), nil
		}
		payload, err := json.Marshal(out)
		if err != nil {
			return errorResult("could not encode the result"), nil
		}
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: string(payload)}}}, nil
	}
}

func errorResult(text string) *mcp.CallToolResult {
	return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: text}}}
}

// AccountUserIDHeader is the internal, request-scoped header AccountHandler
// trusts for the caller's resolved user id. It is set by the Fiber middleware
// that authenticates the bearer token BEFORE this handler ever runs (see
// mountMCPAccount in internal/api/handler) — never read from, or trusted from,
// an external caller.
//
// Authentication lives in that middleware rather than here because a bare
// net/http.Handler (this one, wrapped via adaptor.HTTPHandler) has no route
// back into Fiber's error pipeline: an infra failure authenticating the token
// (a DB error, not an unknown token) must reach Sentry as the 500 it is, and
// only a true Fiber handler returning a Go error gets that for free.
const AccountUserIDHeader = "X-Freehire-Account-User-Id"

// AccountHandler builds the HTTP handler for the signed-in MCP server. It
// trusts AccountUserIDHeader entirely — a request reaching it with no valid
// value there is this package's own wiring bug, not a caller's, so it answers
// 500 rather than 401.
func AccountHandler(tools []assistant.Tool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userID, err := strconv.ParseInt(r.Header.Get(AccountUserIDHeader), 10, 64)
		if err != nil {
			http.Error(w, "internal error: no authenticated user", http.StatusInternalServerError)
			return
		}
		mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server {
			return NewAccountServer(tools, userID)
		}, &mcp.StreamableHTTPOptions{Stateless: true}).ServeHTTP(w, r)
	})
}
