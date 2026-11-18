package assistant

import (
	"testing"

	"github.com/tmc/langchaingo/llms"
)

func TestReplayDropsEmptyAssistantAndMidHistoryOrphan(t *testing.T) {
	empty, err := EncodeAssistant("", nil)
	if err != nil {
		t.Fatal(err)
	}
	text, err := EncodeAssistant("done", nil)
	if err != nil {
		t.Fatal(err)
	}
	orphan, err := EncodeToolResult("missing", "echo", "ok")
	if err != nil {
		t.Fatal(err)
	}
	got := closeDanglingToolCalls([]Message{text, empty, orphan, text})
	if len(got) != 2 {
		t.Fatalf("repaired history = %+v", got)
	}
	for _, msg := range got {
		if msg.Role != RoleAssistant {
			t.Fatalf("unexpected role %s", msg.Role)
		}
	}
}

func TestReplayKeepsOnlyFirstMatchingToolAnswer(t *testing.T) {
	call, _ := EncodeAssistant("", []llms.ToolCall{{ID: "call", Type: "function", FunctionCall: &llms.FunctionCall{Name: "echo", Arguments: "{}"}}})
	first, _ := EncodeToolResult("call", "echo", "first")
	duplicate, _ := EncodeToolResult("call", "echo", "duplicate")
	orphan, _ := EncodeToolResult("other", "echo", "orphan")
	empty, _ := EncodeToolResult("", "echo", "empty")
	got := closeDanglingToolCalls([]Message{call, orphan, empty, first, duplicate})
	if len(got) != 2 || string(got[1].Content) != string(first.Content) {
		t.Fatalf("replayed results=%+v", got)
	}
}
