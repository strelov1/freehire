package assistant

import (
	"encoding/json"
	"testing"

	"github.com/tmc/langchaingo/llms"
)

func TestUserMessageRoundTrip(t *testing.T) {
	stored, err := EncodeUser("find me go jobs")
	if err != nil {
		t.Fatalf("EncodeUser: %v", err)
	}
	if stored.Role != RoleUser {
		t.Fatalf("role = %q, want %q", stored.Role, RoleUser)
	}

	got, err := stored.Decode()
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if got.Role != llms.ChatMessageTypeHuman {
		t.Errorf("decoded role = %q, want human", got.Role)
	}
	if text := textParts(got); text != "find me go jobs" {
		t.Errorf("decoded text = %q, want the original prompt", text)
	}
}

func TestAssistantMessageWithToolCallsRoundTrip(t *testing.T) {
	calls := []llms.ToolCall{
		{ID: "call_1", Type: "function", FunctionCall: &llms.FunctionCall{Name: "search_jobs", Arguments: `{"q":"go"}`}},
		{ID: "call_2", Type: "function", FunctionCall: &llms.FunctionCall{Name: "facets", Arguments: `{}`}},
	}
	stored, err := EncodeAssistant("Looking those up.", calls)
	if err != nil {
		t.Fatalf("EncodeAssistant: %v", err)
	}
	if stored.Role != RoleAssistant {
		t.Fatalf("role = %q, want %q", stored.Role, RoleAssistant)
	}

	got, err := stored.Decode()
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if got.Role != llms.ChatMessageTypeAI {
		t.Errorf("decoded role = %q, want ai", got.Role)
	}
	if text := textParts(got); text != "Looking those up." {
		t.Errorf("decoded text = %q", text)
	}
	decoded := toolCallParts(got)
	if len(decoded) != 2 {
		t.Fatalf("decoded %d tool calls, want 2", len(decoded))
	}
	if decoded[0].ID != "call_1" || decoded[0].FunctionCall.Name != "search_jobs" || decoded[0].FunctionCall.Arguments != `{"q":"go"}` {
		t.Errorf("first tool call lost detail: %+v", decoded[0])
	}
	if decoded[1].FunctionCall.Name != "facets" {
		t.Errorf("second tool call = %+v, want facets", decoded[1])
	}
}

func TestHealToolArgumentsStripsHaikuXMLTrailer(t *testing.T) {
	// Haiku has appended Anthropic XML close-tags after a complete JSON object.
	// Bedrock rejects the next turn unless replay sees legal JSON arguments.
	poisoned := `{"ops":[{"kind":"insert","path":"experience[0].bullets[0]","value":"x","evidence_id":"9ce2fcf5-467b-498b-9c12-737c08cf4697"}],"note":"n","requirement":"r","requirement_status":"closed_bank"}` +
		"\n</invoke>\": \"\"}"
	want := `{"ops":[{"kind":"insert","path":"experience[0].bullets[0]","value":"x","evidence_id":"9ce2fcf5-467b-498b-9c12-737c08cf4697"}],"note":"n","requirement":"r","requirement_status":"closed_bank"}`
	if got := healToolArguments(poisoned); got != want {
		t.Fatalf("healToolArguments = %q, want %q", got, want)
	}

	stored, err := EncodeAssistant("", []llms.ToolCall{{
		ID: "tooluse_x", Type: "function",
		FunctionCall: &llms.FunctionCall{Name: "cv_edit", Arguments: poisoned},
	}})
	if err != nil {
		t.Fatalf("EncodeAssistant: %v", err)
	}
	got, err := stored.Decode()
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	calls := toolCallParts(got)
	if len(calls) != 1 || calls[0].FunctionCall.Arguments != want {
		t.Fatalf("replayed arguments = %+v, want healed JSON", calls)
	}
}

func TestLooksConcatenatedDetectsTwoJSONValues(t *testing.T) {
	if !looksConcatenated(`{"ops":[1]}{"ops":[2]}`) {
		t.Error("want true for two complete JSON objects with no separator")
	}
	if !looksConcatenated(`{"a":1} {"b":2}`) {
		t.Error("want true even with whitespace between the two values")
	}
}

func TestLooksConcatenatedFalseForTrailerJunkAndValidJSON(t *testing.T) {
	// The Haiku XML-trailer case healToolArguments patches — not a second value.
	poisoned := `{"ops":[1]}` + "\n</invoke>\": \"\"}"
	if looksConcatenated(poisoned) {
		t.Error("want false: trailing non-JSON junk is trailer noise, not a second call")
	}
	if looksConcatenated(`{"ops":[1]}`) {
		t.Error("want false for already-valid single JSON")
	}
	if looksConcatenated("") {
		t.Error("want false for empty args")
	}
}

func TestAssistantMessageWithoutTextOmitsTheTextPart(t *testing.T) {
	// A pure tool-call turn has no prose. Sending an empty text part back to the
	// model is a malformed message for some providers, so it must not be emitted.
	stored, err := EncodeAssistant("", []llms.ToolCall{
		{ID: "c", Type: "function", FunctionCall: &llms.FunctionCall{Name: "facets", Arguments: `{}`}},
	})
	if err != nil {
		t.Fatalf("EncodeAssistant: %v", err)
	}
	got, err := stored.Decode()
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if len(got.Parts) != 1 {
		t.Fatalf("parts = %d, want only the tool call", len(got.Parts))
	}
}

func TestToolResultRoundTrip(t *testing.T) {
	stored, err := EncodeToolResult("call_1", "search_jobs", `{"total":3}`)
	if err != nil {
		t.Fatalf("EncodeToolResult: %v", err)
	}
	if stored.Role != RoleTool {
		t.Fatalf("role = %q, want %q", stored.Role, RoleTool)
	}

	got, err := stored.Decode()
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if got.Role != llms.ChatMessageTypeTool {
		t.Errorf("decoded role = %q, want tool", got.Role)
	}
	if len(got.Parts) != 1 {
		t.Fatalf("parts = %d, want one tool response", len(got.Parts))
	}
	resp, ok := got.Parts[0].(llms.ToolCallResponse)
	if !ok {
		t.Fatalf("part is %T, want llms.ToolCallResponse", got.Parts[0])
	}
	if resp.ToolCallID != "call_1" || resp.Name != "search_jobs" || resp.Content != `{"total":3}` {
		t.Errorf("tool response lost detail: %+v", resp)
	}
}

func TestConversationRebuildsHistoryInOrder(t *testing.T) {
	user, _ := EncodeUser("find go jobs")
	assistantTurn, _ := EncodeAssistant("", []llms.ToolCall{
		{ID: "c1", Type: "function", FunctionCall: &llms.FunctionCall{Name: "search_jobs", Arguments: `{"q":"go"}`}},
	})
	result, _ := EncodeToolResult("c1", "search_jobs", `{"total":1}`)
	answer, _ := EncodeAssistant("Found one.", nil)

	history, err := Conversation([]Message{user, assistantTurn, result, answer})
	if err != nil {
		t.Fatalf("Conversation: %v", err)
	}
	wantRoles := []llms.ChatMessageType{
		llms.ChatMessageTypeHuman,
		llms.ChatMessageTypeAI,
		llms.ChatMessageTypeTool,
		llms.ChatMessageTypeAI,
	}
	if len(history) != len(wantRoles) {
		t.Fatalf("history has %d messages, want %d", len(history), len(wantRoles))
	}
	for i, want := range wantRoles {
		if history[i].Role != want {
			t.Errorf("message %d role = %q, want %q", i, history[i].Role, want)
		}
	}
}

func TestDecodeRejectsAnUnknownRole(t *testing.T) {
	m := Message{Role: "wizard", Content: json.RawMessage(`{}`)}
	if _, err := m.Decode(); err == nil {
		t.Fatal("Decode accepted an unknown role; a corrupt transcript must not reach the model")
	}
}

// textParts joins a decoded message's text parts.
func textParts(m llms.MessageContent) string {
	out := ""
	for _, p := range m.Parts {
		if t, ok := p.(llms.TextContent); ok {
			out += t.Text
		}
	}
	return out
}

// toolCallParts collects a decoded message's tool calls.
func toolCallParts(m llms.MessageContent) []llms.ToolCall {
	var out []llms.ToolCall
	for _, p := range m.Parts {
		if tc, ok := p.(llms.ToolCall); ok {
			out = append(out, tc)
		}
	}
	return out
}
