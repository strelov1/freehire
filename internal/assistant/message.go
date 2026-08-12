// Package assistant is the in-app AI agent: a bounded tool-calling loop over the
// shared LLM client, a typed tool registry, and the persisted conversations both
// the chat surface and the CV-tailoring workspace run on.
package assistant

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/tmc/langchaingo/llms"
)

// Roles a stored message can carry. They mirror the transcript table's CHECK
// constraint; the system prompt is derived from the session's preset at turn time
// and is deliberately never stored, so changing a prompt does not require
// rewriting history.
const (
	RoleUser      = "user"
	RoleAssistant = "assistant"
	RoleTool      = "tool"
)

// Message is one row of a session's transcript: the role plus a role-specific
// JSON payload. It is both what the client replays and what the model's history is
// rebuilt from, so an assistant turn's tool calls and each tool's result are
// first-class messages rather than prose the model has to re-parse.
type Message struct {
	Seq     int32           `json:"seq"`
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}

// userContent is a plain prompt.
type userContent struct {
	Text string `json:"text"`
}

// assistantContent is one model turn: its prose (possibly empty on a pure
// tool-calling turn) and the tool calls it asked for.
type assistantContent struct {
	Text      string     `json:"text,omitempty"`
	ToolCalls []toolCall `json:"tool_calls,omitempty"`
}

// toolCall is a stored tool invocation. Arguments stay a string because that is
// what the model emitted — but they must be valid JSON for Bedrock/LiteLLM to
// accept on replay. healToolArguments repairs trailer junk (Haiku has appended
// XML like </invoke>) without re-encoding a parse tree when the bytes are already
// legal.
type toolCall struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// toolResultContent is one tool's answer, addressed back to the call it answers.
type toolResultContent struct {
	ToolCallID string `json:"tool_call_id"`
	Name       string `json:"name"`
	Result     string `json:"result"`
}

// EncodeUser builds the stored message for a user prompt.
func EncodeUser(text string) (Message, error) {
	return encode(RoleUser, userContent{Text: text})
}

// EncodeAssistant builds the stored message for one model turn: its text and the
// tool calls it requested. Either may be empty. Arguments are healed so a later
// retry can replay them to Bedrock — a single Haiku turn that appended </invoke>
// after valid JSON otherwise poisons the session forever.
func EncodeAssistant(text string, calls []llms.ToolCall) (Message, error) {
	c := assistantContent{Text: text}
	for _, call := range calls {
		if call.FunctionCall == nil {
			continue
		}
		c.ToolCalls = append(c.ToolCalls, toolCall{
			ID:        call.ID,
			Name:      call.FunctionCall.Name,
			Arguments: healToolArguments(call.FunctionCall.Arguments),
		})
	}
	return encode(RoleAssistant, c)
}

// EncodeToolResult builds the stored message for one tool's result, keyed to the
// call it answers so the model can match them up.
func EncodeToolResult(callID, name, result string) (Message, error) {
	return encode(RoleTool, toolResultContent{ToolCallID: callID, Name: name, Result: result})
}

func encode(role string, content any) (Message, error) {
	raw, err := json.Marshal(content)
	if err != nil {
		return Message{}, fmt.Errorf("assistant: encode %s message: %w", role, err)
	}
	return Message{Role: role, Content: raw}, nil
}

// Decode turns a stored message back into the model-facing form. An unknown role
// or an undecodable payload is an error rather than a skipped message: silently
// dropping part of a transcript would let the model answer from a conversation
// that never happened.
func (m Message) Decode() (llms.MessageContent, error) {
	switch m.Role {
	case RoleUser:
		var c userContent
		if err := json.Unmarshal(m.Content, &c); err != nil {
			return llms.MessageContent{}, fmt.Errorf("assistant: decode user message: %w", err)
		}
		return llms.TextParts(llms.ChatMessageTypeHuman, c.Text), nil

	case RoleAssistant:
		var c assistantContent
		if err := json.Unmarshal(m.Content, &c); err != nil {
			return llms.MessageContent{}, fmt.Errorf("assistant: decode assistant message: %w", err)
		}
		out := llms.MessageContent{Role: llms.ChatMessageTypeAI}
		if c.Text != "" {
			out.Parts = append(out.Parts, llms.TextContent{Text: c.Text})
		}
		for _, call := range c.ToolCalls {
			out.Parts = append(out.Parts, llms.ToolCall{
				ID:           call.ID,
				Type:         "function",
				FunctionCall: &llms.FunctionCall{Name: call.Name, Arguments: healToolArguments(call.Arguments)},
			})
		}
		return out, nil

	case RoleTool:
		var c toolResultContent
		if err := json.Unmarshal(m.Content, &c); err != nil {
			return llms.MessageContent{}, fmt.Errorf("assistant: decode tool message: %w", err)
		}
		return llms.MessageContent{
			Role:  llms.ChatMessageTypeTool,
			Parts: []llms.ContentPart{llms.ToolCallResponse{ToolCallID: c.ToolCallID, Name: c.Name, Content: c.Result}},
		}, nil
	}
	return llms.MessageContent{}, fmt.Errorf("assistant: unknown message role %q", m.Role)
}

// Conversation rebuilds a model history from a session's stored transcript, in
// order. The system prompt is not part of it — the caller prepends the one its
// preset selects.
func Conversation(msgs []Message) ([]llms.MessageContent, error) {
	out := make([]llms.MessageContent, 0, len(msgs))
	for _, m := range msgs {
		decoded, err := m.Decode()
		if err != nil {
			return nil, err
		}
		out = append(out, decoded)
	}
	return out, nil
}

// minQuoteWords is the shortest citation UserSaid will accept. A substring match with no
// floor lets a common short word — "I", "the", any filler that turns up somewhere in every
// session — count as a verbatim quote of anything containing it, which promotes an invented
// claim to the candidate's own words for free. Three words is short enough to not bother a
// real citation but long enough that it can't be found by accident in unrelated chatter.
const minQuoteWords = 3

// UserSaid reports whether quote appears in what the user actually typed in this
// conversation, compared case-insensitively and with whitespace collapsed.
//
// It exists so a tool can verify a citation instead of trusting one. A model that must
// quote the user verbatim to have its write treated as the user's assertion cannot promote
// its own inference by simply claiming the user said it — the transcript is checked. The
// comparison is deliberately literal: normalizing further (stemming, fuzzy matching) would
// let a paraphrase pass, and a paraphrase is precisely the thing being guarded against.
func UserSaid(transcript []Message, quote string) bool {
	quote = collapseSpace(quote)
	if quote == "" || len(strings.Fields(quote)) < minQuoteWords {
		return false
	}
	for _, m := range transcript {
		if m.Role != RoleUser {
			continue
		}
		var c userContent
		if err := json.Unmarshal(m.Content, &c); err != nil {
			continue
		}
		if strings.Contains(collapseSpace(c.Text), quote) {
			return true
		}
	}
	return false
}

// collapseSpace lowercases and reduces every whitespace run to a single space, so a quote
// that survived a line wrap still matches what the user typed.
func collapseSpace(s string) string {
	return strings.ToLower(strings.Join(strings.Fields(s), " "))
}

// firstJSONValue decodes the first complete JSON value in s, skipping any leading
// non-JSON text up to the first "{" or "[", and returns it along with whatever
// text still follows. ok is false when no valid JSON value could be decoded.
func firstJSONValue(s string) (value, rest string, ok bool) {
	start := strings.IndexAny(s, "{[")
	if start < 0 {
		return "", "", false
	}
	body := s[start:]
	dec := json.NewDecoder(strings.NewReader(body))
	var raw json.RawMessage
	if err := dec.Decode(&raw); err != nil {
		return "", "", false
	}
	return string(raw), body[dec.InputOffset():], true
}

// healToolArguments returns args unchanged when they are already one JSON value.
// Otherwise it carves out the first object/array (Haiku has been seen appending
// "\n</invoke>" and similar XML trailer after a complete object) or falls back
// to "{}" so Bedrock's converter never sees illegal tool-call arguments.
//
// This is a storage/replay concern only — it always keeps the first value, even when
// what follows is itself a second, complete JSON value (the model concatenated two
// calls) rather than trailer junk. That trade-off is fine for what gets persisted and
// replayed to the model on a later turn, but it must never be what decides what a tool
// actually EXECUTES with: see looksConcatenated, which runToolCalls checks first so a
// concatenated call fails loudly via DecodeArgs instead of silently running truncated.
func healToolArguments(args string) string {
	s := strings.TrimSpace(args)
	if s == "" {
		return "{}"
	}
	if json.Valid([]byte(s)) {
		return s
	}
	value, _, ok := firstJSONValue(s)
	if !ok {
		return "{}"
	}
	return value
}

// looksConcatenated reports whether args is more than one complete JSON value run
// together with no separator — e.g. two full tool-call payloads — as opposed to a
// single value followed by non-JSON trailer junk. The two must be told apart:
// healToolArguments keeps only the first value either way (see above), but a genuine
// concatenation means the model intended a SECOND call the caller must not silently
// drop by executing just the first.
func looksConcatenated(args string) bool {
	s := strings.TrimSpace(args)
	if s == "" || json.Valid([]byte(s)) {
		return false
	}
	_, rest, ok := firstJSONValue(s)
	if !ok {
		return false
	}
	rest = strings.TrimSpace(rest)
	return rest != "" && json.Valid([]byte(rest))
}
