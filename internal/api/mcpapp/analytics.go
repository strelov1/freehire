package mcpapp

import (
	"sync/atomic"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/posthog/posthog-go/posthogmcp"
	"github.com/posthog/posthog-go/posthogmcpsdk"
)

// analytics is set once at startup, before the first request, and only when PostHog is
// configured. Unset, every MCP server runs exactly as it did before.
var analytics atomic.Pointer[posthogmcp.Analytics]

// EnableAnalytics turns on PostHog MCP analytics for the MCP servers.
func EnableAnalytics(a *posthogmcp.Analytics) { analytics.Store(a) }

// Instrument reports each tool call of a per-request server to PostHog, under the server's
// own name and version so events from the three servers stay apart. The three
// argument-injecting options stay off: the servers are stateless and rebuilt on every
// request, so adding a required `context` or `llm_model` argument would change the tool
// schemas that ChatGPT and Claude have already learned.
func Instrument(server *mcp.Server, name, version string, opts ...posthogmcpsdk.Option) {
	a := analytics.Load()
	if a == nil {
		return
	}
	posthogmcpsdk.Instrument(server, a, append([]posthogmcpsdk.Option{
		posthogmcpsdk.WithServerInfo(name, version),
		posthogmcpsdk.WithContextParameter(false),
		posthogmcpsdk.WithCaptureModel(false),
		posthogmcpsdk.WithConversationID(false),
	}, opts...)...)
}
