package observability

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	collector "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	"google.golang.org/protobuf/proto"
)

// Removing exception emission or shutting down an inner Bootstrap owner must lose
// a real OTLP event here, rather than merely changing a mock's call count.
func TestLogsExportExceptionsAndKeepOuterOwnerAlive(t *testing.T) {
	requests := make(chan *collector.ExportLogsServiceRequest, 4)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/logs" || r.Header.Get("Content-Type") != "application/x-protobuf" {
			t.Errorf("unexpected OTLP request: %s %s", r.URL.Path, r.Header.Get("Content-Type"))
		}
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
	t.Setenv("OTEL_SERVICE_NAME", "custom-worker")
	t.Setenv("OTEL_RESOURCE_ATTRIBUTES", "service.version=release-42,deployment.environment.name=test")
	shutdown, err := InitLogs("default-worker", "fallback")
	if err != nil {
		t.Fatal(err)
	}
	defer shutdown()
	inner, err := InitLogs("inner", "fallback")
	if err != nil {
		t.Fatal(err)
	}
	inner()
	CaptureError(context.Background(), errors.New("database unavailable"))
	CapturePanic(context.Background(), "worker panic")
	shutdown()
	var request *collector.ExportLogsServiceRequest
	select {
	case request = <-requests:
	case <-time.After(3 * time.Second):
		t.Fatal("no OTLP exceptions delivered during shutdown")
	}
	resource := request.ResourceLogs[0]
	attrs := map[string]string{}
	for _, attr := range resource.Resource.Attributes {
		attrs[attr.Key] = attr.Value.GetStringValue()
	}
	for key, want := range map[string]string{"service.name": "custom-worker", "service.version": "release-42", "deployment.environment.name": "test"} {
		if attrs[key] != want {
			t.Errorf("%s=%q, want %q", key, attrs[key], want)
		}
	}
	records := resource.ScopeLogs[0].LogRecords
	if len(records) != 2 {
		t.Fatalf("got %d exceptions, want 2", len(records))
	}
	for i, record := range records {
		attrs := map[string]string{}
		for _, attr := range record.Attributes {
			attrs[attr.Key] = attr.Value.GetStringValue()
		}
		if attrs["event.name"] != "exception" || attrs["exception.type"] == "" || attrs["exception.message"] == "" || !strings.Contains(attrs["exception.stacktrace"], "TestLogsExportExceptions") {
			t.Errorf("incomplete exception: %v", attrs)
		}
		if record.SeverityNumber < 17 {
			t.Errorf("exception %d has severity %v", i, record.SeverityNumber)
		}
	}
}

func TestLogsRedactsSecretsAndDoesNotSerializePanicObjects(t *testing.T) {
	requests := make(chan *collector.ExportLogsServiceRequest, 1)
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
	shutdown, err := InitLogs("test", "test")
	if err != nil {
		t.Fatal(err)
	}
	defer shutdown()
	failure := &url.Error{Op: "Post", URL: "https://user:private-password@example.com/path?token=query-secret#fragment-secret", Err: errors.New("Bearer bearer-secret token=field-secret password:password-secret contact=user@example.com postgres://db-user:db-password@db.internal/db?credential=dsn-secret access_token=access-private refresh_token:refresh-private client_secret=client-private \"token\":\"json-private\" authorization: Basic dXNlcjpwYXNz")}
	CaptureError(context.Background(), fmt.Errorf("upstream: %w", failure))
	CapturePanic(context.Background(), map[string]string{"candidate": "private-panic-payload"})
	shutdown()
	select {
	case request := <-requests:
		payload := request.String()
		for _, secret := range []string{"private-password", "query-secret", "fragment-secret", "bearer-secret", "field-secret", "password-secret", "user@example.com", "db-user", "db-password", "dsn-secret", "private-panic-payload", "access-private", "refresh-private", "client-private", "json-private", "dXNlcjpwYXNz"} {
			if strings.Contains(payload, secret) {
				t.Errorf("telemetry contains %q", secret)
			}
		}
		if !strings.Contains(payload, "https://example.com/path") {
			t.Error("missing useful upstream URL path")
		}
		if !strings.Contains(payload, "postgres://db.internal/db") {
			t.Error("missing useful database URL path")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("no exceptions delivered")
	}
}

func TestLogsDisabledWithoutEndpoint(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_LOGS_ENDPOINT", "")
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "")
	shutdown, err := InitLogs("test", "test")
	if err != nil {
		t.Fatal(err)
	}
	CaptureError(context.Background(), errors.New("disabled"))
	shutdown()
}

// A stalled collector must never turn exception capture into request latency or
// keep a short-lived worker alive beyond its shutdown budget.
func TestLogsCollectorFailureIsBounded(t *testing.T) {
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { <-release }))
	defer server.Close()
	defer close(release)
	t.Setenv("OTEL_EXPORTER_OTLP_LOGS_ENDPOINT", server.URL+"/v1/logs")
	shutdown, err := InitLogs("test", "test")
	if err != nil {
		t.Fatal(err)
	}
	defer shutdown()
	started := time.Now()
	for range 300 {
		CaptureError(context.Background(), errors.New("collector unavailable"))
	}
	if elapsed := time.Since(started); elapsed > 500*time.Millisecond {
		t.Fatalf("exception capture blocked for %s", elapsed)
	}
	started = time.Now()
	shutdown()
	if elapsed := time.Since(started); elapsed > 2500*time.Millisecond {
		t.Fatalf("shutdown exceeded budget: %s", elapsed)
	}
}
