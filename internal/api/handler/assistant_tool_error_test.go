package handler

import (
	"testing"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/ai/plan"
	"github.com/strelov1/freehire/internal/platform/observability"
)

// An account tool's refusal keeps its kind for the MCP metrics and logs: on 2026-11-04 every
// auto_apply_start refusal (switch off, free plan) was counted as internal_error, because
// toolError rebuilt each one as a bare error. The agent-facing text must not change.
func TestToolErrorKeepsTheRefusalKind(t *testing.T) {
	for _, tc := range []struct {
		name, code, text string
		err              error
	}{
		{"unsupported ATS", "invalid_request", "auto-apply cannot queue an attempt against this posting's ATS",
			fiber.NewError(fiber.StatusBadRequest, "auto-apply cannot queue an attempt against this posting's ATS")},
		{"free plan", "payment_required", plan.ErrPaymentRequired.Error(), plan.ErrPaymentRequired},
		{"allowance spent", "rate_limited", plan.ErrRefused.Error(), plan.ErrRefused},
		{"switch off", "permission_denied", agentsMayApplyOff, errAgentsMayApplyOff},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := toolError(tc.err)
			if code := observability.ErrorCode(got); code != tc.code {
				t.Errorf("code = %q, want %q", code, tc.code)
			}
			if got.Error() != tc.text {
				t.Errorf("text = %q, want %q", got.Error(), tc.text)
			}
		})
	}
}
