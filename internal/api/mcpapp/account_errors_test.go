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
	for _, shown := range []error{
		fiber.NewError(fiber.StatusBadRequest, "slug is required"),
		fmt.Errorf("wrapped: %w", fiber.NewError(fiber.StatusNotFound, "no such answer")),
		fmt.Errorf("%w: unexpected field", assistant.ErrInvalidArguments),
	} {
		if got := agentErrorText(shown); got != shown.Error() {
			t.Errorf("agentErrorText(%q) = %q, want it shown", shown, got)
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
