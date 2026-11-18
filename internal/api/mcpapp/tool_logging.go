package mcpapp

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode"

	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/strelov1/freehire/internal/platform/observability"
)

var toolCallSequence atomic.Uint64

// mcpToolCalls and mcpToolDuration put MCP traffic in Grafana. Every label is bounded:
// surface and tool come from registrations, outcome from the switch below, error_code from
// resultErrorCode's closed list or observability.ErrorCode — never from request data.
var mcpToolCalls = promauto.NewCounterVec(prometheus.CounterOpts{
	Name: "freehire_mcp_tool_calls_total",
	Help: "MCP tool calls by surface (mixed: the hosted MCP, ojcp), tool, outcome and error code.",
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
	if state, ok := ctx.Value(handlerRanKey{}).(*callState); ok {
		state.ran = true
		return func(_ *mcp.CallToolResult, err error) { state.err = err }
	}
	logger := observability.RequestLogger(ctx).With("surface", surface, "tool", name, "call_id", toolCallSequence.Add(1))
	if metadata, ok := ctx.Value(callMetadataKey{}).(*callMetadata); ok {
		logger = logger.With("client", metadata.client, "conversation_id", metadata.conversation)
	}
	started := time.Now()
	logger.InfoContext(ctx, "mcp tool started")
	return func(result *mcp.CallToolResult, err error) {
		metadata, _ := ctx.Value(callMetadataKey{}).(*callMetadata)
		if err == nil && metadata != nil {
			err = metadata.state.err
		}
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
		var protocolError *jsonrpc.Error
		if errors.As(err, &protocolError) && protocolError.Code == jsonrpc.CodeInvalidParams {
			code = "invalid_request"
		}
		if outcome == "canceled" || outcome == "deadline_exceeded" {
			code = outcome
		}
		var badArgs []string
		var reason string
		if metadata != nil && !metadata.state.ran {
			outcome, code = "rejected", "invalid_request"
			badArgs = badArgNames(result)
		}
		if result != nil && result.IsError {
			if code == "" {
				code = resultErrorCode(result)
			}
			reason = handlerReason(result)
			if code == "invalid_request" && badArgs == nil {
				badArgs = handlerBadArgs(result)
			}
			if result.Meta == nil {
				result.Meta = mcp.Meta{}
			}
			result.Meta[diagnosticsMetaKey] = map[string]any{"request_id": observability.RequestID(ctx), "error_code": code}
		}
		elapsed := time.Since(started)
		mcpToolCalls.WithLabelValues(surface, name, outcome, code).Inc()
		mcpToolDuration.WithLabelValues(surface, name).Observe(elapsed.Seconds())
		attrs := []any{"outcome", outcome, "error_code", code, "duration_ms", elapsed.Milliseconds()}
		if len(badArgs) > 0 {
			attrs = append(attrs, "bad_args", strings.Join(badArgs, ","))
		}
		if reason != "" {
			attrs = append(attrs, "error", reason)
		}
		if metadata != nil {
			mcpClientCalls.WithLabelValues(surface, metadata.client).Inc()
			if outcome == "success" && metadata.reportCategory != "" {
				mcpProblemReports.WithLabelValues(surface, metadata.reportCategory).Inc()
				attrs = append(attrs, "report_category", metadata.reportCategory)
			}
		}
		if result != nil {
			if encoded, encodeErr := json.Marshal(result); encodeErr == nil {
				mcpResponseBytes.WithLabelValues(surface, name).Observe(float64(len(encoded)))
				attrs = append(attrs, "response_bytes", len(encoded))
			}
			if outcome == "success" {
				attrs = append(attrs, observeSearchQuality(surface, name, result)...)
			}
		}
		logger.InfoContext(ctx, "mcp tool finished", attrs...)
	}
}

const diagnosticsMetaKey = "freehire.me/diagnostics"

// badArgsKey carries, inside the diagnostics, the names of the arguments a handler blamed
// for an invalid_request. observeTool logs them and replaces the diagnostics before the
// result leaves, so they never reach the client.
const badArgsKey = "bad_args"

// reasonKey carries, inside the diagnostics, the sentence a handler authored for a refusal
// (never a result's text: that can echo the caller's input). observeTool logs it and replaces
// the diagnostics before the result leaves.
const reasonKey = "reason"

const maxLoggedReasonRunes = 200

// handlerReason returns the handler-authored reason, flattened to one bounded line.
func handlerReason(result *mcp.CallToolResult) string {
	diagnostics, _ := result.Meta[diagnosticsMetaKey].(map[string]any)
	text, _ := diagnostics[reasonKey].(string)
	return sanitizeReason(text)
}

func sanitizeReason(text string) string {
	var b strings.Builder
	runes, space := 0, false
	for _, r := range text {
		if unicode.IsSpace(r) {
			space = b.Len() > 0
			continue
		}
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) {
			continue
		}
		if runes >= maxLoggedReasonRunes {
			b.WriteString("...")
			break
		}
		if space {
			b.WriteByte(' ')
			runes++
			space = false
		}
		b.WriteRune(r)
		runes++
	}
	return b.String()
}

func handlerBadArgs(result *mcp.CallToolResult) []string {
	diagnostics, _ := result.Meta[diagnosticsMetaKey].(map[string]any)
	names, _ := diagnostics[badArgsKey].([]string)
	return loggableNames(names)
}

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

// AddObservedTool registers a typed tool with local execution diagnostics.
func AddObservedTool[In, Out any](
	server *mcp.Server,
	surface string,
	tool *mcp.Tool,
	handler mcp.ToolHandlerFor[In, Out],
) {
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

// ObserveServer records final SDK outcomes, including validation failures before and
// after a handler. It needs no external analytics client.
func ObserveServer(server *mcp.Server, surface string) {
	server.AddReceivingMiddleware(toolCallMiddleware(surface))
}

func toolCallMiddleware(surface string) mcp.Middleware {
	return func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
			call, ok := req.(*mcp.CallToolRequest)
			if method != "tools/call" || !ok {
				return next(ctx, method, req)
			}
			metadata := metadataForCall(call)
			ctx = context.WithValue(ctx, callMetadataKey{}, metadata)
			finish := observeTool(ctx, surface, toolLabel(call.Params.Name))
			res, err := next(context.WithValue(ctx, handlerRanKey{}, &metadata.state), method, req)
			result, _ := res.(*mcp.CallToolResult)
			finish(result, err)
			return res, err
		}
	}
}
