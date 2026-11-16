package llm

import (
	"context"
	"testing"
	"time"

	"github.com/tmc/langchaingo/llms"
)

// A text-only round keeps the tools in the request, so the cached prefix still matches, and
// forbids calling them with tool_choice "none". On flagship the reasoning workaround that rides
// on the tools stays in force: changing the effort would rewrite the prefix too.
func TestChatWithoutToolCallsKeepsToolsAndForbidsThem(t *testing.T) {
	m := &chatModel{chunks: []string{"done"}}
	c := &Client{model: m, modelID: "flagship", timeout: time.Second}

	if _, err := c.Chat(WithoutToolCalls(context.Background()), nil, []llms.Tool{searchTool()}, ChatStream{}); err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if len(m.gotTools) != 1 {
		t.Errorf("tools sent = %d, want the same tool as every other round", len(m.gotTools))
	}
	if m.gotChoice != "none" {
		t.Errorf("tool_choice = %v, want none", m.gotChoice)
	}
	if m.gotReasoning != ReasoningNone {
		t.Errorf("reasoning = %q, want the flagship tool-round effort %q", m.gotReasoning, ReasoningNone)
	}
}

func TestChatLeavesToolChoiceUnsetByDefault(t *testing.T) {
	m := &chatModel{}
	c := &Client{model: m, timeout: time.Second}

	if _, err := c.Chat(context.Background(), nil, []llms.Tool{searchTool()}, ChatStream{}); err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if m.gotChoice != nil {
		t.Errorf("tool_choice = %v, want unset (the provider's auto)", m.gotChoice)
	}
}
