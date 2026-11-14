package mcpapp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/strelov1/freehire/internal/ai/assistant"
)

// The tool log may explain a refusal only with a constant the call site wrote; whatever the
// caller sent (a slug, a search text) must never reach it, on either surface.
func TestCallerInputNeverReachesTheToolLog(t *testing.T) {
	const secret = "caller-slug-secret@example.invalid"
	interpolated := fiber.NewError(fiber.StatusNotFound, fmt.Sprintf("no vacancy with slug %q", secret))
	probe := func(err error) assistant.Tool {
		return assistant.Tool{Name: "probe", Schema: map[string]any{"type": "object"},
			Run: func(context.Context, int64, json.RawMessage) (any, error) { return nil, err }}
	}
	finishLog := func(t *testing.T, session *mcp.ClientSession, tool string, records logRecords) (map[string]any, string) {
		t.Helper()
		if _, err := session.CallTool(t.Context(), &mcp.CallToolParams{Name: tool, Arguments: map[string]any{"slug": secret}}); err != nil {
			t.Fatal(err)
		}
		nextToolLog(t, records)
		finish := nextToolLog(t, records)
		raw, _ := json.Marshal(finish)
		return finish, string(raw)
	}
	t.Run("account interpolated fiber error", func(t *testing.T) {
		records := captureToolLogs(t)
		finish, raw := finishLog(t, connectSignedIn(t, []assistant.Tool{probe(interpolated)}), "probe", records)
		if strings.Contains(raw, secret) || finish["error"] != nil || finish["error_code"] != "not_found" {
			t.Fatalf("log = %s", raw)
		}
	})
	t.Run("account wrapped interpolated error", func(t *testing.T) {
		records := captureToolLogs(t)
		_, raw := finishLog(t, connectSignedIn(t, []assistant.Tool{probe(fmt.Errorf("lookup %s: %w", secret, interpolated))}), "probe", records)
		if strings.Contains(raw, secret) {
			t.Fatalf("log = %s", raw)
		}
	})
	t.Run("account opted-in constant reason", func(t *testing.T) {
		records := captureToolLogs(t)
		finish, raw := finishLog(t, connectSignedIn(t, []assistant.Tool{probe(assistant.WithLogReason(interpolated, "vacancy missing"))}), "probe", records)
		if strings.Contains(raw, secret) || finish["error"] != "vacancy missing" {
			t.Fatalf("log = %s", raw)
		}
	})
	t.Run("catalogue not found and generic", func(t *testing.T) {
		for _, failure := range []error{NotFoundError{What: secret}, fmt.Errorf("boom %s", secret)} {
			records := captureToolLogs(t)
			r := readerThatFinds(t)
			r.jobDetail = func(context.Context, string) (JobResult, error) { return JobResult{}, failure }
			if _, raw := finishLog(t, connect(t, r), toolGetJob, records); strings.Contains(raw, secret) {
				t.Fatalf("log = %s", raw)
			}
		}
	})
}
