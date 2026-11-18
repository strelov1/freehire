package assistant

import (
	"context"
	"testing"

	"github.com/tmc/langchaingo/llms"
)

func TestCancellationStopsBeforeTheNextModelCall(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	m := &scriptedModel{replies: []*llms.ContentChoice{
		callReply("echo", `{"text":"hi"}`),
		textReply("never reached"),
	}}
	// Cancel while the first round is in flight: the loop must not start a second.
	m.onEachRun = func() { cancel() }
	q := &fakeQueries{}
	r := testRunner(m, q)

	var events []Event
	err := r.Run(ctx, Session{ID: sessionID, UserID: 3}, NewRegistry(echoTool()), "sys", "go", TurnConfig{}, func(e Event) { events = append(events, e) })
	if err != nil {
		t.Fatalf("a cancelled turn is not a failure: %v", err)
	}
	if m.calls != 1 {
		t.Errorf("model called %d times, want the loop to stop before the second round", m.calls)
	}
	if res := lastResult(t, events); res.StopReason != StopCancelled {
		t.Errorf("stop reason = %q, want %q", res.StopReason, StopCancelled)
	}
	if len(q.messages) == 0 {
		t.Error("the partial transcript must be persisted so the session is resumable")
	}
}

func TestACancelledTurnStillPersistsWhatWasSaid(t *testing.T) {
	// The user reads an answer on screen, then leaves — closing the tab, or hitting
	// stop. Whatever the model already said must be in the transcript: reopening the
	// chat to find the question and no answer looks like the assistant lost the work
	// it visibly did.
	ctx, cancel := context.WithCancel(context.Background())
	m := &scriptedModel{replies: []*llms.ContentChoice{textReply("Here is the answer.")}}
	m.onEachRun = cancel // the client goes away while the answer is streaming
	q := &fakeQueries{}
	r := testRunner(m, q)

	err := r.Run(ctx, Session{ID: sessionID, UserID: 3}, NewRegistry(), "sys", "a question", TurnConfig{}, func(Event) {})
	if err != nil {
		t.Fatalf("a cancelled turn is not a failure: %v", err)
	}

	roles := make([]string, len(q.messages))
	for i, msg := range q.messages {
		roles[i] = msg.Role
	}
	if len(q.messages) != 2 {
		t.Fatalf("transcript has %v, want the prompt AND the answer the user already saw", roles)
	}
	if roles[1] != RoleAssistant {
		t.Errorf("second message is %q, want the assistant's answer", roles[1])
	}
}

func TestACancelledToolRoundKeepsItsResults(t *testing.T) {
	// A tool that already ran changed the user's data; its result belongs in the
	// transcript even though the turn was abandoned, or the next turn re-runs it.
	ctx, cancel := context.WithCancel(context.Background())
	m := &scriptedModel{replies: []*llms.ContentChoice{
		callReply("echo", `{"text":"hi"}`),
		textReply("never reached"),
	}}
	m.onEachRun = cancel
	q := &fakeQueries{}
	r := testRunner(m, q)

	if err := r.Run(ctx, Session{ID: sessionID, UserID: 3}, NewRegistry(echoTool()), "sys", "go", TurnConfig{}, func(Event) {}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	roles := make([]string, len(q.messages))
	for i, msg := range q.messages {
		roles[i] = msg.Role
	}
	want := []string{RoleUser, RoleAssistant, RoleTool}
	if len(roles) != len(want) {
		t.Fatalf("transcript has %v, want %v", roles, want)
	}
}
