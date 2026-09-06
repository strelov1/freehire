package handler

import (
	"context"
	"fmt"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/getsentry/sentry-go"
	sentryfiber "github.com/getsentry/sentry-go/fiber"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/strelov1/freehire/internal/search/search"
)

// recordingTransport captures the events a Sentry hub would deliver, so a test can
// count them without any network. Guarded by a mutex: SendEvent runs on Fiber's
// request goroutine while the test reads from its own.
type recordingTransport struct {
	mu     sync.Mutex
	events []*sentry.Event
}

func (t *recordingTransport) Configure(sentry.ClientOptions) {}
func (t *recordingTransport) SendEvent(e *sentry.Event) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.events = append(t.events, e)
}
func (t *recordingTransport) Flush(time.Duration) bool              { return true }
func (t *recordingTransport) FlushWithContext(context.Context) bool { return true }
func (t *recordingTransport) Close()                                {}

func (t *recordingTransport) count() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return len(t.events)
}

// sentryApp mirrors the server's middleware wiring (recover.New marking panics,
// then the sentryfiber request middleware, RenderError as the ErrorHandler) so a
// test can assert how many events reach Sentry for a given failure.
func sentryApp(t *testing.T, errFn fiber.Handler) (*fiber.App, *recordingTransport) {
	t.Helper()
	tr := &recordingTransport{}
	if err := sentry.Init(sentry.ClientOptions{
		Dsn:       "https://public@o0.ingest.sentry.io/0",
		Transport: tr,
	}); err != nil {
		t.Fatalf("sentry.Init: %v", err)
	}
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	app.Use(recover.New(recover.Config{
		EnableStackTrace:  true,
		StackTraceHandler: func(c *fiber.Ctx, _ any) { c.Locals(LocalPanicReported, true) },
	}))
	app.Use(sentryfiber.New(sentryfiber.Options{Repanic: true, WaitForDelivery: true}))
	app.Get("/x", errFn)
	return app, tr
}

// A recovered panic is captured once by the request middleware (with a stack); the
// error Fiber re-delivers to RenderError must NOT be reported again.
func TestRenderError_PanicReportedExactlyOnce(t *testing.T) {
	app, tr := sentryApp(t, func(*fiber.Ctx) error { panic("boom") })
	resp, err := app.Test(httptest.NewRequestWithContext(context.Background(), fiber.MethodGet, "/x", nil))
	if err != nil {
		t.Fatalf("Test: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusInternalServerError {
		t.Errorf("status = %d, want 500", resp.StatusCode)
	}
	if got := tr.count(); got != 1 {
		t.Errorf("sentry events = %d, want exactly 1", got)
	}
}

// A genuine (non-panic) 500 returned by a handler must still be reported once, so
// the dedup guard doesn't suppress real faults.
func TestRenderError_ReturnedErrorReportedOnce(t *testing.T) {
	app, tr := sentryApp(t, func(*fiber.Ctx) error { return errString("db exploded") })
	resp, err := app.Test(httptest.NewRequestWithContext(context.Background(), fiber.MethodGet, "/x", nil))
	if err != nil {
		t.Fatalf("Test: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusInternalServerError {
		t.Errorf("status = %d, want 500", resp.StatusCode)
	}
	if got := tr.count(); got != 1 {
		t.Errorf("sentry events = %d, want exactly 1", got)
	}
}

// A 500 a handler declared for itself — fiber.NewError(500, "…") to give the caller
// readable words instead of "internal server error" — must reach Sentry like any
// other 500. It reached it never, which is how thirteen genuine faults (a failed
// token mint, an autopilot run, a billing event that could not be recorded) left no
// trace at all: seven of them do not log either.
func TestRenderError_DeclaredInternalErrorIsReported(t *testing.T) {
	app, tr := sentryApp(t, func(*fiber.Ctx) error {
		return fiber.NewError(fiber.StatusInternalServerError, "could not record the event")
	})
	resp, err := app.Test(httptest.NewRequestWithContext(context.Background(), fiber.MethodGet, "/x", nil))
	if err != nil {
		t.Fatalf("Test: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusInternalServerError {
		t.Errorf("status = %d, want 500", resp.StatusCode)
	}
	if got := tr.count(); got != 1 {
		t.Errorf("sentry events = %d, want exactly 1", got)
	}
}

// The counterpart: a 503 says this deployment has no Meilisearch or LLM, which every
// request to that route repeats and nobody can act on from an error inbox. Reporting
// on `code >= 500` would have made ~42 call sites into a permanent alert.
func TestRenderError_UnconfiguredServiceIsNotReported(t *testing.T) {
	app, tr := sentryApp(t, func(*fiber.Ctx) error {
		return fiber.NewError(fiber.StatusServiceUnavailable, "search is not configured")
	})
	resp, err := app.Test(httptest.NewRequestWithContext(context.Background(), fiber.MethodGet, "/x", nil))
	if err != nil {
		t.Fatalf("Test: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", resp.StatusCode)
	}
	if got := tr.count(); got != 0 {
		t.Errorf("sentry events = %d, want 0", got)
	}
}

// errorApp mounts a route that returns errFn's error through RenderError, so the
// status mapping can be asserted end to end.
func errorApp(errFn func(*fiber.Ctx) error) *fiber.App {
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	app.Get("/x", errFn)
	return app
}

func errorStatus(t *testing.T, app *fiber.App) int {
	t.Helper()
	resp, err := app.Test(httptest.NewRequestWithContext(context.Background(), fiber.MethodGet, "/x", nil))
	if err != nil {
		t.Fatalf("Test: %v", err)
	}
	defer resp.Body.Close()
	return resp.StatusCode
}

// A write that references a missing parent row (e.g. applying to a non-existent
// job id) raises a foreign-key violation; the caller deserves 404, not 500.
func TestErrorHandler_MapsForeignKeyViolationTo404(t *testing.T) {
	app := errorApp(func(*fiber.Ctx) error { return &pgconn.PgError{Code: "23503"} })
	if got := errorStatus(t, app); got != fiber.StatusNotFound {
		t.Errorf("status = %d, want 404", got)
	}
}

func TestErrorHandler_MapsNoRowsTo404(t *testing.T) {
	app := errorApp(func(*fiber.Ctx) error { return pgx.ErrNoRows })
	if got := errorStatus(t, app); got != fiber.StatusNotFound {
		t.Errorf("status = %d, want 404", got)
	}
}

func TestErrorHandler_DefaultsTo500(t *testing.T) {
	app := errorApp(func(*fiber.Ctx) error { return errString("boom") })
	if got := errorStatus(t, app); got != fiber.StatusInternalServerError {
		t.Errorf("status = %d, want 500", got)
	}
}

// classify decides both the HTTP mapping and whether an error is a fault worth
// reporting to Sentry. Every 500 must be reported — including one a handler
// DECLARED with fiber.NewError to phrase it for a human, which used to be reported
// as nothing at all; routine 4xx, mapped 404s and the deployment-shaped 503s must
// not be, so the error inbox stays signal, not noise.
func TestClassify_ReportsOnlyUnexpected500(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantReport bool
	}{
		{"generic error is an unexpected 500", errString("boom"), fiber.StatusInternalServerError, true},
		{"fiber 4xx is routine", fiber.NewError(fiber.StatusBadRequest, "bad"), fiber.StatusBadRequest, false},
		// A handler that wrapped a real failure in fiber.NewError purely to give the
		// caller readable words thereby took it out of the error inbox. Thirteen call
		// sites did, and seven of them logged nothing either.
		{"declared 500 is still a fault", fiber.NewError(fiber.StatusInternalServerError, "could not record the event"), fiber.StatusInternalServerError, true},
		// The equality, not `>= 500`: ~42 sites answer 503 to say this deployment has no
		// Meilisearch or LLM configured. That is a statement about the environment,
		// repeated on every request to the route, and nobody can act on it from Sentry.
		{"503 means unconfigured, not broken", fiber.NewError(fiber.StatusServiceUnavailable, "search is not configured"), fiber.StatusServiceUnavailable, false},
		{"wrapped declared 500 is still a fault", fmt.Errorf("issue session: %w", fiber.NewError(fiber.StatusInternalServerError, "failed to start session")), fiber.StatusInternalServerError, true},
		{"no rows maps to 404", pgx.ErrNoRows, fiber.StatusNotFound, false},
		{"foreign-key violation maps to 404", &pgconn.PgError{Code: "23503"}, fiber.StatusNotFound, false},
		{"client disconnect is not a fault", context.Canceled, statusClientClosedRequest, false},
		{"wrapped client disconnect still 499", fmt.Errorf("search: query: %w", context.Canceled), statusClientClosedRequest, false},
		{"malformed search query maps to 400", fmt.Errorf("search: query: %w: bad filter", search.ErrBadQuery), fiber.StatusBadRequest, false},
		{"server-side timeout is still reported", context.DeadlineExceeded, fiber.StatusInternalServerError, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, _, report := classify(tt.err)
			if status != tt.wantStatus {
				t.Errorf("status = %d, want %d", status, tt.wantStatus)
			}
			if report != tt.wantReport {
				t.Errorf("report = %v, want %v", report, tt.wantReport)
			}
		})
	}
}

type errString string

func (e errString) Error() string { return string(e) }
