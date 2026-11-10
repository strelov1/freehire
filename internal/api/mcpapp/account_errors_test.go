package mcpapp

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/ai/assistant"
)

// A third-party agent reads a tool's error verbatim, so only text written for the caller
// may reach it; an internal fault's text (ids, SQL, personal data) must not.
func TestAgentErrorText_ShowsOnlyCallerMessages(t *testing.T) {
	for err, want := range map[error]string{
		fiber.NewError(fiber.StatusBadRequest, "slug is required"): "slug is required",
		// A wrapper's internal context stays out; only the fiber error's own message is shown.
		fmt.Errorf("load member 4242: %w", fiber.NewError(fiber.StatusNotFound, "no such answer")): "no such answer",
		fmt.Errorf("%w: unexpected field", assistant.ErrInvalidArguments):                          "invalid arguments: unexpected field",
	} {
		if got := agentErrorText(err); got != want {
			t.Errorf("agentErrorText(%q) = %q, want %q", err, got, want)
		}
	}
	internal := fmt.Errorf("load member 4242 (jane@example.com): %w", errors.New("pq: relation missing"))
	if got := agentErrorText(internal); strings.Contains(got, "4242") || strings.Contains(got, "jane") || strings.Contains(got, "pq") {
		t.Errorf("internal detail reached the agent: %q", got)
	}
	if got := agentErrorText(fmt.Errorf("query: %w", context.DeadlineExceeded)); strings.Contains(got, "query") {
		t.Errorf("deadline text leaked: %q", got)
	}
}
