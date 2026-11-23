package handler

import (
	"context"
	"errors"
	"strconv"
	"strings"

	"github.com/strelov1/freehire/internal/platform/observability"
	"go.opentelemetry.io/otel/attribute"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5"

	"github.com/strelov1/freehire/internal/application/inbox"
	"github.com/strelov1/freehire/internal/candidate/fitanalysis"
	"github.com/strelov1/freehire/internal/platform/pgerr"
	"github.com/strelov1/freehire/internal/search/search"
)

// statusClientClosedRequest is nginx's non-standard 499: the client left before the
// handler finished. Not an application fault, so it is not reported.
const statusClientClosedRequest = 499

// LocalPanicReported marks a panic captured by the recovery callback. Fiber re-delivers
// the recovered error to the ErrorHandler; RenderError checks this to report it once.
const LocalPanicReported = "panic_reported"

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
// Anything that ends as a 500 is reported to the exception log
// (observability.CaptureError), declared or fall-through. Routine 4xx, mapped 404s and
// deployment-shaped 503s are not, so the inbox holds genuine faults. Panics are reported
// by ReportPanic.
func RenderError(c *fiber.Ctx, err error) error {
	var ce *codedError
	if errors.As(err, &ce) {
		return c.Status(ce.status).JSON(fiber.Map{"error": ce.message, "code": ce.code})
	}
	status, msg, report := classify(err)
	if errors.Is(err, search.ErrBusy) {
		c.Set(fiber.HeaderRetryAfter, strconv.Itoa(int(search.BusyRetryAfter.Seconds())))
	}

	// A recovered panic was already captured with its stack by ReportPanic.
	if report && c.Locals(LocalPanicReported) == nil {
		reportUnexpected(c, err)
	}

	return c.Status(status).JSON(fiber.Map{"error": msg})
}

// reportUnexpected reports err as a genuine fault, for call sites that build their own
// response (a redirect, say) instead of returning through RenderError but want the same
// "only real faults" gate.
func reportUnexpected(c *fiber.Ctx, err error) {
	if _, _, report := classify(err); !report {
		return
	}
	observability.CaptureError(c.UserContext(), err, requestErrorAttributes(c)...)
}

// ReportPanic is Fiber's recovery callback. It records the panic with its stack,
// then marks it so RenderError cannot emit a second event.
func ReportPanic(c *fiber.Ctx, value any) {
	observability.CapturePanic(c.UserContext(), value, requestErrorAttributes(c)...)
	c.Locals(LocalPanicReported, true)
}

func requestErrorAttributes(c *fiber.Ctx) []attribute.KeyValue {
	route := "unmatched"
	if registered := c.Route(); len(registered.Handlers) != 0 {
		route = registered.Path
	}
	return []attribute.KeyValue{attribute.String("http.request.method", strings.Clone(c.Method())), attribute.String("http.route", strings.Clone(route))}
}

// classify maps an error to its HTTP status and message and reports whether it is
// a server fault worth reporting. The 404-mapped DB errors, a client disconnect and a
// malformed search query are routine; every 500 is a fault, including one a handler
// DECLARED with fiber.NewError to phrase a readable message. Hence the branch tests the
// code instead of answering false for every *fiber.Error.
//
// It is an EQUALITY, not `code >= 500`: 503 means "this deployment has no
// Meilisearch/LLM configured", an environment statement nobody can act on from reports.
//
// A *fiber.Error cannot wrap, so a declared 500 is reported with no cause. A call site
// with a cause worth keeping should return `fmt.Errorf("…: %w", err)` and let the
// fall-through render the generic body.
func classify(err error) (status int, msg string, report bool) {
	var fe *fiber.Error
	var invalidValue *inbox.InvalidError
	var badBatch *inbox.BatchError
	switch {
	case errors.As(err, &fe):
		return fe.Code, fe.Message, fe.Code == fiber.StatusInternalServerError
	case errors.Is(err, pgx.ErrNoRows), pgerr.IsForeignKeyViolation(err), errors.Is(err, inbox.ErrNotFound):
		return fiber.StatusNotFound, "not found", false
	// The mail service validates against its own vocabularies and refuses to record an
	// application over an unanswered suggestion. Both readers need the same decision; this
	// is the HTTP half.
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
	// No fit analysis for this job, so the tailoring surfaces have nothing to ground on.
	// A state, not a fault; the status is decided here for both readers of the fit service.
	//
	// errNoRequirementList is the same state from the other side: autopilot refused by
	// refuseAutopilotWithoutAPlan. It is listed HERE because both RenderError and
	// reportStreamFault ask only this function whether a failure is worth reporting, and
	// the refusal has already explained itself to the candidate.
	case errors.Is(err, fitanalysis.ErrNoAnalysis), errors.Is(err, errNoRequirementList):
		return fiber.StatusConflict, "run the fit analysis first", false
	// The client cancelled (navigated away, closed the tab); it propagates through DB and
	// Meilisearch calls as context.Canceled. Not a fault. context.DeadlineExceeded is
	// deliberately NOT matched: our own deadline firing is worth reporting.
	case errors.Is(err, context.Canceled):
		return statusClientClosedRequest, "client closed request", false
	// q carried more OR alternatives than search allows; name the limit.
	case errors.Is(err, search.ErrTooManyAlternatives):
		return fiber.StatusBadRequest, search.ErrTooManyAlternatives.Error(), false
	// Meilisearch rejected the request as malformed (400), e.g. an unparseable client
	// filter value: a bad request, not a fault.
	case errors.Is(err, search.ErrBadQuery):
		return fiber.StatusBadRequest, "invalid search query", false
	// Meilisearch's search queue was full: overload or a brief stall, not a code fault. The
	// caller is told when to retry (RenderError sets Retry-After).
	case errors.Is(err, search.ErrBusy):
		return fiber.StatusServiceUnavailable, "search is busy, try again shortly", false
	default:
		return fiber.StatusInternalServerError, "internal server error", true
	}
}
