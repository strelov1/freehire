package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

// decodeBatch unmarshals the ingestion body into a generic shape so tests assert
// on the wire fields Langfuse expects, without pinning random ids/timestamps.
func decodeBatch(t *testing.T, body []byte) []map[string]any {
	t.Helper()
	var envelope struct {
		Batch []map[string]any `json:"batch"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		t.Fatalf("batch is not valid JSON: %v", err)
	}
	return envelope.Batch
}

// findEvent returns the first batch event of the given ingestion type.
func findEvent(t *testing.T, batch []map[string]any, typ string) map[string]any {
	t.Helper()
	for _, ev := range batch {
		if ev["type"] == typ {
			return ev
		}
	}
	t.Fatalf("no %q event in batch", typ)
	return nil
}

func TestEncodeBatch_successGeneration(t *testing.T) {
	start := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)
	g := Generation{
		Model:  "qwen2.5-72b",
		System: "system prompt",
		User:   "user prompt",
		Output: `{"seniority":"senior"}`,
		Usage:  &Usage{Input: 1200, Output: 40, Total: 1240},
		Start:  start,
		End:    start.Add(900 * time.Millisecond),
		Source: "enrich",
	}

	body, err := encodeBatch([]Generation{g})
	if err != nil {
		t.Fatalf("encodeBatch: %v", err)
	}
	batch := decodeBatch(t, body)

	gen := findEvent(t, batch, "generation-create")
	if gen["type"] != "generation-create" {
		t.Fatalf("expected generation-create event")
	}
	genBody, _ := gen["body"].(map[string]any)
	if genBody == nil {
		t.Fatal("generation event has no body object")
	}
	if genBody["model"] != "qwen2.5-72b" {
		t.Errorf("model = %v, want qwen2.5-72b", genBody["model"])
	}
	if genBody["level"] != "DEFAULT" {
		t.Errorf("level = %v, want DEFAULT for success", genBody["level"])
	}
	// input is a chat-message array so Langfuse renders a transcript.
	input, _ := genBody["input"].([]any)
	if len(input) != 2 {
		t.Fatalf("input = %v, want 2 chat messages", genBody["input"])
	}
	sys, _ := input[0].(map[string]any)
	usr, _ := input[1].(map[string]any)
	if sys["role"] != "system" || sys["content"] != "system prompt" {
		t.Errorf("input[0] = %v, want system message", input[0])
	}
	if usr["role"] != "user" || usr["content"] != "user prompt" {
		t.Errorf("input[1] = %v, want user message", input[1])
	}
	if genBody["output"] != `{"seniority":"senior"}` {
		t.Errorf("output = %v, want raw response", genBody["output"])
	}
	// usage tokens are present.
	usage, _ := genBody["usage"].(map[string]any)
	if usage == nil {
		t.Fatal("usage missing on a generation that reported tokens")
	}
	if usage["input"].(float64) != 1200 || usage["output"].(float64) != 40 || usage["total"].(float64) != 1240 {
		t.Errorf("usage = %v, want 1200/40/1240", usage)
	}
	// metadata attributes the workload.
	meta, _ := genBody["metadata"].(map[string]any)
	if meta["source"] != "enrich" {
		t.Errorf("metadata.source = %v, want enrich", meta["source"])
	}
	// a generation must belong to a trace.
	if genBody["traceId"] == nil || genBody["traceId"] == "" {
		t.Error("generation has no traceId")
	}
}

func TestEncodeBatch_errorGenerationOmitsUsage(t *testing.T) {
	start := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)
	g := Generation{
		Model:  "qwen2.5-72b",
		System: "system prompt",
		User:   "user prompt",
		Err:    errTest,
		Start:  start,
		End:    start.Add(50 * time.Millisecond),
		Source: "telegram",
	}

	body, err := encodeBatch([]Generation{g})
	if err != nil {
		t.Fatalf("encodeBatch: %v", err)
	}
	batch := decodeBatch(t, body)
	gen := findEvent(t, batch, "generation-create")
	genBody, _ := gen["body"].(map[string]any)

	if genBody["level"] != "ERROR" {
		t.Errorf("level = %v, want ERROR", genBody["level"])
	}
	if genBody["statusMessage"] != errTest.Error() {
		t.Errorf("statusMessage = %v, want %q", genBody["statusMessage"], errTest.Error())
	}
	// usage must be omitted when the model reported no tokens, not sent as zeros.
	if _, ok := genBody["usage"]; ok {
		t.Errorf("usage present on a call with no tokens, want omitted: %v", genBody["usage"])
	}
}

func TestSend_postsToIngestionWithAuth(t *testing.T) {
	var gotPath, gotUser, gotPass, gotCT string
	var gotBatchLen int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotUser, gotPass, _ = r.BasicAuth()
		gotCT = r.Header.Get("Content-Type")
		var env struct {
			Batch []json.RawMessage `json:"batch"`
		}
		json.NewDecoder(r.Body).Decode(&env)
		gotBatchLen = len(env.Batch)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	tr := newTestTracer(srv.URL, "pk-lf-x", "sk-lf-y")
	err := tr.send(context.Background(), []Generation{{Model: "m", Source: "enrich"}})
	if err != nil {
		t.Fatalf("send: %v", err)
	}

	if gotPath != "/api/public/ingestion" {
		t.Errorf("path = %q, want /api/public/ingestion", gotPath)
	}
	if gotUser != "pk-lf-x" || gotPass != "sk-lf-y" {
		t.Errorf("basic auth = %q:%q, want pk-lf-x:sk-lf-y", gotUser, gotPass)
	}
	if gotCT != "application/json" {
		t.Errorf("content-type = %q, want application/json", gotCT)
	}
	if gotBatchLen != 2 { // one generation → trace-create + generation-create
		t.Errorf("batch len = %d, want 2", gotBatchLen)
	}
}

func TestSend_207PartialErrorsAreLoggedNotFailed(t *testing.T) {
	// Langfuse returns 207 for per-event validation errors, hiding them in the
	// body — not the status. We must surface them (log) but not treat 207 as a
	// transport failure.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusMultiStatus)
		w.Write([]byte(`{"successes":[],"errors":[{"id":"abc","status":400,"message":"trace name too long"}]}`))
	}))
	defer srv.Close()

	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(os.Stderr)

	tr := newTestTracer(srv.URL, "pk", "sk")
	if err := tr.send(context.Background(), []Generation{{Source: "enrich"}}); err != nil {
		t.Errorf("207 must not be a transport error, got %v", err)
	}
	if !strings.Contains(buf.String(), "trace name too long") {
		t.Errorf("expected a warning naming the rejected event, got %q", buf.String())
	}
}

func TestSend_non2xxIsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	tr := newTestTracer(srv.URL, "pk", "sk")
	if err := tr.send(context.Background(), []Generation{{Source: "enrich"}}); err == nil {
		t.Error("send to a 401 endpoint returned nil, want error")
	}
}

func TestNewTracer_gatedOnConfig(t *testing.T) {
	if tr := NewTracer("", "pk", "sk"); tr != nil {
		t.Error("NewTracer with empty base returned non-nil, want nil (disabled)")
	}
	if tr := NewTracer("https://x", "pk", ""); tr != nil {
		t.Error("NewTracer with empty secret returned non-nil, want nil (disabled)")
	}
	tr := NewTracer("https://x", "pk", "sk")
	if tr == nil {
		t.Fatal("NewTracer with all three set returned nil, want a live tracer")
	}
	// Fully configured tracer must shut down cleanly.
	if err := tr.Shutdown(context.Background()); err != nil {
		t.Errorf("Shutdown: %v", err)
	}
}

func TestObserveAndShutdown_flushesBufferedGenerations(t *testing.T) {
	var gotGenerations int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var env struct {
			Batch []map[string]any `json:"batch"`
		}
		json.NewDecoder(r.Body).Decode(&env)
		for _, ev := range env.Batch {
			if ev["type"] == "generation-create" {
				gotGenerations++
			}
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	tr := NewTracer(srv.URL, "pk", "sk")
	for i := 0; i < 3; i++ {
		tr.Observe(Generation{Model: "m", Source: "enrich"})
	}
	if err := tr.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
	if gotGenerations != 3 {
		t.Errorf("server received %d generations, want 3 (Shutdown must flush)", gotGenerations)
	}
}

func TestShutdown_safeToCallTwice(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	tr := NewTracer(srv.URL, "pk", "sk")
	if err := tr.Shutdown(context.Background()); err != nil {
		t.Fatalf("first Shutdown: %v", err)
	}
	// A duplicate Shutdown must not panic (close of closed channel) — it returns.
	if err := tr.Shutdown(context.Background()); err != nil {
		t.Errorf("second Shutdown: %v", err)
	}
}

func TestObserve_bestEffortOnServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	tr := NewTracer(srv.URL, "pk", "sk")
	tr.Observe(Generation{Source: "enrich"})
	// A failing endpoint must not surface through Shutdown — tracing is best-effort.
	if err := tr.Shutdown(context.Background()); err != nil {
		t.Errorf("Shutdown returned %v on a failing server, want nil (error swallowed)", err)
	}
}

func TestObserve_doesNotBlockWhenBufferFull(t *testing.T) {
	// Server blocks forever, so the loop stalls on its first send and the buffer
	// fills. Observe must still return promptly (drop), never block the caller.
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-release
	}))
	defer srv.Close()

	tr := NewTracer(srv.URL, "pk", "sk")
	done := make(chan struct{})
	go func() {
		for i := 0; i < 10000; i++ {
			tr.Observe(Generation{Source: "enrich"})
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Observe blocked when buffer was full, want non-blocking drop")
	}

	// Release the server and drain the tracer so its background goroutine doesn't
	// outlive the test (a leaked goroutine writing to the global log races other tests).
	close(release)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	tr.Shutdown(ctx)
}

// newTestTracer builds a tracer pointing at a test server, with no async loop.
func newTestTracer(base, pub, sec string) *langfuseTracer {
	return &langfuseTracer{
		endpoint: base + ingestionPath,
		pub:      pub,
		sec:      sec,
		client:   &http.Client{Timeout: 5 * time.Second},
	}
}

var errTest = errTestType("boom")

type errTestType string

func (e errTestType) Error() string { return string(e) }
