//go:build integration

// A session runs one turn at a time. Before the turn registry, a second message ran beside the
// first — two conversations editing one CV, each blind to the other's edits — and the only
// thing that usually prevented it was the client aborting its own stream first.
// Run with: go test -tags=integration ./internal/handler/
package handler

import (
	"net"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"github.com/strelov1/freehire/internal/auth"
)

// awaitEvent reads the stream until the named SSE event shows up, so a test can act on where a
// turn has got to instead of sleeping and hoping.
func awaitEvent(t *testing.T, conn net.Conn, event string) {
	t.Helper()
	if err := conn.SetReadDeadline(time.Now().Add(20 * time.Second)); err != nil {
		t.Fatalf("set read deadline: %v", err)
	}

	var seen strings.Builder
	buf := make([]byte, 512)
	for {
		n, err := conn.Read(buf)
		seen.Write(buf[:n])
		if strings.Contains(seen.String(), event) {
			return
		}
		if err != nil {
			t.Fatalf("waiting for %q: %v\nstream so far:\n%s", event, err, seen.String())
		}
	}
}

func TestASecondMessageWaitsForTheTurnInFlight(t *testing.T) {
	pool := startPostgres(t)
	iss := auth.NewIssuer("test-secret", time.Hour)
	model := newDisconnectModel(t)
	defer model.letGo()
	app, _ := newAssistantApp(pool, iss, model)
	_, cookie := assistantUser(t, pool, iss, "queue@example.test", true)
	id := createSession(t, app, cookie)
	addr := serveOnSocket(t, app)

	running := startTurnInBackground(t, addr, id, cookie)
	select {
	case <-model.started:
	case <-time.After(10 * time.Second):
		t.Fatal("the first turn never started")
	}

	// The second message queues. It says so before it says anything else — a stream that goes
	// quiet while it waits is indistinguishable from one that broke.
	queued := dialTurn(t, addr, id, cookie)
	defer func() { _ = queued.Close() }()
	awaitEvent(t, queued, "event: queued")

	// The third is refused: one waiter is a courtesy, a queue a client can grow is a way to
	// hold the process open.
	resp := assistantRequest(t, app, fiber.MethodPost, "/api/v1/assistant/sessions/"+id+"/messages", cookie,
		map[string]string{"text": "and another thing"})
	if resp.StatusCode != fiber.StatusConflict {
		t.Fatalf("third message: status %d, want 409", resp.StatusCode)
	}

	// Once the first turn ends, the one that waited runs as a turn of its own.
	model.letGo()
	<-running
	awaitEvent(t, queued, "event: result")
}

// A wait that runs out still owes its client a terminal event — a client that receives none
// waits forever — and must not leave the session looking occupied.
func TestAQueuedMessageThatWaitsTooLongEndsCleanly(t *testing.T) {
	pool := startPostgres(t)
	iss := auth.NewIssuer("test-secret", time.Hour)
	model := newDisconnectModel(t)
	defer model.letGo()
	app, handlers := newAssistantApp(pool, iss, model)
	_, cookie := assistantUser(t, pool, iss, "queue-timeout@example.test", true)
	id := createSession(t, app, cookie)
	addr := serveOnSocket(t, app)

	previous := turnQueueWait
	turnQueueWait = 300 * time.Millisecond
	t.Cleanup(func() { turnQueueWait = previous })

	running := startTurnInBackground(t, addr, id, cookie)
	select {
	case <-model.started:
	case <-time.After(10 * time.Second):
		t.Fatal("the first turn never started")
	}

	queued := dialTurn(t, addr, id, cookie)
	defer func() { _ = queued.Close() }()
	awaitEvent(t, queued, "event: queued")
	// The wait runs out while the turn ahead is still held: the client is told, rather than
	// left on a stream that says nothing more.
	awaitEvent(t, queued, "event: result")

	// And the waiting place is given back, so the session is not left refusing every later
	// message.
	if _, waiter, err := handlers.turns.claim(uuid.New(), func() {}); err != nil || waiter != nil {
		t.Fatalf("an unrelated session could not claim a slot: %v, %v", waiter, err)
	}
	model.letGo()
	<-running
}
