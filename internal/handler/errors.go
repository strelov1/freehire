package handler

import (
	"errors"

	sentryfiber "github.com/getsentry/sentry-go/fiber"
	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5"

	"github.com/strelov1/freehire/internal/pgerr"
)

// LocalPanicReported is a c.Locals key set by the server's recover middleware when
// it unwinds a panic. The sentryfiber middleware has already captured that panic
// (with a stack) before re-raising it, and Fiber then re-delivers the recovered
// error to the ErrorHandler — so RenderError checks this marker to avoid reporting
// the same panic a second time (as a stackless 500).
const LocalPanicReported = "sentry_panic_reported"

// RenderError is the single place every error returned by a handler becomes an
// HTTP response. It is wired into fiber.New so the error envelope (`{"error":
// ...}`, mirroring the `{"data": ...}` success shape) and the status mapping
// live in one place instead of being hand-rolled per handler:
//
//   - a *fiber.Error (from fiber.NewError) keeps its code and message — this is
//     how handlers declare a specific HTTP meaning (e.g. 400 "invalid job id");
//   - a not-found from the DB layer (pgx.ErrNoRows) maps to 404, so read
//     handlers can just `return err`;
//   - a foreign-key violation (a write referencing a missing parent row, e.g.
//     applying to a non-existent job id) also maps to 404 — the referenced
//     resource doesn't exist;
//   - anything else is an unexpected failure: 500 with a generic message, never
//     leaking internals.
//
// Only that last, unexpected case is reported to Sentry (via the request-scoped
// hub the sentryfiber middleware installs). Routine 4xx and mapped 404s are
// deliberately not reported, so the error inbox reflects genuine faults rather
// than normal client traffic. When Sentry is disabled the hub is absent and the
// capture is skipped — panics are handled separately by the middleware itself.
func RenderError(c *fiber.Ctx, err error) error {
	status, msg, report := classify(err)

	// Report only genuine, not-yet-reported faults. A recovered panic is already
	// captured (with a stack) by the sentryfiber middleware, which flags it via
	// LocalPanicReported; reporting the re-delivered error again would duplicate it.
	if report && c.Locals(LocalPanicReported) == nil {
		if hub := sentryfiber.GetHubFromContext(c); hub != nil {
			hub.CaptureException(err)
		}
	}

	return c.Status(status).JSON(fiber.Map{"error": msg})
}

// classify maps an error to its HTTP status and message and reports whether it is
// an unexpected fault worth sending to Sentry. Only the fall-through 500 is
// unexpected; a *fiber.Error and the 404-mapped DB errors are routine.
func classify(err error) (status int, msg string, report bool) {
	var fe *fiber.Error
	switch {
	case errors.As(err, &fe):
		return fe.Code, fe.Message, false
	case errors.Is(err, pgx.ErrNoRows), pgerr.IsForeignKeyViolation(err):
		return fiber.StatusNotFound, "not found", false
	default:
		return fiber.StatusInternalServerError, "internal server error", true
	}
}
