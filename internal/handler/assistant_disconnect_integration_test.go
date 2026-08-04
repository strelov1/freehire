//go:build integration

// A turn must not be abandoned because the reader stopped reading. On a phone a backgrounded
// tab is frozen, not closed, and the failed write it produces used to be read as "the user
// left" — which threw away live work: an unattended tailoring pass lost its report after
// twenty-five committed CV edits.
//
// These tests drive a real socket rather than fiber's in-memory test transport, because the
// behaviour under test IS the socket dying mid-turn.
// Run with: go test -tags=integration ./internal/handler/
package handler

import (
	"context"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/tmc/langchaingo/llms"

	"github.com/strelov1/freehire/internal/auth"
	"github.com/strelov1/freehire/internal/llm"
)

// disconnectModel scripts a turn of two rounds and lets the test stand between them. The first
// round calls a tool, so the runner reaches its cancellation check; the second answers. The
// test cuts the connection while the first round is held open, which is the moment a phone
// freezes its tab.
type disconnectModel struct {
	calls    int
	started  chan struct{} // closed once the first round is under way
	release  chan struct{} // released once the test has cut or cancelled the connection
	answered chan struct{} // closed when the second round runs — the turn survived

	releaseOnce  sync.Once
	answeredOnce sync.Once
}

// newDisconnectModel wires the model and guarantees it is let go at the end of the test.
//
// A model left blocked on release holds the turn's goroutine, and fiber's Shutdown waits on the
// connection that goroutine is writing to, with no deadline — so a failing assertion would hang
// the whole package instead of reporting itself. t.Cleanup alone does NOT prevent that: cleanups
// run last-in-first-out, and serveOnSocket registers Shutdown after this, so Shutdown would run
// first and wait forever. Each test therefore also defers letGo, which runs before any cleanup;
// this registration is the backstop for a test that forgets.
func newDisconnectModel(t *testing.T) *disconnectModel {
	t.Helper()
	m := &disconnectModel{
		started:  make(chan struct{}),
		release:  make(chan struct{}),
		answered: make(chan struct{}),
	}
	t.Cleanup(m.letGo)
	return m
}

// letGo releases the model however many times it is called, so a test may release it explicitly
// and still be cleaned up after.
func (m *disconnectModel) letGo() {
	m.releaseOnce.Do(func() { close(m.release) })
}

const disconnectAnswer = "the answer nobody was listening for"

func (m *disconnectModel) Chat(_ context.Context, _ []llms.MessageContent, _ []llms.Tool, s llm.ChatStream) (*llms.ContentChoice, error) {
	m.calls++
	if m.calls == 1 {
		if s.OnText != nil {
			s.OnText("working on it")
		}
		close(m.started)
		<-m.release
		// The bank is wired in this harness, so the round does real work rather than tripping
		// over an unassembled dependency.
		return callReplyChoice("experience_search", `{"query":"anything"}`), nil
	}
	// Once, because a session may run more than one turn through this model and closing a
	// closed channel panics.
	m.answeredOnce.Do(func() { close(m.answered) })
	return &llms.ContentChoice{Content: disconnectAnswer}, nil
}

// serveOnSocket puts the app on a real loopback listener and returns its address.
func serveOnSocket(t *testing.T, app *fiber.App) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	go func() { _ = app.Listener(ln) }()
	t.Cleanup(func() { _ = app.Shutdown() })
	return ln.Addr().String()
}

// dialTurn opens a turn on a raw socket and returns the connection, so a test can decide what
// to do to it — cut it, or keep reading.
func dialTurn(t *testing.T, addr, sessionID, cookie string) net.Conn {
	t.Helper()
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}

	body := `{"text":"find me go jobs"}`
	request := fmt.Sprintf("POST /api/v1/assistant/sessions/%s/messages HTTP/1.1\r\n"+
		"Host: 127.0.0.1\r\nCookie: %s=%s\r\nContent-Type: application/json\r\n"+
		"Content-Length: %d\r\nConnection: close\r\n\r\n%s",
		sessionID, auth.CookieName, cookie, len(body), body)
	if _, err := conn.Write([]byte(request)); err != nil {
		t.Fatalf("write request: %v", err)
	}
	return conn
}

// startTurnInBackground opens a turn and keeps reading it — what a client that is actually
// watching does, so nothing about the connection can be mistaken for the user leaving. The
// returned channel closes when the stream ends.
func startTurnInBackground(t *testing.T, addr, sessionID, cookie string) <-chan struct{} {
	t.Helper()
	conn := dialTurn(t, addr, sessionID, cookie)

	done := make(chan struct{})
	go func() {
		defer close(done)
		defer func() { _ = conn.Close() }()
		_, _ = io.Copy(io.Discard, conn)
	}()
	return done
}

// startTurnThenCut opens a turn, waits for it to be under way, and cuts the connection —
// leaving the server writing into a socket that is gone.
func startTurnThenCut(t *testing.T, addr, sessionID, cookie string, started <-chan struct{}) {
	t.Helper()
	conn := dialTurn(t, addr, sessionID, cookie)

	// Read enough to know the response is open, so the cut lands mid-turn rather than before
	// the handler ever ran.
	if err := conn.SetReadDeadline(time.Now().Add(10 * time.Second)); err != nil {
		t.Fatalf("set read deadline: %v", err)
	}
	if _, err := conn.Read(make([]byte, 1)); err != nil {
		t.Fatalf("read first byte: %v", err)
	}

	select {
	case <-started:
	case <-time.After(10 * time.Second):
		t.Fatal("the turn never started")
	}
	if err := conn.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
}

func TestTurnSurvivesAClientThatStopsReading(t *testing.T) {
	pool := startPostgres(t)
	iss := auth.NewIssuer("test-secret", time.Hour)
	model := newDisconnectModel(t)
	defer model.letGo()
	app, _ := newAssistantApp(pool, iss, model)
	_, cookie := assistantUser(t, pool, iss, "disconnect@example.test", true)
	id := createSession(t, app, cookie)
	addr := serveOnSocket(t, app)

	startTurnThenCut(t, addr, id, cookie, model.started)
	model.letGo()

	select {
	case <-model.answered:
	case <-time.After(15 * time.Second):
		t.Fatal("the turn stopped when its reader went away; it must run to its own end")
	}

	// The work is only real if it was stored: the reader that would have carried it is gone.
	waitForTranscript(t, pool, id, disconnectAnswer)
}

// waitForTranscript polls the stored transcript for text, because the turn finishes on its own
// goroutine after the request that started it is long gone.
func waitForTranscript(t *testing.T, pool *pgxpool.Pool, sessionID, want string) {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	var seen string
	for time.Now().Before(deadline) {
		rows, err := pool.Query(context.Background(),
			`SELECT content FROM assistant_messages WHERE session_id = $1 ORDER BY seq`, sessionID)
		if err != nil {
			t.Fatalf("read transcript: %v", err)
		}
		seen = ""
		for rows.Next() {
			var content []byte
			if err := rows.Scan(&content); err != nil {
				rows.Close()
				t.Fatalf("scan transcript: %v", err)
			}
			seen += string(content)
		}
		rows.Close()

		if strings.Contains(seen, want) {
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatalf("transcript never carried %q; it holds:\n%s", want, seen)
}
