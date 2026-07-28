package assistant

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/tmc/langchaingo/llms"

	"github.com/strelov1/freehire/internal/llm"
)

// scriptedModel answers each call with the next scripted reply, recording the
// history and tool list it was given each time.
type scriptedModel struct {
	replies []*llms.ContentChoice
	err     error

	calls     int
	gotTools  [][]llms.Tool
	gotHist   [][]llms.MessageContent
	onEachRun func()
}

func (m *scriptedModel) Chat(_ context.Context, msgs []llms.MessageContent, tools []llms.Tool, s llm.ChatStream) (*llms.ContentChoice, error) {
	m.calls++
	m.gotTools = append(m.gotTools, tools)
	m.gotHist = append(m.gotHist, msgs)
	if m.onEachRun != nil {
		m.onEachRun()
	}
	if m.err != nil {
		return nil, m.err
	}
	if len(m.replies) == 0 {
		return &llms.ContentChoice{Content: "done"}, nil
	}
	reply := m.replies[0]
	m.replies = m.replies[1:]
	if s.OnText != nil && reply.Content != "" {
		s.OnText(reply.Content)
	}
	return reply, nil
}

func textReply(s string) *llms.ContentChoice { return &llms.ContentChoice{Content: s} }

func callReply(name, args string) *llms.ContentChoice {
	return &llms.ContentChoice{ToolCalls: []llms.ToolCall{{
		ID: "call_" + name, Type: "function",
		FunctionCall: &llms.FunctionCall{Name: name, Arguments: args},
	}}}
}

// collect runs a turn and returns the events it emitted.
func collect(t *testing.T, r *Runner, sess Session, reg *Registry, prompt string) ([]Event, error) {
	t.Helper()
	var got []Event
	err := r.Run(context.Background(), sess, reg, "system prompt", prompt, func(e Event) { got = append(got, e) })
	return got, err
}

func kinds(events []Event) []EventKind {
	out := make([]EventKind, len(events))
	for i, e := range events {
		out[i] = e.Kind
	}
	return out
}

func hasKind(events []Event, k EventKind) bool {
	for _, e := range events {
		if e.Kind == k {
			return true
		}
	}
	return false
}

func lastResult(t *testing.T, events []Event) Event {
	t.Helper()
	for i := len(events) - 1; i >= 0; i-- {
		if events[i].Kind == EventResult {
			return events[i]
		}
	}
	t.Fatal("the turn emitted no terminal result event; the client would wait forever")
	return Event{}
}

func testRunner(m *scriptedModel, q *fakeQueries) *Runner {
	return NewRunner(m, NewStore(q), RunnerConfig{MaxSteps: 3, HistoryLimit: 50})
}

func TestTurnWithoutToolsEndsInAnAnswer(t *testing.T) {
	m := &scriptedModel{replies: []*llms.ContentChoice{textReply("Hello there.")}}
	q := &fakeQueries{}
	r := testRunner(m, q)

	events, err := collect(t, r, Session{ID: 7, UserID: 3, Preset: PresetChat}, NewRegistry(), "hi")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if kinds(events)[0] != EventUserPrompt {
		t.Errorf("first event = %q, want the recorded user prompt", kinds(events)[0])
	}
	if !hasKind(events, EventAssistantText) {
		t.Errorf("events = %v, want the answer streamed", kinds(events))
	}
	if res := lastResult(t, events); res.StopReason != StopEndTurn || res.IsError {
		t.Errorf("result = %+v, want a clean end_turn", res)
	}
	if len(q.messages) != 2 {
		t.Errorf("persisted %d messages, want the prompt and the answer", len(q.messages))
	}
}

func TestTurnRunsAToolThenAnswers(t *testing.T) {
	m := &scriptedModel{replies: []*llms.ContentChoice{
		callReply("echo", `{"text":"hi"}`),
		textReply("It said hi."),
	}}
	q := &fakeQueries{}
	r := testRunner(m, q)

	events, err := collect(t, r, Session{ID: 7, UserID: 3}, NewRegistry(echoTool()), "say hi")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !hasKind(events, EventToolUse) || !hasKind(events, EventToolResult) {
		t.Fatalf("events = %v, want the tool call and its result surfaced", kinds(events))
	}
	if m.calls != 2 {
		t.Errorf("model called %d times, want two rounds", m.calls)
	}
	// The tool's result must be in the history of the second call, or the model
	// answers without having seen what the tool returned.
	second := m.gotHist[1]
	if second[len(second)-1].Role != llms.ChatMessageTypeTool {
		t.Errorf("second call's history ends with %q, want the tool result", second[len(second)-1].Role)
	}
	if len(q.messages) != 4 {
		t.Errorf("persisted %d messages, want prompt + tool call + tool result + answer", len(q.messages))
	}
}

func TestStepCapForcesAFinalAnswerWithoutTools(t *testing.T) {
	// The model never stops asking for tools.
	m := &scriptedModel{replies: []*llms.ContentChoice{
		callReply("echo", `{"text":"1"}`),
		callReply("echo", `{"text":"2"}`),
		callReply("echo", `{"text":"3"}`),
		textReply("Alright, here is what I found."),
	}}
	q := &fakeQueries{}
	r := NewRunner(m, NewStore(q), RunnerConfig{MaxSteps: 3, HistoryLimit: 50})

	events, err := collect(t, r, Session{ID: 7, UserID: 3}, NewRegistry(echoTool()), "go")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if m.calls != 4 {
		t.Fatalf("model called %d times, want 3 capped rounds plus one final answer", m.calls)
	}
	if tools := m.gotTools[3]; len(tools) != 0 {
		t.Errorf("final call offered %d tools, want none — that is what forces an answer", len(tools))
	}
	if res := lastResult(t, events); res.StopReason != StopMaxSteps {
		t.Errorf("stop reason = %q, want %q", res.StopReason, StopMaxSteps)
	}
}

func TestAMalformedToolCallIsCorrectableInTheSameTurn(t *testing.T) {
	m := &scriptedModel{replies: []*llms.ContentChoice{
		callReply("echo", `{"txt":"typo"}`), // wrong field name
		callReply("echo", `{"text":"fixed"}`),
		textReply("Done."),
	}}
	q := &fakeQueries{}
	r := testRunner(m, q)

	events, err := collect(t, r, Session{ID: 7, UserID: 3}, NewRegistry(echoTool()), "go")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	var firstResult Event
	for _, e := range events {
		if e.Kind == EventToolResult {
			firstResult = e
			break
		}
	}
	if !firstResult.IsError {
		t.Fatalf("first tool result = %+v, want it marked as failed", firstResult)
	}
	if !strings.Contains(firstResult.Result, "txt") {
		t.Errorf("result = %s, want the decode error naming the offending field", firstResult.Result)
	}
	if res := lastResult(t, events); res.IsError {
		t.Errorf("the turn should still end cleanly after the model corrected itself: %+v", res)
	}
}

func TestAFailingToolDoesNotAbortTheTurn(t *testing.T) {
	boom := Tool{
		Name:   "boom",
		Schema: map[string]any{"type": "object"},
		Run: func(context.Context, int64, json.RawMessage) (any, error) {
			return nil, errors.New("search backend unavailable")
		},
	}
	m := &scriptedModel{replies: []*llms.ContentChoice{
		callReply("boom", `{}`),
		textReply("Search is down; try again shortly."),
	}}
	r := testRunner(m, &fakeQueries{})

	events, err := collect(t, r, Session{ID: 7, UserID: 3}, NewRegistry(boom), "go")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res := lastResult(t, events); res.IsError || res.StopReason != StopEndTurn {
		t.Errorf("result = %+v, want the turn to end normally after a tool failure", res)
	}
}

func TestAModelFailureEndsTheTurnWithAnErrorResult(t *testing.T) {
	m := &scriptedModel{err: errors.New("gateway boom")}
	r := testRunner(m, &fakeQueries{})

	events, err := collect(t, r, Session{ID: 7, UserID: 3}, NewRegistry(), "hi")
	if err == nil {
		t.Fatal("Run should surface the model failure to its caller")
	}
	res := lastResult(t, events)
	if !res.IsError || res.StopReason != StopError {
		t.Errorf("result = %+v, want an errored terminal event so the client stops waiting", res)
	}
}

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
	err := r.Run(ctx, Session{ID: 7, UserID: 3}, NewRegistry(echoTool()), "sys", "go", func(e Event) { events = append(events, e) })
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

func TestHistoryIsBoundedToTheMostRecentMessages(t *testing.T) {
	q := &fakeQueries{}
	store := NewStore(q)
	ctx := context.Background()
	for i := range 10 {
		msg, _ := EncodeUser(strings.Repeat("old ", i+1))
		if _, err := store.Append(ctx, 7, msg); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	m := &scriptedModel{replies: []*llms.ContentChoice{textReply("ok")}}
	r := NewRunner(m, store, RunnerConfig{MaxSteps: 3, HistoryLimit: 4})

	if _, err := collect(t, r, Session{ID: 7, UserID: 3}, NewRegistry(), "new question"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	// system prompt + the 4 most recent transcript messages (the new prompt included).
	if got := len(m.gotHist[0]); got != 5 {
		t.Errorf("history had %d messages, want the system prompt plus the 4 most recent", got)
	}
	if m.gotHist[0][0].Role != llms.ChatMessageTypeSystem {
		t.Errorf("history starts with %q, want the system prompt", m.gotHist[0][0].Role)
	}
}

func TestHistoryNeverStartsWithAnOrphanToolResult(t *testing.T) {
	q := &fakeQueries{}
	store := NewStore(q)
	ctx := context.Background()
	call, _ := EncodeAssistant("", []llms.ToolCall{{ID: "c1", Type: "function", FunctionCall: &llms.FunctionCall{Name: "echo", Arguments: `{}`}}})
	result, _ := EncodeToolResult("c1", "echo", `{"ok":true}`)
	answer, _ := EncodeAssistant("done", nil)
	for _, msg := range []Message{call, result, answer} {
		if _, err := store.Append(ctx, 7, msg); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	m := &scriptedModel{replies: []*llms.ContentChoice{textReply("ok")}}
	// A window of 3 would start at the tool result, whose originating call is gone.
	r := NewRunner(m, store, RunnerConfig{MaxSteps: 3, HistoryLimit: 3})

	if _, err := collect(t, r, Session{ID: 7, UserID: 3}, NewRegistry(), "next"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	first := m.gotHist[0][1] // [0] is the system prompt
	if first.Role == llms.ChatMessageTypeTool {
		t.Error("history begins with a tool result whose call was trimmed away; providers reject that message sequence")
	}
}

func TestTheFirstUserMessageLabelsTheSession(t *testing.T) {
	q := &fakeQueries{}
	m := &scriptedModel{replies: []*llms.ContentChoice{textReply("ok")}}
	r := testRunner(m, q)

	if _, err := collect(t, r, Session{ID: 7, UserID: 3}, NewRegistry(), "find me go jobs"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if q.labelSet != "find me go jobs" {
		t.Errorf("label = %q, want the first user message", q.labelSet)
	}
}
