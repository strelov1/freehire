package mcpapp

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/strelov1/freehire/internal/ai/assistant"
)

type logRecords chan string

func (records logRecords) Write(p []byte) (int, error) {
	records <- string(p)
	return len(p), nil
}

func captureToolLogs(t *testing.T) logRecords {
	t.Helper()
	records := make(logRecords, 32)
	previous := slog.Default()
	output, flags := log.Writer(), log.Flags()
	slog.SetDefault(slog.New(slog.NewJSONHandler(records, nil)))
	t.Cleanup(func() {
		slog.SetDefault(previous)
		log.SetOutput(output)
		log.SetFlags(flags)
	})
	return records
}

func nextToolLog(t *testing.T, records logRecords) map[string]any {
	t.Helper()
	select {
	case line := <-records:
		var record map[string]any
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Fatal(err)
		}
		return record
	case <-time.After(5 * time.Second):
		t.Fatal("missing tool log")
		return nil
	}
}

func TestToolLogsSpanActualExecutionAndExcludePayloads(t *testing.T) {
	const private = "private-query-token-and-result@example.invalid"
	for _, surface := range []string{"public", "account"} {
		t.Run(surface, func(t *testing.T) {
			records := captureToolLogs(t)
			release := make(chan struct{})
			// Unblock a failed test before closing its SDK sessions.
			unblock := sync.OnceFunc(func() { close(release) })
			defer unblock()
			var server *mcp.Server
			if surface == "public" {
				reader := readerThatFinds(t)
				reader.searchJobs = func(ctx context.Context, in SearchInput) (SearchJobsResult, error) {
					select {
					case <-release:
					case <-ctx.Done():
						return SearchJobsResult{}, ctx.Err()
					}
					return SearchJobsResult{Query: in.Query}, nil
				}
				server = newServer(reader, "")
			} else {
				server = NewAccountServer(
					[]assistant.Tool{{Name: toolSearchJobs, Schema: map[string]any{"type": "object"}, Run: func(ctx context.Context, _ int64, _ json.RawMessage) (any, error) {
						select {
						case <-release:
						case <-ctx.Done():
							return nil, ctx.Err()
						}
						return map[string]string{"private": private}, nil
					}}},
					987654321,
					"",
					nil,
				)
			}
			clientTransport, serverTransport := mcp.NewInMemoryTransports()
			serverSession, err := server.Connect(t.Context(), serverTransport, nil)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = serverSession.Close() })
			session, err := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "0"}, nil).Connect(t.Context(), clientTransport, nil)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = session.Close() })
			done := make(chan error, 1)
			go func() {
				result, err := session.CallTool(t.Context(), &mcp.CallToolParams{Name: toolSearchJobs, Arguments: map[string]any{"query": private}})
				if err == nil && result.IsError {
					err = errors.New("tool returned an error")
				}
				done <- err
			}()
			start := nextToolLog(t, records)
			if start["msg"] != "mcp tool started" || start["tool"] != toolSearchJobs || start["surface"] != surface {
				t.Fatalf("unexpected start: %v", start)
			}
			select {
			case err := <-done:
				t.Fatalf("call finished before release: %v", err)
			default:
			}
			unblock()
			select {
			case err := <-done:
				if err != nil {
					t.Fatal(err)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("tool never finished")
			}
			finish := nextToolLog(t, records)
			if finish["msg"] != "mcp tool finished" || finish["outcome"] != "success" || finish["call_id"] != start["call_id"] {
				t.Fatalf("unexpected finish: %v", finish)
			}
			if _, ok := finish["duration_ms"].(float64); !ok {
				t.Fatal("missing execution duration")
			}
			_, _ = session.CallTool(t.Context(), &mcp.CallToolParams{Name: private})
			unknownStart, unknownFinish := nextToolLog(t, records), nextToolLog(t, records)
			if unknownStart["tool"] != "unknown" || unknownFinish["tool"] != "unknown" || unknownFinish["outcome"] != "rejected" {
				t.Fatalf("unknown tool was not safely classified: %v", unknownFinish)
			}
			raw, _ := json.Marshal([]map[string]any{unknownStart, unknownFinish})
			if strings.Contains(string(raw), private) {
				t.Fatalf("unregistered tool name leaked: %s", raw)
			}
			for _, record := range []map[string]any{start, finish} {
				raw, _ := json.Marshal(record)
				if strings.Contains(string(raw), private) || strings.Contains(string(raw), "987654321") {
					t.Fatalf("private data logged: %s", raw)
				}
			}
		})
	}
}

func TestToolLogClassifiesFailuresWithoutErrorText(t *testing.T) {
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	expired, cancelDeadline := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancelDeadline()
	for _, tc := range []struct {
		name   string
		ctx    context.Context
		result *mcp.CallToolResult
		err    error
		want   string
		code   string
	}{
		{name: "error", ctx: context.Background(), err: errors.New("private-error-secret"), want: "error", code: "internal_error"},
		{name: "tool_error", ctx: context.Background(), result: &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: "private-error-secret"}}}, want: "tool_error", code: "internal_error"},
		{name: "untrusted_code", ctx: context.Background(), result: &mcp.CallToolResult{IsError: true, Meta: mcp.Meta{diagnosticsMetaKey: map[string]any{"error_code": "private-error-secret"}}}, want: "tool_error", code: "internal_error"},
		{name: "canceled", ctx: context.Background(), err: context.Canceled, want: "canceled", code: "canceled"},
		{name: "deadline", ctx: context.Background(), err: context.DeadlineExceeded, want: "deadline_exceeded", code: "deadline_exceeded"},
		{name: "canceled_tool_result", ctx: canceled, result: &mcp.CallToolResult{IsError: true}, want: "canceled", code: "canceled"},
		{name: "expired_tool_result", ctx: expired, result: &mcp.CallToolResult{IsError: true}, want: "deadline_exceeded", code: "deadline_exceeded"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			records := captureToolLogs(t)
			finish := observeTool(tc.ctx, "public", toolSearchJobs)
			nextToolLog(t, records)
			finish(tc.result, tc.err)
			record := nextToolLog(t, records)
			raw, _ := json.Marshal(record)
			if record["outcome"] != tc.want || record["error_code"] != tc.code || strings.Contains(string(raw), "private-error-secret") {
				t.Fatalf("unexpected failure log: %s", raw)
			}
			if tc.result != nil && tc.result.IsError {
				diagnostics, _ := tc.result.Meta[diagnosticsMetaKey].(map[string]any)
				if diagnostics["error_code"] != tc.code {
					t.Errorf("response cause differs from log: %v", diagnostics)
				}
			}
		})
	}
}
