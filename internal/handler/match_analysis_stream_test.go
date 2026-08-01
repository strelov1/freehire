package handler

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/getsentry/sentry-go"
)

// stalledConn models the socket of a reader that stopped reading: writes block until the
// write deadline the caller set, then fail the way a real one does. With no deadline set
// it blocks for far longer than any test would wait — which is precisely the production
// hazard, since SetWriteDeadline(time.Time{}) clears the deadline for good.
type stalledConn struct {
	net.Conn
	mu        sync.Mutex
	deadline  time.Time
	deadlines int
}

func (c *stalledConn) SetWriteDeadline(t time.Time) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.deadline = t
	c.deadlines++
	return nil
}

func (c *stalledConn) Write(p []byte) (int, error) {
	c.mu.Lock()
	d := c.deadline
	c.mu.Unlock()
	if d.IsZero() {
		time.Sleep(time.Minute) // unbounded in production; the test must never reach this
		return len(p), nil
	}
	if wait := time.Until(d); wait > 0 {
		time.Sleep(wait)
	}
	return 0, os.ErrDeadlineExceeded
}

func (c *stalledConn) deadlinesSet() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.deadlines
}

// The stream clears the connection's write deadline so the server's 10s WriteTimeout can't
// kill a long analysis — but an unbounded write lets a reader that went away block Flush
// forever, stranding the analysis goroutine holding the lock. Every write must therefore
// carry its own deadline, refreshed per write so a slow-but-alive reader is never cut off.
func TestSSEStream_BoundsEveryWriteSoADeadReaderCannotStrandTheStream(t *testing.T) {
	conn := &stalledConn{}
	s := newSSEStream(bufio.NewWriter(conn), conn, 50*time.Millisecond)

	done := make(chan struct{})
	go func() {
		defer close(done)
		s.event("meta", map[string]bool{"has_cv": true})
		s.event("stage_start", map[string]int{"stage": 1})
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("writes did not return; a dead reader can still strand the stream")
	}
	if got := conn.deadlinesSet(); got < 2 {
		t.Errorf("write deadlines set = %d, want one per write (>=2) so a slow reader is not cut off", got)
	}
}

// Without a connection (the handler captures a nil conn) the stream must still write —
// the deadline is simply not settable, exactly as before.
func TestSSEStream_NilConnStillWrites(t *testing.T) {
	var buf bytes.Buffer
	s := newSSEStream(bufio.NewWriter(&buf), nil, time.Second)

	s.event("meta", map[string]bool{"has_cv": false})
	s.comment("keepalive")

	if !strings.Contains(buf.String(), "event: meta") {
		t.Errorf("body = %q, want the meta event written", buf.String())
	}
}

// streamFaultHub builds an isolated hub over a recording transport. It deliberately
// avoids the global sentry.Init that sentryApp uses: these tests assert what one hub
// delivered, and a package-global client would couple them to test execution order.
func streamFaultHub(t *testing.T) (*sentry.Hub, *recordingTransport) {
	t.Helper()
	tr := &recordingTransport{}
	client, err := sentry.NewClient(sentry.ClientOptions{
		Dsn:       "https://public@o0.ingest.sentry.io/0",
		Transport: tr,
	})
	if err != nil {
		t.Fatalf("sentry.NewClient: %v", err)
	}
	return sentry.NewHub(client, sentry.NewScope()), tr
}

// The whole point of the seam: a fault raised after the body began streaming is never
// returned to Fiber, so RenderError never sees it. Without this call such a failure is
// invisible in Sentry — which is exactly how the fit-stream outage went unreported.
func TestReportStreamFault_ReportsUnexpectedFault(t *testing.T) {
	hub, tr := streamFaultHub(t)

	reportStreamFault(hub, errString("llm chain exploded"))

	if got := tr.count(); got != 1 {
		t.Errorf("sentry events = %d, want exactly 1", got)
	}
}

// A reader that walked away is not an application fault. classify() already encodes
// that rule for the returned-error path; the streaming path must not disagree with it,
// or every closed tab becomes an error-inbox entry.
func TestReportStreamFault_IgnoresClientDisconnect(t *testing.T) {
	hub, tr := streamFaultHub(t)

	reportStreamFault(hub, fmt.Errorf("stage 1: %w", context.Canceled))

	if got := tr.count(); got != 0 {
		t.Errorf("sentry events = %d, want 0 for a client disconnect", got)
	}
}

// Sentry is opt-in and env-gated: with no DSN the middleware installs no hub, so the
// writer holds a nil one. Reporting must degrade to nothing rather than panic inside
// the SSE goroutine, where a panic would take the whole stream down.
func TestReportStreamFault_NilHubIsNoop(t *testing.T) {
	reportStreamFault(nil, errString("llm chain exploded"))
}

// failingWriter fails every write, standing in for a reader that went away.
type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("connection reset") }

// countingWriter records what was written and how many times, safely enough to be read
// from the test goroutine while the heartbeat writes from its own.
type countingWriter struct {
	mu     sync.Mutex
	writes int
}

func (c *countingWriter) Write(p []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.writes++
	return len(p), nil
}

func (c *countingWriter) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.writes
}

// A caller cancels its work when event reports false, so the two failure modes must not be
// spelled the same. A dead reader is a reason to stop; an unencodable frame is our own bug
// and stopping the turn over it would abort a live client's work for our mistake.
func TestSSEStream_EventDistinguishesADeadReaderFromOurOwnBug(t *testing.T) {
	var sb strings.Builder
	live := newSSEStream(bufio.NewWriter(&sb), nil, time.Second)

	if !live.event("token", map[string]string{"text": "hi"}) {
		t.Error("event over a live writer reported the client gone")
	}
	if got := sb.String(); !strings.Contains(got, "event: token") || !strings.Contains(got, `"text":"hi"`) {
		t.Errorf("frame = %q, want an SSE event carrying the payload", got)
	}

	// A channel cannot be marshalled. The frame never goes out, and the caller must not
	// read that as a disconnect.
	if !live.event("token", make(chan int)) {
		t.Error("an unencodable payload reported the client gone; the caller would cancel a live turn")
	}

	dead := newSSEStream(bufio.NewWriter(failingWriter{}), nil, time.Second)
	if dead.event("token", map[string]string{"text": "hi"}) {
		t.Error("event over a failing writer reported success; the turn would run on for nobody")
	}
}

// The stop returned by keepalive must not return until the goroutine has finished, or a
// comment can land in a body the caller believes is closed. Both endpoints hand-rolled this
// ticker-plus-WaitGroup pairing before it moved onto the type.
func TestSSEStream_KeepaliveStopsSynchronously(t *testing.T) {
	var w countingWriter
	stream := newSSEStream(bufio.NewWriter(&w), nil, time.Second)

	stop := stream.keepalive(time.Millisecond)
	// Let it beat a few times so the goroutine is genuinely in flight, not merely started.
	deadline := time.Now().Add(2 * time.Second)
	for w.count() == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if w.count() == 0 {
		t.Fatal("the heartbeat never wrote")
	}
	stop()

	settled := w.count()
	time.Sleep(20 * time.Millisecond) // ~20 ticks' worth, had it survived stop
	if got := w.count(); got != settled {
		t.Errorf("the heartbeat wrote %d more times after stop returned", got-settled)
	}
}

// comment swallows its failure: the heartbeat has nobody to report to, and the next event
// is what tells the caller the reader is gone.
func TestSSEStream_CommentIsBestEffort(t *testing.T) {
	stream := newSSEStream(bufio.NewWriter(failingWriter{}), nil, time.Second)
	stream.comment("keepalive") // must not panic
	if stream.event("token", "x") {
		t.Error("event over the same failing writer reported success")
	}
}
