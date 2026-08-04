//go:build integration

// Stopping a turn used to be a side effect of the transport: the client aborted its read, the
// server's next write failed, and the turn was cancelled. That made "the user pressed Stop"
// indistinguishable from "the tab went to the background", so one of them had to be given a
// channel of its own. This is that channel.
// Run with: go test -tags=integration ./internal/handler/
package handler

import (
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/auth"
)

func TestCancelStopsARunningTurn(t *testing.T) {
	pool := startPostgres(t)
	iss := auth.NewIssuer("test-secret", time.Hour)
	model := newDisconnectModel(t)
	// Before any t.Cleanup: fiber's Shutdown waits on the connection the blocked turn holds,
	// so a failed assertion would hang the package rather than report itself.
	defer model.letGo()
	app, _ := newAssistantApp(pool, iss, model)
	_, cookie := assistantUser(t, pool, iss, "cancel@example.test", true)
	id := createSession(t, app, cookie)
	addr := serveOnSocket(t, app)

	// The turn is started on a connection the test keeps open — this is a deliberate stop,
	// not a disconnection.
	streamed := startTurnInBackground(t, addr, id, cookie)

	select {
	case <-model.started:
	case <-time.After(10 * time.Second):
		t.Fatal("the turn never started")
	}

	resp := assistantRequest(t, app, fiber.MethodPost, "/api/v1/assistant/sessions/"+id+"/cancel", cookie, nil)
	if resp.StatusCode != fiber.StatusNoContent {
		t.Fatalf("cancel: status %d, want 204", resp.StatusCode)
	}
	model.letGo()

	// The runner stops before spending another model call, so the second round never runs.
	select {
	case <-model.answered:
		t.Fatal("the turn kept going after it was told to stop")
	case <-time.After(3 * time.Second):
	}
	<-streamed

	// Stopping is not undoing: what the turn did before it was told to stop is the candidate's,
	// and the transcript has to carry it or the session cannot be resumed. Transcript writes
	// run detached from the turn's context for exactly this reason.
	waitForTranscript(t, pool, id, "experience_search")
}

// Cancelling an idle session is how a client stops a turn whose end it cannot see. Making it an
// error would force the client to first prove the turn is alive — which is the thing it cannot
// know.
func TestCancelIsHarmlessWhenNothingIsRunning(t *testing.T) {
	pool := startPostgres(t)
	iss := auth.NewIssuer("test-secret", time.Hour)
	app, _ := newAssistantApp(pool, iss, &turnModel{})
	_, cookie := assistantUser(t, pool, iss, "cancel-idle@example.test", true)
	id := createSession(t, app, cookie)

	resp := assistantRequest(t, app, fiber.MethodPost, "/api/v1/assistant/sessions/"+id+"/cancel", cookie, nil)
	if resp.StatusCode != fiber.StatusNoContent {
		t.Fatalf("cancel: status %d, want 204", resp.StatusCode)
	}
}

// A session someone else owns is reported missing, never forbidden — the same rule every other
// session route follows, so an id cannot be probed for existence.
func TestCancelIsOwnerScoped(t *testing.T) {
	pool := startPostgres(t)
	iss := auth.NewIssuer("test-secret", time.Hour)
	model := newDisconnectModel(t)
	// Before any t.Cleanup: fiber's Shutdown waits on the connection the blocked turn holds,
	// so a failed assertion would hang the package rather than report itself.
	defer model.letGo()
	app, _ := newAssistantApp(pool, iss, model)
	_, owner := assistantUser(t, pool, iss, "cancel-owner@example.test", true)
	_, stranger := assistantUser(t, pool, iss, "cancel-stranger@example.test", true)
	id := createSession(t, app, owner)
	addr := serveOnSocket(t, app)

	streamed := startTurnInBackground(t, addr, id, owner)
	select {
	case <-model.started:
	case <-time.After(10 * time.Second):
		t.Fatal("the turn never started")
	}

	resp := assistantRequest(t, app, fiber.MethodPost, "/api/v1/assistant/sessions/"+id+"/cancel", stranger, nil)
	if resp.StatusCode != fiber.StatusNotFound {
		t.Fatalf("cancel by a stranger: status %d, want 404", resp.StatusCode)
	}

	// And the turn it did not own is untouched.
	model.letGo()
	select {
	case <-model.answered:
	case <-time.After(10 * time.Second):
		t.Fatal("a stranger's refused cancel stopped the owner's turn")
	}
	<-streamed
}
