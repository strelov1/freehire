package handler

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/search/embedprogress"
)

// fakeProgress is a progress source the test drives by hand.
type fakeProgress struct {
	mu      sync.Mutex
	current embedprogress.Progress
	subs    []chan embedprogress.Progress
}

func (f *fakeProgress) Current() embedprogress.Progress {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.current
}

func (f *fakeProgress) Subscribe() (embedprogress.Progress, <-chan embedprogress.Progress, func()) {
	f.mu.Lock()
	defer f.mu.Unlock()
	ch := make(chan embedprogress.Progress, 4)
	f.subs = append(f.subs, ch)
	return f.current, ch, func() {}
}

func (f *fakeProgress) publish(p embedprogress.Progress) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.current = p
	for _, ch := range f.subs {
		ch <- p
	}
}

func queuedProgress(n int64) embedprogress.Progress {
	at := time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)
	return embedprogress.Build(nil, &embedprogress.Sample{Depth: n, At: at}, nil)
}

func TestEmbedProgress_JSON(t *testing.T) {
	src := &fakeProgress{current: queuedProgress(123)}
	h := newEmbedProgressHandlers(src)
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	app.Get("/api/v1/stats/embeddings", newContract(contractHandlers{embedProgressHandlers: h}))

	status, body := doGet(t, app, "/api/v1/stats/embeddings")
	if status != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	data, ok := body["data"].(map[string]any)
	if !ok {
		t.Fatalf("body = %v, want {\"data\": ...}", body)
	}
	if data["queued"] != float64(123) {
		t.Errorf("queued = %v, want 123", data["queued"])
	}
	// No snapshot published yet: unmeasured, not zero.
	if v, present := data["eligible"]; !present || v != nil {
		t.Errorf("eligible = %v (present %v), want null", v, present)
	}
}

// The stream never ends on its own, so it needs a real listener (see the recent-feed
// stream test for why app.Test cannot read it).
func TestEmbedProgress_StreamSendsCurrentThenEachSample(t *testing.T) {
	src := &fakeProgress{current: queuedProgress(500)}
	h := newEmbedProgressHandlers(src)
	h.pingInterval = 10 * time.Millisecond
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	app.Get("/api/v1/stats/embeddings/stream", h.Stream)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ln, err := (&net.ListenConfig{}).Listen(ctx, "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	go func() { _ = app.Listener(ln) }()
	t.Cleanup(func() { _ = app.Shutdown() })

	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		fmt.Sprintf("http://%s/api/v1/stats/embeddings/stream", ln.Addr().String()), nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()
	if ct := resp.Header.Get(fiber.HeaderContentType); ct != "text/event-stream" {
		t.Errorf("content-type = %q, want text/event-stream", ct)
	}
	reader := bufio.NewReader(resp.Body)

	first := readSSEFrame(t, reader)
	if !strings.Contains(first, "event: progress") || !strings.Contains(first, `"queued":500`) {
		t.Fatalf("first frame = %q, want the current progress at once", first)
	}

	src.publish(queuedProgress(480))
	for {
		frame := readSSEFrame(t, reader)
		if strings.Contains(frame, "event: ping") {
			continue
		}
		if !strings.Contains(frame, "event: progress") || !strings.Contains(frame, `"queued":480`) {
			t.Fatalf("next frame = %q, want the new sample", frame)
		}
		break
	}
}

// Before the sampler's first reading the document is all null; sending it would wipe
// the figures the page already rendered on the server.
func TestEmbedProgress_StreamSkipsAnUnsampledDocument(t *testing.T) {
	src := &fakeProgress{} // nothing sampled yet
	h := newEmbedProgressHandlers(src)
	h.pingInterval = 10 * time.Millisecond
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	app.Get("/api/v1/stats/embeddings/stream", h.Stream)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ln, err := (&net.ListenConfig{}).Listen(ctx, "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	go func() { _ = app.Listener(ln) }()
	t.Cleanup(func() { _ = app.Shutdown() })

	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		fmt.Sprintf("http://%s/api/v1/stats/embeddings/stream", ln.Addr().String()), nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	if first := readSSEFrame(t, bufio.NewReader(resp.Body)); !strings.Contains(first, "event: ping") {
		t.Fatalf("first frame = %q, want a ping, not an empty progress document", first)
	}
}
