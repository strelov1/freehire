package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	collector "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	"google.golang.org/protobuf/proto"
)

// A startup refusal must report and flush before returning a nonzero status;
// log.Fatal would bypass every deferred reporting cleanup.
func TestRunMainReportsStartupFailure(t *testing.T) {
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
	t.Setenv("JWT_SECRET", "")
	if code := runMain(); code != 1 {
		t.Fatalf("exit=%d, want 1", code)
	}
	select {
	case request := <-requests:
		records := request.ResourceLogs[0].ScopeLogs[0].LogRecords
		if len(records) != 1 || !strings.Contains(records[0].Body.GetStringValue(), "JWT_SECRET") {
			t.Fatalf("missing startup failure: %v", records)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("startup failure was not flushed")
	}
}
