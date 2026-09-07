package handler

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/job/recentfeed"
)

// readSSEFrame reads one "event: ...\ndata: ...\n\n" frame from r, blocking until
// a full frame — terminated by a blank line — has arrived. It never waits for
// EOF, which this endpoint's indefinite stream never produces.
func readSSEFrame(t *testing.T, r *bufio.Reader) string {
	t.Helper()
	var sb strings.Builder
	for {
		line, err := r.ReadString('\n')
		sb.WriteString(line)
		if err != nil {
			t.Fatalf("read SSE frame: %v (partial: %q)", err, sb.String())
		}
		if line == "\n" {
			return sb.String()
		}
	}
}

// This endpoint's stream never closes on its own (see StreamRecentJobs), so it
// needs a real TCP connection to test — app.Test buffers the whole response
// before returning, which would simply time out against a stream with no EOF.
func TestRecentFeedStream_ReplaysBacklogThenLivePublish(t *testing.T) {
	b := recentfeed.NewBroadcaster(10)
	b.Publish(recentfeed.Entry{
		Kind: recentfeed.KindSingle, Title: "Senior Backend Engineer",
		CompanyName: "Acme", JobSlug: "acme-sbe",
	})

	h := newRecentFeedHandlers(b, newTestThrottler(t))
	h.pingInterval = 10 * time.Millisecond // detect the test's connection close quickly
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	app.Get("/api/v1/feed/recent", h.StreamRecentJobs)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	go func() { _ = app.Listener(ln) }()
	t.Cleanup(func() { _ = app.Shutdown() })

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		fmt.Sprintf("http://%s/api/v1/feed/recent", ln.Addr().String()), nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if ct := resp.Header.Get(fiber.HeaderContentType); ct != "text/event-stream" {
		t.Errorf("content-type = %q, want text/event-stream", ct)
	}

	reader := bufio.NewReader(resp.Body)

	backlog := readSSEFrame(t, reader)
	if !strings.Contains(backlog, "event: job") || !strings.Contains(backlog, "Senior Backend Engineer") {
		t.Fatalf("first frame = %q, want the backlog entry replayed immediately on connect", backlog)
	}

	// A client connecting must never wait on the next poll tick to see what was
	// already produced before it arrived — this is the whole point of the backlog.
	b.Publish(recentfeed.Entry{
		Kind: recentfeed.KindSingle, Title: "Staff Frontend Engineer",
		CompanyName: "Globex", JobSlug: "globex-sfe",
	})

	live := readSSEFrame(t, reader)
	if !strings.Contains(live, "event: job") || !strings.Contains(live, "Staff Frontend Engineer") {
		t.Fatalf("second frame = %q, want the newly published entry", live)
	}
}

func TestRecentFeedStream_NilBroadcasterIsServiceUnavailable(t *testing.T) {
	h := newRecentFeedHandlers(nil, newTestThrottler(t))
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	app.Get("/api/v1/feed/recent", h.StreamRecentJobs)

	req := httptest.NewRequest(fiber.MethodGet, "/api/v1/feed/recent", nil)
	resp, err := app.Test(req, 5000)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503 for a deployment with no broadcaster wired up", resp.StatusCode)
	}
}
