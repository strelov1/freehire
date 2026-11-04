package mcpapp

import (
	"sync/atomic"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	posthog "github.com/posthog/posthog-go"
	"github.com/posthog/posthog-go/posthogmcp"
	"github.com/posthog/posthog-go/posthogmcpsdk"
)

// serverAnalytics is set once at startup, before the first request, and only when PostHog is
// configured. Unset, every MCP server runs exactly as it did before.
var serverAnalytics atomic.Pointer[analyticsSinks]

// analyticsSinks keeps the account server's events apart: its arguments, results and error
// text are the account's own data, so only the tool name, outcome and timing may leave.
type analyticsSinks struct {
	public  *posthogmcp.Analytics
	account *posthogmcp.Analytics
}

// EnableAnalytics turns on PostHog MCP analytics for the MCP servers.
func EnableAnalytics(client posthog.EnqueueClient) {
	serverAnalytics.Store(&analyticsSinks{
		public: posthogmcp.New(client),
		// A failed call's text would also go out as a $exception event, which the account
		// sink never sends; accountMetadataOnly drops every other non-allowlisted field.
		account: posthogmcp.New(accountMetadataOnly{client}, posthogmcp.WithExceptionAutocapture(false)),
	})
}

// Instrument reports each tool call of a per-request public server to PostHog, under the
// server's own name and version so events from the servers stay apart.
func Instrument(server *mcp.Server, name, version string) {
	if sinks := serverAnalytics.Load(); sinks != nil {
		instrument(server, sinks.public, name, version)
	}
}

func instrumentAccount(server *mcp.Server) {
	if sinks := serverAnalytics.Load(); sinks != nil {
		instrument(server, sinks.account, "freehire-account", Version,
			posthogmcpsdk.WithCaptureParameters(false),
			posthogmcpsdk.WithCaptureResponses(false))
	}
}

// instrument keeps the three argument-injecting options off: the servers are stateless and
// rebuilt on every request, so adding a required `context` or `llm_model` argument would
// change the tool schemas that ChatGPT and Claude have already learned.
func instrument(server *mcp.Server, a *posthogmcp.Analytics, name, version string, opts ...posthogmcpsdk.Option) {
	posthogmcpsdk.Instrument(server, a, append([]posthogmcpsdk.Option{
		posthogmcpsdk.WithServerInfo(name, version),
		posthogmcpsdk.WithContextParameter(false),
		posthogmcpsdk.WithCaptureModel(false),
		posthogmcpsdk.WithConversationID(false),
	}, opts...)...)
}

// accountEventProperties are the only properties an account event may carry: what the
// server itself sets (tool, outcome, timing, session). Anything the caller authors — its
// User-Agent, client name, arguments, results, error text — and any field a later SDK adds
// is dropped, so the boundary holds by construction rather than by a list of known leaks.
var accountEventProperties = map[string]bool{
	"$mcp_tool_name":          true,
	"$mcp_resource_name":      true,
	"$mcp_server_name":        true,
	"$mcp_server_version":     true,
	"$mcp_protocol_version":   true,
	"$mcp_is_error":           true,
	"$mcp_error_type":         true,
	"$mcp_duration_ms":        true,
	"$mcp_source":             true,
	"$session_id":             true,
	"$process_person_profile": true,
}

// accountMetadataOnly forwards an account $mcp_tool_call with its properties cut down to
// accountEventProperties, and nothing else: every other event (an unknown tool, named by
// the caller) is dropped.
type accountMetadataOnly struct{ posthog.EnqueueClient }

func (c accountMetadataOnly) Enqueue(msg posthog.Message) error {
	capture, ok := msg.(posthog.Capture)
	if !ok || capture.Event != "$mcp_tool_call" {
		return nil
	}
	kept := posthog.NewProperties()
	for key, value := range capture.Properties {
		if accountEventProperties[key] {
			kept[key] = value
		}
	}
	capture.Properties = kept
	return c.EnqueueClient.Enqueue(capture)
}
