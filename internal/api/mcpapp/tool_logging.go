package mcpapp

import (
	"context"
	"errors"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

var toolCallSequence atomic.Uint64

// observeTool measures execution, rather than the Fiber adaptor's return after
// opening an SSE stream. Names come from registrations, never from request data.
// A start without a finish identifies a call that is still running or was lost
// when the process exited. Neither errors nor arguments are safe to log here.
func observeTool(ctx context.Context, surface, name string) func(*mcp.CallToolResult, error) {
	logger := slog.Default().With("surface", surface, "tool", name, "call_id", toolCallSequence.Add(1))
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
		logger.InfoContext(ctx, "mcp tool finished", "outcome", outcome, "duration_ms", time.Since(started).Milliseconds())
	}
}

func addLoggedTool[In, Out any](server *mcp.Server, tool *mcp.Tool, handler mcp.ToolHandlerFor[In, Out]) {
	// Capture the registered name so a caller cannot inject a name into the log.
	name := tool.Name
	mcp.AddTool(server, tool, func(ctx context.Context, req *mcp.CallToolRequest, in In) (result *mcp.CallToolResult, output Out, err error) {
		finish := observeTool(ctx, "public", name)
		// A panic deliberately leaves the start unmatched; do not recover it or log
		// its value, which can contain input or other sensitive data.
		result, output, err = handler(ctx, req, in)
		finish(result, err)
		return result, output, err
	})
}

func loggedAccountTool(name string, handler mcp.ToolHandler) mcp.ToolHandler {
	return func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		finish := observeTool(ctx, "account", name)
		result, err := handler(ctx, req)
		finish(result, err)
		return result, err
	}
}
