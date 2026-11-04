package mcpapp

import (
	"maps"
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
		// A failed call's text reaches PostHog twice: as $mcp_error_message and as a
		// $exception event. The SDK sends both even with responses off, so the exception
		// is switched off here and the message is cut before the event is queued.
		account: posthogmcp.New(withoutErrorText{client}, posthogmcp.WithExceptionAutocapture(false)),
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

// withoutErrorText drops a failed call's error text from every event before it is queued.
type withoutErrorText struct{ posthog.EnqueueClient }

func (c withoutErrorText) Enqueue(msg posthog.Message) error {
	if capture, ok := msg.(posthog.Capture); ok {
		capture.Properties = maps.Clone(capture.Properties)
		delete(capture.Properties, "$mcp_error_message")
		msg = capture
	}
	return c.EnqueueClient.Enqueue(msg)
}
