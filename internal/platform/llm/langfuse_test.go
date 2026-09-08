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
	// usage tokens are present, as Langfuse's exclusive buckets. No total is sent —
	// Langfuse derives it, and a total of our own could disagree with its own parts.
	usage, _ := genBody["usageDetails"].(map[string]any)
	if usage == nil {
		t.Fatal("usageDetails missing on a generation that reported tokens")
	}
	if usage["input"].(float64) != 1200 || usage["output"].(float64) != 40 {
		t.Errorf("usageDetails = %v, want input 1200 and output 40", usage)
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

// Langfuse counts every key of usageDetails as its own non-overlapping bucket, so the
// plain input bucket must EXCLUDE what the cache served. The provider reports the two
// overlapping (prompt_tokens includes the cached ones), and sending them through
// unadjusted would bill the cached prefix twice — at the full input rate as well as the
// cache-read one, which is the direction that makes a model look more expensive than it
// is.
func TestEncodeBatchSplitsCachedTokensIntoTheirOwnBucket(t *testing.T) {
	start := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)
	g := Generation{
		Model:  "deepseek-v4-flash",
		System: "system prompt",
		User:   "user prompt",
		Output: `{"ok":true}`,
		Usage:  &Usage{Input: 1200, Output: 40, CachedInput: 900, Total: 1240},
		Start:  start,
		End:    start.Add(time.Second),
		Source: "assistant",
	}

	batch := decodeBatch(t, mustEncode(t, g))
	details := usageDetailsOf(t, batch)

	if details["input"] != float64(300) {
		t.Errorf("input bucket = %v, want 300 (1200 reported minus 900 cached)", details["input"])
	}
	if details["input_cached_tokens"] != float64(900) {
		t.Errorf("input_cached_tokens = %v, want 900", details["input_cached_tokens"])
	}
	if details["output"] != float64(40) {
		t.Errorf("output = %v, want 40", details["output"])
	}
}

// A provider that named no cached count leaves the whole prompt in the plain bucket, and
// no cached bucket is sent at all — an explicit zero would assert a cache miss we did not
// measure.
func TestEncodeBatchOmitsTheCachedBucketWhenNoneWasReported(t *testing.T) {
	start := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)
	g := Generation{
		Model:  "m",
		Output: "{}",
		Usage:  &Usage{Input: 1200, Output: 40, Total: 1240},
		Start:  start,
		End:    start.Add(time.Second),
		Source: "enrich",
	}

	details := usageDetailsOf(t, decodeBatch(t, mustEncode(t, g)))
	if details["input"] != float64(1200) {
		t.Errorf("input bucket = %v, want the full 1200 when nothing was cached", details["input"])
	}
	if _, ok := details["input_cached_tokens"]; ok {
		t.Errorf("cached bucket present as %v, want it absent", details["input_cached_tokens"])
	}
}

// A cached count that exceeds the reported prompt total is not arithmetic we can trust.
// Subtracting would produce a negative bucket, which Langfuse would take at face value,
// so the reading is passed through whole and unsplit rather than turned into a number
// nobody measured.
func TestEncodeBatchLeavesTheInputWholeWhenCachedExceedsIt(t *testing.T) {
	start := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)
	g := Generation{
		Model:  "m",
		Output: "{}",
		Usage:  &Usage{Input: 100, Output: 5, CachedInput: 900, Total: 105},
		Start:  start,
		End:    start.Add(time.Second),
		Source: "enrich",
	}

	details := usageDetailsOf(t, decodeBatch(t, mustEncode(t, g)))
	if details["input"] != float64(100) {
		t.Errorf("input bucket = %v, want the reported 100 left whole", details["input"])
	}
	if _, ok := details["input_cached_tokens"]; ok {
		t.Errorf("cached bucket present as %v, want it absent on an impossible reading", details["input_cached_tokens"])
	}
}

func mustEncode(t *testing.T, g Generation) []byte {
	t.Helper()
	body, err := encodeBatch([]Generation{g})
	if err != nil {
		t.Fatalf("encodeBatch: %v", err)
	}
	return body
}

func usageDetailsOf(t *testing.T, batch []map[string]any) map[string]any {
	t.Helper()
	genBody, _ := findEvent(t, batch, "generation-create")["body"].(map[string]any)
	if genBody == nil {
		t.Fatal("generation event has no body object")
	}
	details, _ := genBody["usageDetails"].(map[string]any)
	if details == nil {
		t.Fatalf("generation carries no usageDetails: %v", genBody)
	}
	return details
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
	if _, ok := genBody["usageDetails"]; ok {
		t.Errorf("usageDetails present on a call with no tokens, want omitted: %v", genBody["usageDetails"])
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
		_ = json.NewDecoder(r.Body).Decode(&env)
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
		_, _ = w.Write([]byte(`{"successes":[],"errors":[{"id":"abc","status":400,"message":"trace name too long"}]}`))
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
		_ = json.NewDecoder(r.Body).Decode(&env)
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
	_ = tr.Shutdown(ctx)
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
