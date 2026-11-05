package mcpapp

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/strelov1/freehire/internal/platform/observability"
)

var toolCallSequence atomic.Uint64

// mcpToolCalls and mcpToolDuration put MCP traffic in Grafana on our own Prometheus: the
// PostHog product-analytics quota can drop $mcp_tool_call for weeks. Every label is bounded:
// surface and tool come from registrations, outcome from the switch below, error_code from
// resultErrorCode's closed list or observability.ErrorCode — never from request data.
var mcpToolCalls = promauto.NewCounterVec(prometheus.CounterOpts{
	Name: "freehire_mcp_tool_calls_total",
	Help: "MCP tool calls by surface (public, account, mixed), tool, outcome and error code.",
}, []string{"surface", "tool", "outcome", "error_code"})

// The buckets reach 60s because an account tool can drive a model or a browser.
var mcpToolDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
	Name:    "freehire_mcp_tool_duration_seconds",
	Help:    "How long each MCP tool call took, by surface and tool.",
	Buckets: []float64{0.025, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30, 60},
}, []string{"surface", "tool"})

// observeTool measures execution, rather than the Fiber adaptor's return after
// opening an SSE stream. Names come from registrations, never from request data.
// A start without a finish identifies a call that is still running or was lost
// when the process exited. Neither errors nor arguments are safe to log here.
func observeTool(ctx context.Context, surface, name string) func(*mcp.CallToolResult, error) {
	if ran, ok := ctx.Value(handlerRanKey{}).(*bool); ok {
		*ran = true
	}
	logger := observability.RequestLogger(ctx).With("surface", surface, "tool", name, "call_id", toolCallSequence.Add(1))
	started := time.Now()
	logger.InfoContext(ctx, "mcp tool started")
	return func(result *mcp.CallToolResult, err error) {
		outcome := "success"
		switch {
		case errors.Is(err, context.DeadlineExceeded), errors.Is(ctx.Err(), context.DeadlineExceeded):
			outcome = "deadline_exceeded"
		case errors.Is(err, context.Canceled), errors.Is(ctx.Err(), context.Canceled):
			outcome = "canceled"
		case err != nil:
			outcome = "error"
		case result != nil && result.IsError:
			outcome = "tool_error"
		}
		code := observability.ErrorCode(err)
		if outcome == "canceled" || outcome == "deadline_exceeded" {
			code = outcome
		}
		if result != nil && result.IsError {
			if code == "" {
				code = resultErrorCode(result)
			}
			if result.Meta == nil {
				result.Meta = mcp.Meta{}
			}
			result.Meta[diagnosticsMetaKey] = map[string]any{"request_id": observability.RequestID(ctx), "error_code": code}
		}
		elapsed := time.Since(started)
		mcpToolCalls.WithLabelValues(surface, name, outcome, code).Inc()
		mcpToolDuration.WithLabelValues(surface, name).Observe(elapsed.Seconds())
		logger.InfoContext(ctx, "mcp tool finished", "outcome", outcome, "error_code", code, "duration_ms", elapsed.Milliseconds())
	}
}

const diagnosticsMetaKey = "freehire.me/diagnostics"

func resultErrorCode(result *mcp.CallToolResult) string {
	diagnostics, _ := result.Meta[diagnosticsMetaKey].(map[string]any)
	code, _ := diagnostics["error_code"].(string)
	// Only codes authored here may enter the log; a result's text is private.
	switch code {
	case "canceled", "deadline_exceeded", "not_found", "invalid_request", "unauthenticated", "payment_required", "permission_denied", "conflict", "rate_limited", "unavailable", "database_error", "internal_error":
		return code
	default:
		return "internal_error"
	}
}

func addLoggedToolForSurface[In, Out any](
	server *mcp.Server,
	surface string,
	tool *mcp.Tool,
	handler mcp.ToolHandlerFor[In, Out],
) {
	if surface == "mixed" {
		if tool.Meta == nil {
			tool.Meta = mcp.Meta{}
		}
		tool.Meta["securitySchemes"] = []map[string]any{{"type": "noauth"}}
	}
	// Capture the registered name so a caller cannot inject a name into the log.
	name := tool.Name
	registerTool(name)
	if tool.InputSchema == nil {
		tool.InputSchema = inputSchema[In]()
	}
	mcp.AddTool(server, tool, func(ctx context.Context, req *mcp.CallToolRequest, in In) (*mcp.CallToolResult, Out, error) {
		finish := observeTool(ctx, surface, name)
		// A panic deliberately leaves the start unmatched; do not recover it or log
		// its value, which can contain input or other sensitive data.
		result, output, err := handler(ctx, req, in)
		finish(result, err)
		return result, output, err
	})
}

func loggedAccountTool(name string, handler mcp.ToolHandler) mcp.ToolHandler {
	return loggedTool("account", name, handler)
}

func loggedTool(surface, name string, handler mcp.ToolHandler) mcp.ToolHandler {
	registerTool(name)
	return func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		finish := observeTool(ctx, surface, name)
		result, err := handler(ctx, req)
		finish(result, err)
		return result, err
	}
}

// registeredTools holds every tool name a server registered, the only names allowed into a
// metric label: a tools/call request names its tool, and that string is the caller's.
var registeredTools sync.Map

func registerTool(name string) { registeredTools.Store(name, struct{}{}) }

func toolLabel(name string) string {
	if _, ok := registeredTools.Load(name); ok {
		return name
	}
	return "unknown"
}

type handlerRanKey struct{}

// countRejectedToolCalls counts the tools/call requests the SDK answers without running a
// handler: arguments failing the input schema, or a tool that does not exist. observeTool sees
// only calls that reach a handler, and a refused call is exactly what an agent hits when it
// misreads the schema (work_mode: "remote" on 2026-11-04). It marks the context, observeTool
// flips the mark, and only an unflipped call is counted here, so no call is counted twice.
func countRejectedToolCalls(surface string) mcp.Middleware {
	return func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
			call, ok := req.(*mcp.CallToolRequest)
			if method != "tools/call" || !ok {
				return next(ctx, method, req)
			}
			ran := new(bool)
			started := time.Now()
			res, err := next(context.WithValue(ctx, handlerRanKey{}, ran), method, req)
			if !*ran {
				name := toolLabel(call.Params.Name)
				mcpToolCalls.WithLabelValues(surface, name, "rejected", "invalid_request").Inc()
				mcpToolDuration.WithLabelValues(surface, name).Observe(time.Since(started).Seconds())
			}
			return res, err
		}
	}
}
