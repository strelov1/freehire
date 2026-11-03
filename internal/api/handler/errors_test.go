package handler

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	collector "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	"google.golang.org/protobuf/proto"

	"github.com/strelov1/freehire/internal/platform/observability"
	"github.com/strelov1/freehire/internal/search/search"
)

// exceptionRecorder stands in for the OTLP collector the server exports exception logs
// to, so a test can count what was reported without any network beyond loopback. count
// shuts the exporter down first: the batch processor delivers asynchronously, and only
// its shutdown guarantees every captured record has arrived. Call it once, at the end.
type exceptionRecorder struct {
	mu       sync.Mutex
	records  int
	shutdown func()
}

func recordExceptions(t *testing.T) *exceptionRecorder {
	t.Helper()
	r := &exceptionRecorder{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		body, _ := io.ReadAll(req.Body)
		export := new(collector.ExportLogsServiceRequest)
		if err := proto.Unmarshal(body, export); err != nil {
			t.Error(err)
		}
		r.mu.Lock()
		for _, resource := range export.ResourceLogs {
			for _, scope := range resource.ScopeLogs {
				r.records += len(scope.LogRecords)
			}
		}
		r.mu.Unlock()
		w.Header().Set("Content-Type", "application/x-protobuf")
	}))
	t.Cleanup(server.Close)
	t.Setenv("OTEL_EXPORTER_OTLP_LOGS_ENDPOINT", server.URL+"/v1/logs")
	shutdown, err := observability.InitLogs("freehire-api")
	if err != nil {
		t.Fatalf("InitLogs: %v", err)
	}
	r.shutdown = shutdown
	t.Cleanup(shutdown)
	return r
}

func (r *exceptionRecorder) count() int {
	r.shutdown()
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.records
}

// reportingApp mirrors the server's middleware wiring (recover.New reporting panics
// through ReportPanic, RenderError as the ErrorHandler) so a test can assert how many
// exceptions are reported for a given failure.
func reportingApp(t *testing.T, errFn fiber.Handler) (*fiber.App, *exceptionRecorder) {
	t.Helper()
	rec := recordExceptions(t)
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	app.Use(recover.New(recover.Config{EnableStackTrace: true, StackTraceHandler: ReportPanic}))
	app.Get("/x", errFn)
	return app, rec
}

// A recovered panic is captured once by ReportPanic (with a stack); the
// error Fiber re-delivers to RenderError must NOT be reported again.
func TestRenderError_PanicReportedExactlyOnce(t *testing.T) {
	app, tr := reportingApp(t, func(*fiber.Ctx) error { panic("boom") })
	resp, err := app.Test(httptest.NewRequestWithContext(context.Background(), fiber.MethodGet, "/x", nil))
	if err != nil {
		t.Fatalf("Test: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusInternalServerError {
		t.Errorf("status = %d, want 500", resp.StatusCode)
	}
	if got := tr.count(); got != 1 {
		t.Errorf("reported exceptions = %d, want exactly 1", got)
	}
}

// A genuine (non-panic) 500 returned by a handler must still be reported once, so
// the dedup guard doesn't suppress real faults.
func TestRenderError_ReturnedErrorReportedOnce(t *testing.T) {
	app, tr := reportingApp(t, func(*fiber.Ctx) error { return errString("db exploded") })
	resp, err := app.Test(httptest.NewRequestWithContext(context.Background(), fiber.MethodGet, "/x", nil))
	if err != nil {
		t.Fatalf("Test: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusInternalServerError {
		t.Errorf("status = %d, want 500", resp.StatusCode)
	}
	if got := tr.count(); got != 1 {
		t.Errorf("reported exceptions = %d, want exactly 1", got)
	}
}

// A 500 a handler declared for itself — fiber.NewError(500, "…") to give the caller
// readable words instead of "internal server error" — must be reported like any
// other 500. It reached it never, which is how thirteen genuine faults (a failed
// token mint, an autopilot run, a billing event that could not be recorded) left no
// trace at all: seven of them do not log either.
func TestRenderError_DeclaredInternalErrorIsReported(t *testing.T) {
	app, tr := reportingApp(t, func(*fiber.Ctx) error {
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
		t.Errorf("reported exceptions = %d, want exactly 1", got)
	}
}

// The counterpart: a 503 says this deployment has no Meilisearch or LLM, which every
// request to that route repeats and nobody can act on from an error inbox. Reporting
// on `code >= 500` would have made ~42 call sites into a permanent alert.
func TestRenderError_UnconfiguredServiceIsNotReported(t *testing.T) {
	app, tr := reportingApp(t, func(*fiber.Ctx) error {
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
		t.Errorf("reported exceptions = %d, want 0", got)
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
// reporting it. Every 500 must be reported — including one a handler
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
		// repeated on every request to the route, and nobody can act on it from the error reports.
		{"503 means unconfigured, not broken", fiber.NewError(fiber.StatusServiceUnavailable, "search is not configured"), fiber.StatusServiceUnavailable, false},
		{"wrapped declared 500 is still a fault", fmt.Errorf("issue session: %w", fiber.NewError(fiber.StatusInternalServerError, "failed to start session")), fiber.StatusInternalServerError, true},
		{"no rows maps to 404", pgx.ErrNoRows, fiber.StatusNotFound, false},
		{"foreign-key violation maps to 404", &pgconn.PgError{Code: "23503"}, fiber.StatusNotFound, false},
		{"client disconnect is not a fault", context.Canceled, statusClientClosedRequest, false},
		{"wrapped client disconnect still 499", fmt.Errorf("search: query: %w", context.Canceled), statusClientClosedRequest, false},
		{"malformed search query maps to 400", fmt.Errorf("search: query: %w: bad filter", search.ErrBadQuery), fiber.StatusBadRequest, false},
		{"server-side timeout is still reported", context.DeadlineExceeded, fiber.StatusInternalServerError, true},
		// An autopilot run refused for want of a fit analysis has already said so to the
		// candidate, on the stream they were reading, in words they can act on. Filing it
		// as a fault put 12 copies of a refusal working exactly as designed into an inbox
		// whose monthly allowance the site had already spent — a state, like ErrNoAnalysis
		// it shares the condition with, not a fault.
		{"autopilot refused for want of a plan is a state", errNoRequirementList, fiber.StatusConflict, false},
		{"wrapped, it is still a state", fmt.Errorf("autopilot: %w", errNoRequirementList), fiber.StatusConflict, false},
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

// Too many OR alternatives is the caller's mistake, and the 400 says which limit
// they hit instead of the generic "invalid search query".
func TestClassifyNamesTheOrAlternativeLimit(t *testing.T) {
	status, msg, report := classify(fmt.Errorf("search: facet query: %w", search.ErrTooManyAlternatives))
	if status != fiber.StatusBadRequest || report {
		t.Fatalf("status = %d report = %v, want 400 and no report", status, report)
	}
	if msg != "q has more than 5 OR alternatives" {
		t.Fatalf("message = %q, want the limit named", msg)
	}
}
