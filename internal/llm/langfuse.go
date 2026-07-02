package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"
)

// Tracer observes LLM generations. A nil Tracer is a valid no-op: callers guard
// their calls with a nil check, so an unconfigured worker holds a nil Tracer and
// performs no tracing work.
//
// Lifecycle contract (satisfied by the run-once workers): all Observe calls
// happen-before Shutdown — the worker drains its LLM work, then defers a single
// Shutdown. Observe after Shutdown is not supported; Shutdown itself is safe to
// call more than once.
type Tracer interface {
	// Observe records one generation. It never blocks the caller and never fails:
	// delivery is best-effort and asynchronous.
	Observe(Generation)
	// Shutdown flushes buffered generations, returning when the buffer is drained
	// or ctx is done. Send failures are swallowed; only ctx expiry is returned.
	// Safe to call multiple times.
	Shutdown(context.Context) error
}

const (
	// ingestionPath is the Langfuse batch ingestion endpoint, appended to the base URL.
	ingestionPath = "/api/public/ingestion"
	// bufferSize bounds the in-memory queue; a full buffer drops rather than blocks.
	bufferSize = 512
	// maxBatch caps how many generations one ingestion POST carries.
	maxBatch = 16
	// sendTimeout bounds a single ingestion POST.
	sendTimeout = 10 * time.Second
)

// langfuseTracer reports generations to Langfuse Cloud over the ingestion API. It
// buffers on a channel and flushes from a single background goroutine.
type langfuseTracer struct {
	endpoint  string
	pub, sec  string
	client    *http.Client
	ch        chan Generation
	done      chan struct{}
	closeOnce sync.Once
}

// NewTracer builds a Langfuse tracer and starts its background sender. It returns
// nil (a valid no-op Tracer) unless base URL, public key, and secret key are all
// set — so a missing configuration disables tracing without any caller change.
func NewTracer(baseURL, publicKey, secretKey string) Tracer {
	if baseURL == "" || publicKey == "" || secretKey == "" {
		return nil
	}
	t := &langfuseTracer{
		endpoint: baseURL + ingestionPath,
		pub:      publicKey,
		sec:      secretKey,
		client:   &http.Client{Timeout: sendTimeout},
		ch:       make(chan Generation, bufferSize),
		done:     make(chan struct{}),
	}
	go t.loop()
	return t
}

// Observe enqueues a generation without blocking; a full buffer drops it with a
// warning rather than stalling the LLM call.
func (t *langfuseTracer) Observe(g Generation) {
	select {
	case t.ch <- g:
	default:
		log.Printf("langfuse: buffer full, dropping generation")
	}
}

// loop drains the buffer, batching sends and flushing whatever remains when the
// channel closes. Send failures are logged and dropped — tracing is best-effort.
func (t *langfuseTracer) loop() {
	defer close(t.done)
	batch := make([]Generation, 0, maxBatch)
	flush := func() {
		if len(batch) == 0 {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), sendTimeout)
		if err := t.send(ctx, batch); err != nil {
			log.Printf("langfuse: send failed, dropping %d generations: %v", len(batch), err)
		}
		cancel()
		batch = batch[:0]
	}
	for g := range t.ch {
		batch = append(batch, g)
		if len(batch) >= maxBatch {
			flush()
		}
	}
	flush()
}

// Shutdown stops accepting generations and waits for the buffer to flush, or for
// ctx to expire. It returns only ctx errors; send failures are already swallowed.
// The channel close is guarded by a sync.Once so a duplicate Shutdown is a safe
// wait rather than a close-of-closed-channel panic.
func (t *langfuseTracer) Shutdown(ctx context.Context) error {
	t.closeOnce.Do(func() { close(t.ch) })
	select {
	case <-t.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// send encodes the generations and POSTs them to the ingestion endpoint under
// HTTP Basic auth. A non-2xx response or transport error is returned to the
// caller, which decides how to handle it (the async loop logs and drops).
func (t *langfuseTracer) send(ctx context.Context, gens []Generation) error {
	body, err := encodeBatch(gens)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.SetBasicAuth(t.pub, t.sec)
	req.Header.Set("Content-Type", "application/json")

	resp, err := t.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("langfuse: ingestion returned %s", resp.Status)
	}
	// Ingestion answers 207 with per-event validation errors in the body, not the
	// status. Surface them (best-effort log) so silently dropped events are visible.
	if resp.StatusCode == http.StatusMultiStatus {
		logIngestionErrors(resp.Body)
	}
	return nil
}

// logIngestionErrors logs any per-event errors carried in a 207 response body.
func logIngestionErrors(body io.Reader) {
	var r struct {
		Errors []struct {
			ID      string `json:"id"`
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.NewDecoder(body).Decode(&r); err != nil || len(r.Errors) == 0 {
		return
	}
	log.Printf("langfuse: %d event(s) rejected by ingestion, first: %s (id %s)",
		len(r.Errors), r.Errors[0].Message, r.Errors[0].ID)
}

// Generation is one observed LLM call, handed to a Tracer. A non-nil Err records
// the call at error level; a nil Usage means the provider reported no token counts
// (recorded as absent rather than zero). Source labels the originating workload
// (e.g. "enrich" / "telegram").
type Generation struct {
	Model  string
	System string
	User   string
	Output string
	Usage  *Usage
	Start  time.Time
	End    time.Time
	Err    error
	Source string
}

// Usage is the token accounting for a single call.
type Usage struct {
	Input  int
	Output int
	Total  int
}

// encodeBatch renders generations as a Langfuse ingestion request body. Each
// generation becomes a trace plus a generation observation sharing one trace id,
// so the call is filterable by workload and carries its prompt/response/usage.
func encodeBatch(gens []Generation) ([]byte, error) {
	events := make([]ingestionEvent, 0, len(gens)*2)
	for _, g := range gens {
		traceID := uuid.NewString()
		now := rfc3339(time.Now())

		events = append(events, ingestionEvent{
			ID:        uuid.NewString(),
			Type:      "trace-create",
			Timestamp: now,
			Body: traceBody{
				ID:        traceID,
				Name:      g.Source,
				Timestamp: rfc3339(g.Start),
			},
		})
		events = append(events, ingestionEvent{
			ID:        uuid.NewString(),
			Type:      "generation-create",
			Timestamp: now,
			Body:      newGenerationBody(g, traceID),
		})
	}
	return json.Marshal(ingestionRequest{Batch: events})
}

// rfc3339 formats a time as the UTC RFC3339 timestamp Langfuse expects.
func rfc3339(t time.Time) string { return t.UTC().Format(time.RFC3339Nano) }

// newGenerationBody maps a Generation onto the Langfuse generation observation.
func newGenerationBody(g Generation, traceID string) generationBody {
	b := generationBody{
		ID:        uuid.NewString(),
		TraceID:   traceID,
		Name:      g.Source,
		StartTime: rfc3339(g.Start),
		EndTime:   rfc3339(g.End),
		Model:     g.Model,
		Input:     []chatMessage{{Role: "system", Content: g.System}, {Role: "user", Content: g.User}},
		Output:    g.Output,
		Level:     "DEFAULT",
		Metadata:  map[string]string{"source": g.Source},
	}
	if g.Usage != nil {
		b.Usage = &usageBody{
			Input:  g.Usage.Input,
			Output: g.Usage.Output,
			Total:  g.Usage.Total,
			Unit:   "TOKENS",
		}
	}
	if g.Err != nil {
		b.Level = "ERROR"
		b.StatusMessage = g.Err.Error()
	}
	return b
}

// Langfuse ingestion wire types.

type ingestionRequest struct {
	Batch []ingestionEvent `json:"batch"`
}

type ingestionEvent struct {
	ID        string `json:"id"`
	Type      string `json:"type"`
	Timestamp string `json:"timestamp"`
	Body      any    `json:"body"`
}

type traceBody struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Timestamp string `json:"timestamp"`
}

type generationBody struct {
	ID            string            `json:"id"`
	TraceID       string            `json:"traceId"`
	Name          string            `json:"name"`
	StartTime     string            `json:"startTime"`
	EndTime       string            `json:"endTime"`
	Model         string            `json:"model"`
	Input         []chatMessage     `json:"input"`
	Output        string            `json:"output"`
	Usage         *usageBody        `json:"usage,omitempty"`
	Level         string            `json:"level"`
	StatusMessage string            `json:"statusMessage,omitempty"`
	Metadata      map[string]string `json:"metadata"`
}

// chatMessage is one entry of the chat-transcript input Langfuse renders.
type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type usageBody struct {
	Input  int    `json:"input"`
	Output int    `json:"output"`
	Total  int    `json:"total"`
	Unit   string `json:"unit"`
}
