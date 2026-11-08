package mcpapp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strconv"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/strelov1/freehire/internal/ai/assistant"
	"github.com/strelov1/freehire/internal/platform/observability"
)

// Account initialization describes identity, capabilities and result states. The
// optional workflow lives in the packaged skill; permissions remain enforced by tools.
const accountServerInstructions = "This is the signed-in freehire account. " +
	"Catalogue reads are public; personal tools change this account. Geography filters form a union. "

const accountAutoApplyInstructions = "Auto-apply prepares applications for review; saved answers may be sent to employers. "

const accountAlertInstructions = "Alerts use configured channels. "

const accountDictionaryInstructions = "Use suggest_dictionary_term for an unknown skill, specialization or industry; " +
	"zero search hits alone is not a gap. "

const accountProblemInstructions = "Use report_problem for failed operations or missing capabilities; " +
	"omit secrets and personal documents. Do not report report failures."

// NewAccountServer builds the per-request signed-in MCP server for one resolved
// user. tools is the caller's assistantDiscoveryTools() + assistantTrackingTools()
// — the same capabilities the in-app assistant already has. reader adds the catalogue
// tools it does not own; nil omits them for focused account-tool tests.
func NewAccountServer(tools []assistant.Tool, userID int64, origin string, reader Reader) *mcp.Server {
	server := mcp.NewServer(Identity("freehire-account", Version, origin),
		&mcp.ServerOptions{
			Instructions: accountInstructions(tools),
			// The authenticated tool list is fixed for this request's server.
			Capabilities: &mcp.ServerCapabilities{
				Logging: &mcp.LoggingCapabilities{}, //nolint:staticcheck // Preserve the SDK default for older MCP clients.
				Tools:   &mcp.ToolCapabilities{},
			},
		})
	ObserveServer(server, "account")

	if reader != nil {
		registerCatalogueTools(server, reader, "account")
	}

	for _, t := range tools {
		server.AddTool(&mcp.Tool{
			Name:        t.Name,
			Title:       t.Title,
			Description: t.Description,
			InputSchema: t.Schema,
			Annotations: accountAnnotations(t),
		}, loggedAccountTool(t.Name, accountToolHandler(t, userID)))
	}
	return server
}

// accountInstructions is the account server's instructions for the tools this session is
// actually served: each workflow paragraph only when the tool it begins with is among them.
func accountInstructions(tools []assistant.Tool) string {
	served := func(name string) bool {
		return slices.ContainsFunc(tools, func(t assistant.Tool) bool { return t.Name == name })
	}
	out := accountServerInstructions
	if served("auto_apply_start") {
		out += accountAutoApplyInstructions
	}
	if served("subscribe") {
		out += accountAlertInstructions
	}
	if served("suggest_dictionary_term") {
		out += accountDictionaryInstructions
	}
	if served("report_problem") {
		out += accountProblemInstructions
	}
	return out
}

// accountAnnotations declares what a tool does to the account, from the effect the tool
// itself carries. A client reads these to decide whether to ask the person before a call,
// so they are not decoration: unset, every search is a confirmation prompt. Every tool is
// open-world for the same reason the anonymous server's are: the catalogue and the
// tracking board both change outside this conversation, on the website.
func accountAnnotations(t assistant.Tool) *mcp.ToolAnnotations {
	destructive, openWorld := t.Effect == assistant.EffectOverwrites, true
	return &mcp.ToolAnnotations{
		Title:           t.Title,
		IdempotentHint:  t.Idempotent,
		ReadOnlyHint:    t.Effect == assistant.EffectReads,
		DestructiveHint: &destructive,
		OpenWorldHint:   &openWorld,
	}
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
			code := observability.ErrorCode(err)
			if errors.Is(err, assistant.ErrInvalidArguments) {
				code = "invalid_request"
			}
			result := errorResult(err.Error())
			result.Meta = mcp.Meta{diagnosticsMetaKey: map[string]any{"error_code": code}}
			return result, nil
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
// (a DB error, not an unknown token) must be reported as the 500 it is, and
// only a true Fiber handler returning a Go error gets that for free.
const AccountUserIDHeader = "X-Freehire-Account-User-Id"

// AccountHandler builds the HTTP handler for the signed-in MCP server. It
// trusts AccountUserIDHeader entirely — a request reaching it with no valid
// value there is this package's own wiring bug, not a caller's, so it answers
// 500 rather than 401.
func AccountHandler(tools []assistant.Tool, origin string, reader Reader) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userID, err := strconv.ParseInt(r.Header.Get(AccountUserIDHeader), 10, 64)
		if err != nil {
			http.Error(w, "internal error: no authenticated user", http.StatusInternalServerError)
			return
		}
		mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server {
			return NewAccountServer(
				tools,
				userID,
				origin,
				reader,
			)
		}, &mcp.StreamableHTTPOptions{Stateless: true}).ServeHTTP(w, r)
	})
}
