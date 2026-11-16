package assistant

import "testing"

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
