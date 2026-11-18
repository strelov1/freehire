package assistant

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/tmc/langchaingo/llms"
)

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

// replayWindow is the transcript as the store hands it to trim on one turn: the
// newest `fetch` messages, oldest first, with seq running up to maxSeq.
func replayWindow(maxSeq, fetch int32) []Message {
	start := maxSeq - fetch + 1
	if start < 1 {
		start = 1
	}
	out := make([]Message, 0, maxSeq-start+1)
	for seq := start; seq <= maxSeq; seq++ {
		out = append(out, Message{Seq: seq, Role: RoleUser, Content: json.RawMessage(`{"text":"x"}`)})
	}
	return out
}

// Prompt caching is a PREFIX match, so the oldest message in the replayed window
// decides whether any of the history can be served from cache. A window that keeps
// "the newest N" moves that head by one on every single turn, which is why the
// assistant's measured cache-hit rate sat at the size of its static head (system
// prompt plus tool schemas) and the conversation itself was re-read at full price
// every turn.
func TestReplayedHistoryHeadMovesInBlocksSoTheCacheSurvives(t *testing.T) {
	const (
		limit = 60
		turns = 25
	)

	heads := make(map[int32]struct{})
	for i := int32(0); i < turns; i++ {
		got := trim(replayWindow(200+i, limit+historyBlock), limit)
		if len(got) == 0 {
			t.Fatalf("turn %d: empty window", i)
		}
		heads[got[0].Seq] = struct{}{}
	}

	// 25 turns span at most two block boundaries.
	if len(heads) > 2 {
		t.Fatalf("window head took %d distinct values over %d turns, want at most 2 — "+
			"a head that moves every turn means the replayed history never caches", len(heads), turns)
	}
}

// The block quantisation must not let the window grow without bound: it buys prefix
// stability with up to one extra block of history, never more.
func TestReplayedHistoryWindowStaysBounded(t *testing.T) {
	const limit = 60

	for i := int32(0); i < 200; i++ {
		got := trim(replayWindow(200+i, limit+historyBlock), limit)
		if len(got) < limit || len(got) >= limit+historyBlockFor(limit) {
			t.Fatalf("maxSeq=%d: window is %d messages, want it in [%d, %d)",
				200+i, len(got), limit, limit+historyBlockFor(limit))
		}
	}
}
