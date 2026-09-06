package handler

import (
	"context"
	"errors"

	sentryfiber "github.com/getsentry/sentry-go/fiber"
	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5"

	"github.com/strelov1/freehire/internal/application/inbox"
	"github.com/strelov1/freehire/internal/candidate/fitanalysis"
	"github.com/strelov1/freehire/internal/platform/pgerr"
	"github.com/strelov1/freehire/internal/search/search"
)

// statusClientClosedRequest is nginx's non-standard 499: the client went away
// before the handler finished. There is no one to receive the body, so it is not
// an application fault — we classify it away from the reported 500s.
const statusClientClosedRequest = 499

// LocalPanicReported is a c.Locals key set by the server's recover middleware when
// it unwinds a panic. The sentryfiber middleware has already captured that panic
// (with a stack) before re-raising it, and Fiber then re-delivers the recovered
// error to the ErrorHandler — so RenderError checks this marker to avoid reporting
// the same panic a second time (as a stackless 500).
const LocalPanicReported = "sentry_panic_reported"

type codedError struct {
	status        int
	code, message string
}

func (e *codedError) Error() string { return e.message }
func authError(status int, code, message string) error {
	return &codedError{status: status, code: code, message: message}
}

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
// Anything that ends as a 500 is reported to Sentry (via the request-scoped hub
// the sentryfiber middleware installs), whether the handler declared it or fell
// through to one. Routine 4xx, mapped 404s and the deployment-shaped 503s are
// deliberately not reported, so the error inbox reflects genuine faults rather
// than normal client traffic. When Sentry is disabled the hub is absent and the
// capture is skipped — panics are handled separately by the middleware itself.
func RenderError(c *fiber.Ctx, err error) error {
	var ce *codedError
	if errors.As(err, &ce) {
		return c.Status(ce.status).JSON(fiber.Map{"error": ce.message, "code": ce.code})
	}
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

// reportUnexpected sends err to Sentry as a genuine fault. It exists for call
// sites that build their own response — a redirect, typically — instead of
// returning through RenderError, but still want the same "only real faults
// reach the error inbox" gate RenderError applies to everything else.
func reportUnexpected(c *fiber.Ctx, err error) {
	if hub := sentryfiber.GetHubFromContext(c); hub != nil {
		hub.CaptureException(err)
	}
}

// classify maps an error to its HTTP status and message and reports whether it is
// a server fault worth sending to Sentry. The 404-mapped DB errors, a client
// disconnect and a malformed search query are all routine; every 500 is a fault,
// including one a handler DECLARED with fiber.NewError to give the caller a
// readable message.
//
// That last clause is the whole reason this branch tests the code instead of
// answering false for every *fiber.Error. Thirteen call sites wrapped a genuine
// failure — a DB read, a token mint, an autopilot run — in fiber.NewError(500,
// "…") purely to phrase it for a human, and thereby took it out of the error
// inbox; seven of them do not log either, so the failure left no trace anywhere.
// The streamed autopilot already decided the other way (see reportStreamFault in
// assistant.go), and the synchronous route sent the SAME error past it.
//
// It is an EQUALITY, not `code >= 500`. Forty-two sites answer 503 to mean "this
// deployment has no Meilisearch/LLM configured" — a statement about the
// environment, repeated on every request to that route, and not a fault anyone
// can act on from Sentry.
//
// A *fiber.Error cannot wrap, so a 500 declared this way reaches Sentry as its
// own message with no cause behind it. Call sites that have a cause worth keeping
// should therefore return `fmt.Errorf("…: %w", err)` and let the fall-through
// render the generic body, rather than phrasing the message and dropping the
// error.
func classify(err error) (status int, msg string, report bool) {
	var fe *fiber.Error
	var invalidValue *inbox.InvalidError
	var badBatch *inbox.BatchError
	switch {
	case errors.As(err, &fe):
		return fe.Code, fe.Message, fe.Code == fiber.StatusInternalServerError
	case errors.Is(err, pgx.ErrNoRows), pgerr.IsForeignKeyViolation(err), errors.Is(err, inbox.ErrNotFound):
		return fiber.StatusNotFound, "not found", false
	// The mail service validates against its own vocabularies and refuses to record
	// an application over a suggestion nobody has answered. Both readers of that
	// service need the same decision; only the rendering differs, and this is the
	// HTTP half of it.
	case errors.As(err, &invalidValue):
		return fiber.StatusBadRequest, invalidValue.Error(), false
	case errors.Is(err, inbox.ErrSlugRequired):
		return fiber.StatusBadRequest, err.Error(), false
	// A harness pushed a batch the mail service refuses: empty, over the ceiling, or missing
	// the deduplication key. A caller mistake, named so they can fix it.
	case errors.As(err, &badBatch):
		return fiber.StatusBadRequest, badBatch.Error(), false
	case errors.Is(err, inbox.ErrPendingSuggestion):
		return fiber.StatusConflict, err.Error(), false
	// The candidate has not run their fit analysis for this job, so the tailoring surfaces
	// have nothing to ground on. A state, not a fault — and one both readers of the fit
	// service meet, so the status is decided here rather than at each call site.
	case errors.Is(err, fitanalysis.ErrNoAnalysis):
		return fiber.StatusConflict, "run the fit analysis first", false
	// The client cancelled the request (navigated away, closed the tab). The
	// cancellation propagates through downstream calls (DB, Meilisearch) as
	// context.Canceled — not a server fault, and there is no one left to answer.
	// A server-side timeout (context.DeadlineExceeded) is deliberately NOT matched
	// here: that is our own deadline firing and is worth reporting.
	case errors.Is(err, context.Canceled):
		return statusClientClosedRequest, "client closed request", false
	// Meilisearch rejected the request as malformed (a 400) — e.g. an unparseable
	// filter value from client input. That is a bad request, not a fault.
	case errors.Is(err, search.ErrBadQuery):
		return fiber.StatusBadRequest, "invalid search query", false
	default:
		return fiber.StatusInternalServerError, "internal server error", true
	}
}
