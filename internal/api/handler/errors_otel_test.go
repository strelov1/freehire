package handler

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/strelov1/freehire/internal/platform/observability"
	collector "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	"google.golang.org/protobuf/proto"
)

// Missing Sentry hubs must not suppress unexpected errors or stream failures;
// expected client cancellation and 4xx must stay out of the exception stream.
func TestOTelExceptionsWithoutSentry(t *testing.T) {
	requests := make(chan *collector.ExportLogsServiceRequest, 4)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		request := new(collector.ExportLogsServiceRequest)
		if err := proto.Unmarshal(body, request); err != nil {
			t.Error(err)
		}
		requests <- request
		w.Header().Set("Content-Type", "application/x-protobuf")
	}))
	defer server.Close()
	t.Setenv("OTEL_EXPORTER_OTLP_LOGS_ENDPOINT", server.URL+"/v1/logs")
	shutdown, err := observability.InitLogs("freehire-api", "test")
	if err != nil {
		t.Fatal(err)
	}
	defer shutdown()
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	app.Use(recover.New(recover.Config{EnableStackTrace: true, StackTraceHandler: ReportPanic}))
	app.Get("/fault/:id", func(c *fiber.Ctx) error { return errors.New("unexpected database failure") })
	app.Get("/panic", func(c *fiber.Ctx) error { panic("unexpected request panic") })
	app.Get("/expected", func(c *fiber.Ctx) error { return fiber.ErrBadRequest })
	for _, path := range []string{"/fault/secret?token=private", "/panic", "/expected"} {
		response, err := app.Test(httptest.NewRequestWithContext(t.Context(), "GET", path, nil))
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
	}
	reportStreamFault(nil, errors.New("unexpected stream failure"))
	reportStreamFault(nil, context.Canceled)
	func() { defer recoverStream(nil, "test stream", nil); panic("stream panic") }()
	shutdown()
	select {
	case request := <-requests:
		records := request.ResourceLogs[0].ScopeLogs[0].LogRecords
		if len(records) != 4 {
			t.Fatalf("got %d exceptions, want handler, request panic, stream fault and stream panic", len(records))
		}
		for _, record := range records {
			attrs := map[string]string{}
			for _, attr := range record.Attributes {
				attrs[attr.Key] = attr.Value.GetStringValue()
			}
			if attrs["exception.message"] == "unexpected database failure" && attrs["http.route"] != "/fault/:id" {
				t.Errorf("route=%q, want registered pattern", attrs["http.route"])
			}
			if attrs["exception.message"] == "unexpected request panic" && attrs["error.source"] != "panic" {
				t.Error("panic missing error.source")
			}
			for key, value := range attrs {
				if strings.Contains(value, "token=private") || strings.Contains(value, "/fault/secret") {
					t.Errorf("private URL leaked in %s", key)
				}
			}
		}
	case <-time.After(3 * time.Second):
		t.Fatal("no exceptions delivered")
	}
}
