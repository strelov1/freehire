package mcpapp

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/strelov1/freehire/internal/ai/assistant"
	"github.com/strelov1/freehire/internal/identity/auth/oauth2server"
)

// ResourceMetadataPath is the RFC 9728 path-inserted metadata of /api/v1/mcp. Clients
// following the 2025-11-25 discovery order ask for it before the root document.
const ResourceMetadataPath = "/.well-known/oauth-protected-resource" + oauth2server.MCPResourcePath

// surfaceMCP labels this server's tool logs and metrics. "mixed" is a legacy value kept so
// the Grafana series continue.
const surfaceMCP = "mixed"

// oauthOnly is every tool's securitySchemes: the server admits no anonymous caller, so the
// public catalogue tools need the same sign-in as the personal ones.
var oauthOnly = []map[string]any{{"type": "oauth2", "scopes": []string{}}}

// addCatalogueTool registers a catalogue tool on this server: observed under surfaceMCP and
// declaring the same OAuth requirement as every other tool here.
func addCatalogueTool[In, Out any](server *mcp.Server, tool *mcp.Tool, handler mcp.ToolHandlerFor[In, Out]) {
	if tool.Meta == nil {
		tool.Meta = mcp.Meta{}
	}
	tool.Meta["securitySchemes"] = oauthOnly
	AddObservedTool(server, surfaceMCP, tool, handler)
}

// catalogueOwned are the in-app assistant tool names the public catalogue tools serve instead,
// with their own schemas, previews and widgets.
var catalogueOwned = map[string]bool{toolSearchJobs: true, toolGetJob: true, toolGetCompany: true}

// cliSessionInstructions ends the instructions of a session the freehire CLI opened
// (`freehire mcp`). That agent has a shell, so context-flooding work can go to disk through
// the CLI; nothing else tells it the CLI is there. It grants nothing.
const cliSessionInstructions = "The freehire CLI is on this shell: `freehire search <query> --json > jobs.json` " +
	"saves whole results with full descriptions to grep; `freehire cv render <id> --out cv.pdf` writes the CV PDF."

func newServer(tools []assistant.Tool, userID int64, origin string, reader Reader, viaCLI bool) *mcp.Server {
	instructions := "Geography filters form a union. " + accountInstructions(tools)
	if viaCLI {
		// The account paragraphs do not all end in a space (report_problem's does not).
		instructions = strings.TrimRight(instructions, " ") + " " + cliSessionInstructions
	}
	server := newCatalogueServer(reader, origin, instructions, signedInNext(tools))
	addPromptSpecs(server, accountPrompts(tools))
	for _, tool := range tools {
		// Preserve the catalogue's schemas, previews and widgets for the tools it owns.
		if catalogueOwned[tool.Name] {
			continue
		}
		server.AddTool(&mcp.Tool{
			Name: tool.Name, Title: tool.Title, Description: tool.Description,
			InputSchema: tool.Schema, Annotations: accountAnnotations(tool),
			Meta: mcp.Meta{"securitySchemes": oauthOnly},
		}, loggedTool(surfaceMCP, tool.Name, accountToolHandler(tool, userID)))
	}
	return server
}

// Handler serves the catalogue and account tools to the user the authentication middleware
// stamped on the request. That middleware answers a credential-less request 401 (the MCP
// signal that sign-in is needed) before it gets here, so a request without a valid identity
// is a wiring error, never an anonymous session.
func Handler(tools []assistant.Tool, origin string, reader Reader) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userID, err := strconv.ParseInt(r.Header.Get(AccountUserIDHeader), 10, 64)
		if err != nil || userID <= 0 {
			http.Error(w, "internal error: no authenticated user", http.StatusInternalServerError)
			return
		}
		viaCLI := strings.HasPrefix(r.Header.Get("User-Agent"), "freehire-cli/")
		StatelessHandler(func() *mcp.Server {
			return newServer(tools, userID, origin, reader, viaCLI)
		}).ServeHTTP(w, r)
	})
}
