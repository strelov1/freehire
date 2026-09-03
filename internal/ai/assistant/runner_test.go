package assistant

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/tmc/langchaingo/llms"

	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/llm"
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
	err := r.Run(context.Background(), sess, reg, "system prompt", prompt, TurnConfig{}, func(e Event) { got = append(got, e) })
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

// A turn runs on the caller's own gateway credential, which means a different model
// client per turn while the runner itself is built once at boot. Swapping the model must
// leave the loop's bounds and its store exactly as configured — a turn that quietly got
// the default ceiling instead of the raised one would cut an unattended run in half.
func TestWithSwapsTheModelAndKeepsTheBounds(t *testing.T) {
	original := &scriptedModel{replies: []*llms.ContentChoice{textReply("from the original")}}
	replacement := &scriptedModel{replies: []*llms.ContentChoice{textReply("from the replacement")}}
	q := &fakeQueries{}
	r := NewRunner(original, NewStore(q), RunnerConfig{MaxSteps: 3, HistoryLimit: 50})

	bound := r.With(replacement)
	if bound == r {
		t.Fatal("With returned the same runner; the original must keep its own model")
	}
	if bound.cfg != r.cfg || bound.store != r.store {
		t.Errorf("bounds or store changed: %+v against %+v", bound.cfg, r.cfg)
	}

	if _, err := collect(t, bound, Session{ID: sessionID, UserID: 3}, NewRegistry(), "hi"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if replacement.calls != 1 || original.calls != 0 {
		t.Errorf("original called %d times, replacement %d — the turn must run on the bound model",
			original.calls, replacement.calls)
	}
}

// A nil model is what an unconfigured gateway resolves to. It must leave the runner alone
// rather than produce one that cannot answer at all.
func TestWithANilModelLeavesTheRunnerUsable(t *testing.T) {
	m := &scriptedModel{replies: []*llms.ContentChoice{textReply("still here")}}
	r := testRunner(m, &fakeQueries{})

	if got := r.With(nil); got != r {
		t.Error("With(nil) should return the runner unchanged")
	}
}

func TestTurnWithoutToolsEndsInAnAnswer(t *testing.T) {
	m := &scriptedModel{replies: []*llms.ContentChoice{textReply("Hello there.")}}
	q := &fakeQueries{}
	r := testRunner(m, q)

	events, err := collect(t, r, Session{ID: sessionID, UserID: 3, Preset: PresetChat}, NewRegistry(), "hi")
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

	events, err := collect(t, r, Session{ID: sessionID, UserID: 3}, NewRegistry(echoTool()), "say hi")
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

// A model call can come back with no tool calls AND no text at all — observed in
// production after a turn's context grew large (a batch of tool results feeding the
// next call): the provider reports success with an empty completion. Silently treating
// that as a clean end_turn leaves nothing in the transcript and nothing in the UI, so a
// turn that "finished" looks indistinguishable from one that never started — the user
// sees the assistant go silent and has no way to tell a turn even ran.
func TestEmptyModelReplyGetsAFallbackNoteInsteadOfVanishing(t *testing.T) {
	m := &scriptedModel{replies: []*llms.ContentChoice{textReply("")}}
	q := &fakeQueries{}
	r := testRunner(m, q)

	events, err := collect(t, r, Session{ID: sessionID, UserID: 3}, NewRegistry(), "hi")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !hasKind(events, EventAssistantText) {
		t.Fatalf("events = %v, want a fallback note so the turn leaves a visible trace", kinds(events))
	}
	if res := lastResult(t, events); res.StopReason != StopEndTurn || res.IsError {
		t.Errorf("result = %+v, want a clean end_turn", res)
	}
	if len(q.messages) != 3 {
		t.Fatalf("persisted %d messages, want the prompt, the empty model turn, and a fallback note", len(q.messages))
	}
	last := q.messages[len(q.messages)-1]
	if last.Role != RoleAssistant {
		t.Fatalf("last persisted message role = %q, want assistant", last.Role)
	}
	if strings.TrimSpace(string(last.Content)) == `{"text":""}` || strings.TrimSpace(string(last.Content)) == `{}` {
		t.Errorf("persisted assistant content = %s, want a non-empty note", last.Content)
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

	events, err := collect(t, r, Session{ID: sessionID, UserID: 3}, NewRegistry(echoTool()), "go")
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

func TestMaxStepsWrapUpRunsToolCallsProvidersStillEmit(t *testing.T) {
	// Haiku on Bedrock has returned tool calls on the no-tools wrap-up call. Persisting
	// them without running left a dangling tool_use the UI showed as a hung turn.
	m := &scriptedModel{replies: []*llms.ContentChoice{
		callReply("echo", `{"text":"1"}`),
		callReply("echo", `{"text":"2"}`),
		callReply("echo", `{"text":"3"}`),
		callReply("echo", `{"text":"orphan"}`),
	}}
	q := &fakeQueries{}
	r := NewRunner(m, NewStore(q), RunnerConfig{MaxSteps: 3, HistoryLimit: 50})

	events, err := collect(t, r, Session{ID: sessionID, UserID: 3}, NewRegistry(echoTool()), "go")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res := lastResult(t, events); res.StopReason != StopMaxSteps {
		t.Errorf("stop reason = %q, want %q", res.StopReason, StopMaxSteps)
	}
	var toolResults int
	var sawNote bool
	for _, e := range events {
		if e.Kind == EventToolResult {
			toolResults++
		}
		if e.Kind == EventAssistantText && strings.Contains(e.Text, "step limit") {
			sawNote = true
		}
	}
	if toolResults != 4 {
		t.Errorf("tool results = %d, want 4 — the wrap-up call must still be executed", toolResults)
	}
	if !sawNote {
		t.Error("expected a step-limit note when the wrap-up had no prose")
	}
	roles := make([]string, 0, len(q.messages))
	for _, msg := range q.messages {
		roles = append(roles, msg.Role)
	}
	if roles[len(roles)-1] != RoleAssistant {
		t.Errorf("transcript ends with %v, want a closing assistant note (not a dangling tool_use)", roles[len(roles)-3:])
	}
}

// A turn may raise its own ceiling: an unattended run walks a dozen requirements where a
// question needs two rounds. The runner's configured default stands for every turn that names
// none, so raising it for one workflow cannot quietly raise it for the chat.
func TestAPerTurnCeilingReplacesTheRunnerDefault(t *testing.T) {
	m := &scriptedModel{replies: []*llms.ContentChoice{
		callReply("echo", `{"text":"1"}`),
		callReply("echo", `{"text":"2"}`),
		callReply("echo", `{"text":"3"}`),
		callReply("echo", `{"text":"4"}`),
		textReply("Done walking the list."),
	}}
	q := &fakeQueries{}
	r := NewRunner(m, NewStore(q), RunnerConfig{MaxSteps: 3, HistoryLimit: 50})

	var events []Event
	err := r.Run(context.Background(), Session{ID: sessionID, UserID: 3}, NewRegistry(echoTool()),
		"system prompt", "go", TurnConfig{MaxSteps: 6}, func(e Event) { events = append(events, e) })
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res := lastResult(t, events); res.StopReason != StopEndTurn {
		t.Errorf("stop reason = %q, want %q — four tool rounds fit under a ceiling of six", res.StopReason, StopEndTurn)
	}
	if m.calls != 5 {
		t.Errorf("model called %d times, want 4 tool rounds plus the answer", m.calls)
	}
}

func TestATurnNamingNoCeilingUsesTheConfiguredDefault(t *testing.T) {
	m := &scriptedModel{replies: []*llms.ContentChoice{
		callReply("echo", `{"text":"1"}`),
		callReply("echo", `{"text":"2"}`),
		callReply("echo", `{"text":"3"}`),
		textReply("Alright."),
	}}
	q := &fakeQueries{}
	r := NewRunner(m, NewStore(q), RunnerConfig{MaxSteps: 3, HistoryLimit: 50})

	events, err := collect(t, r, Session{ID: sessionID, UserID: 3}, NewRegistry(echoTool()), "go")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res := lastResult(t, events); res.StopReason != StopMaxSteps {
		t.Errorf("stop reason = %q, want %q — an unnamed ceiling is the runner's own", res.StopReason, StopMaxSteps)
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

	events, err := collect(t, r, Session{ID: sessionID, UserID: 3}, NewRegistry(echoTool()), "go")
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

// A provider that concatenates two complete JSON payloads for one tool call (the same
// malformation class healToolArguments exists to patch around, but a SECOND full value
// rather than trailer junk) must not have the call silently execute with only the first
// payload — that would run the model's edit half-applied while reporting success.
// Regression for a bug where healToolArguments' "keep the first value" storage/replay
// fallback was also feeding tool EXECUTION, so DecodeArgs' own trailing-content guard
// never got to see the raw string.
func TestAConcatenatedToolCallFailsInsteadOfRunningTruncated(t *testing.T) {
	m := &scriptedModel{replies: []*llms.ContentChoice{
		callReply("echo", `{"text":"first"}{"text":"second"}`),
		callReply("echo", `{"text":"fixed"}`),
		textReply("Done."),
	}}
	q := &fakeQueries{}
	r := testRunner(m, q)

	events, err := collect(t, r, Session{ID: sessionID, UserID: 3}, NewRegistry(echoTool()), "go")
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
		t.Fatalf("first tool result = %+v, want it marked as failed rather than run on the first half", firstResult)
	}
	if strings.Contains(firstResult.Result, "echoed") {
		t.Fatalf("result = %s, want no echoed payload — the call must not have executed", firstResult.Result)
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

	events, err := collect(t, r, Session{ID: sessionID, UserID: 3}, NewRegistry(boom), "go")
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

	events, err := collect(t, r, Session{ID: sessionID, UserID: 3}, NewRegistry(), "hi")
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

func TestHistoryIsBoundedToTheMostRecentMessages(t *testing.T) {
	q := &fakeQueries{}
	store := NewStore(q)
	ctx := context.Background()
	for i := range 10 {
		msg, _ := EncodeUser(strings.Repeat("old ", i+1))
		if _, err := store.Append(ctx, sessionID, msg); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	m := &scriptedModel{replies: []*llms.ContentChoice{textReply("ok")}}
	r := NewRunner(m, store, RunnerConfig{MaxSteps: 3, HistoryLimit: 4})

	if _, err := collect(t, r, Session{ID: sessionID, UserID: 3}, NewRegistry(), "new question"); err != nil {
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
		if _, err := store.Append(ctx, sessionID, msg); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	m := &scriptedModel{replies: []*llms.ContentChoice{textReply("ok")}}
	// A window of 3 would start at the tool result, whose originating call is gone.
	r := NewRunner(m, store, RunnerConfig{MaxSteps: 3, HistoryLimit: 3})

	if _, err := collect(t, r, Session{ID: sessionID, UserID: 3}, NewRegistry(), "next"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	first := m.gotHist[0][1] // [0] is the system prompt
	if first.Role == llms.ChatMessageTypeTool {
		t.Error("history begins with a tool result whose call was trimmed away; providers reject that message sequence")
	}
}

func TestHistoryClosesDanglingToolCallsBeforeReplay(t *testing.T) {
	// A turn that persisted the model's tool_use and then died leaves a transcript
	// Bedrock rejects on the next turn. Replay must synthesise the missing results.
	q := &fakeQueries{}
	store := NewStore(q)
	ctx := context.Background()
	call, _ := EncodeAssistant("", []llms.ToolCall{{ID: "c1", Type: "function", FunctionCall: &llms.FunctionCall{Name: "echo", Arguments: `{}`}}})
	user, _ := EncodeUser("still there?")
	for _, msg := range []Message{call, user} {
		if _, err := store.Append(ctx, sessionID, msg); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	m := &scriptedModel{replies: []*llms.ContentChoice{textReply("ok")}}
	r := NewRunner(m, store, RunnerConfig{MaxSteps: 3, HistoryLimit: 50})

	if _, err := collect(t, r, Session{ID: sessionID, UserID: 3}, NewRegistry(), "retry"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	hist := m.gotHist[0]
	var sawCall, sawResult bool
	for _, msg := range hist {
		for _, part := range msg.Parts {
			switch p := part.(type) {
			case llms.ToolCall:
				if p.ID == "c1" {
					sawCall = true
				}
			case llms.ToolCallResponse:
				if p.ToolCallID == "c1" {
					sawResult = true
				}
			}
		}
	}
	if !sawCall || !sawResult {
		t.Fatalf("replayed history missing closed tool pair: call=%v result=%v", sawCall, sawResult)
	}
}

func TestTheFirstUserMessageLabelsTheSession(t *testing.T) {
	// The fake now mirrors SetAssistantSessionLabel's owner scoping, so it must know
	// about a session matching the (id, user_id) the runner labels below.
	q := &fakeQueries{session: db.AssistantSession{ID: sessionID, UserID: 3, Preset: PresetChat}}
	m := &scriptedModel{replies: []*llms.ContentChoice{textReply("ok")}}
	r := testRunner(m, q)

	if _, err := collect(t, r, Session{ID: sessionID, UserID: 3}, NewRegistry(), "find me go jobs"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if q.labelSet != "find me go jobs" {
		t.Errorf("label = %q, want the first user message", q.labelSet)
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

// A retry after a transport failure must not append another copy of the user's
// prompt — that would leave the model answering a conversation that asked twice.
func TestContinueDoesNotRecordAnotherUserPrompt(t *testing.T) {
	failing := &scriptedModel{err: errors.New("upstream 502")}
	q := &fakeQueries{}
	r := testRunner(failing, q)

	var failed []Event
	err := r.Run(context.Background(), Session{ID: sessionID, UserID: 3}, NewRegistry(),
		"sys", "tailor this for me", TurnConfig{}, func(e Event) { failed = append(failed, e) })
	if err == nil {
		t.Fatal("want the first turn to fail")
	}
	if res := lastResult(t, failed); !res.IsError {
		t.Fatalf("first turn result = %+v, want is_error", res)
	}
	users := 0
	for _, m := range q.messages {
		if m.Role == RoleUser {
			users++
		}
	}
	if users != 1 {
		t.Fatalf("after the failed turn: %d user messages, want 1", users)
	}

	ok := &scriptedModel{replies: []*llms.ContentChoice{textReply("Picking up where we left off.")}}
	r = testRunner(ok, q)
	var events []Event
	if err := r.Continue(context.Background(), Session{ID: sessionID, UserID: 3}, NewRegistry(),
		"sys", TurnConfig{}, func(e Event) { events = append(events, e) }); err != nil {
		t.Fatalf("Continue: %v", err)
	}
	if hasKind(events, EventUserPrompt) {
		t.Fatal("Continue emitted user_prompt — that would duplicate the request in the UI and the model")
	}
	users = 0
	for _, m := range q.messages {
		if m.Role == RoleUser {
			users++
		}
	}
	if users != 1 {
		t.Fatalf("after Continue: %d user messages, want still 1", users)
	}
	if res := lastResult(t, events); res.StopReason != StopEndTurn || res.IsError {
		t.Fatalf("Continue result = %+v, want a clean end_turn", res)
	}
}

func TestContinueWithEmptyTranscriptIsRefused(t *testing.T) {
	r := testRunner(&scriptedModel{}, &fakeQueries{})
	err := r.Continue(context.Background(), Session{ID: sessionID, UserID: 3}, NewRegistry(),
		"sys", TurnConfig{}, func(Event) {})
	if !errors.Is(err, ErrNothingToContinue) {
		t.Fatalf("Continue on empty session: %v, want ErrNothingToContinue", err)
	}
}
