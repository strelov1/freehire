package handler

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/ingest/directapply"
)

// Each refusal answers with its own status and its own text; a delivery failure answers
// with ErrDelivery's text only, never the transport's, which can name the employer.
func TestDirectApplyError(t *testing.T) {
	cases := []struct {
		err    error
		status int
	}{
		{fmt.Errorf("%w: %q needs an answer", directapply.ErrInvalid, "Stack"), fiber.StatusBadRequest},
		{directapply.ErrNotFound, fiber.StatusNotFound},
		{directapply.ErrUnverified, fiber.StatusForbidden},
		{directapply.ErrClosed, fiber.StatusGone},
		{directapply.ErrAlreadyApplied, fiber.StatusConflict},
		{directapply.ErrInProgress, fiber.StatusConflict},
		{directapply.ErrDailyLimit, fiber.StatusTooManyRequests},
		{fmt.Errorf("%w: %w", directapply.ErrDelivery, errors.New("ses: rejected hr@acme.test")), fiber.StatusBadGateway},
	}
	for _, tc := range cases {
		var fe *fiber.Error
		if !errors.As(directApplyError(tc.err), &fe) {
			t.Fatalf("%v: not a fiber error", tc.err)
		}
		if fe.Code != tc.status {
			t.Errorf("%v: status = %d, want %d", tc.err, fe.Code, tc.status)
		}
		if strings.Contains(fe.Message, "acme.test") || strings.HasPrefix(fe.Message, "invalid application") {
			t.Errorf("%v: message %q leaks the cause or the sentinel prefix", tc.err, fe.Message)
		}
	}
}
