// Package assistant is the in-app AI agent: a bounded tool-calling loop over the shared
// LLM client, a typed tool registry, and the persisted conversations the chat surface and
// the CV-tailoring workspace run on.
package assistant

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/tmc/langchaingo/llms"
)

// Roles a stored message can carry. They mirror the transcript table's CHECK constraint.
// The system prompt is derived from the session's preset at turn time and never stored.
const (
	RoleUser      = "user"
	RoleAssistant = "assistant"
	RoleTool      = "tool"
	// RoleNote is a status note freehire hands the model (what happened to the application a
	// conversation is held against), not anything the candidate said. Stored as the CHECK's
	// 'system' value; the client never renders it.
	RoleNote = "system"
)

// notePreamble opens a note as the model reads it. A note travels as a human turn (the one
// mid-conversation role every provider accepts), so the preamble keeps it from reading as
// the candidate's request.
const notePreamble = "[Status update from freehire — not written by the candidate]\n"

// noteContent is a status note's payload.
type noteContent struct {
	Text string `json:"text"`
}

// EncodeNote builds the stored message for a status note.
func EncodeNote(text string) (Message, error) {
	return encode(RoleNote, noteContent{Text: text})
}

// Message is one row of a session's transcript: the role plus a role-specific JSON payload.
// It is both what the client replays and what the model's history is rebuilt from.
type Message struct {
	Seq     int32           `json:"seq"`
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}

// userContent is a prompt: what the candidate typed, plus any documents attached to it.
type userContent struct {
	Text        string       `json:"text"`
	Attachments []Attachment `json:"attachments,omitempty"`
}

// assistantContent is one model turn: its prose (possibly empty on a pure
// tool-calling turn) and the tool calls it asked for.
type assistantContent struct {
	Text      string     `json:"text,omitempty"`
	ToolCalls []toolCall `json:"tool_calls,omitempty"`
}

// toolCall is a stored tool invocation. Arguments stay a string, as emitted, but must be
// valid JSON for Bedrock/LiteLLM on replay; healToolArguments repairs trailer junk.
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

// EncodeUser builds the stored message for a user prompt and the documents attached to it.
func EncodeUser(text string, docs ...Attachment) (Message, error) {
	return encode(RoleUser, userContent{Text: text, Attachments: docs})
}

// EncodeAssistant builds the stored message for one model turn: its text and requested tool
// calls, either of which may be empty. Arguments are healed so a replay to Bedrock cannot be
// poisoned by one malformed turn.
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

// EncodeToolResult builds the stored message for one tool's result, keyed to its call.
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

// Decode turns a stored message back into the model-facing form. An unknown role or
// undecodable payload is an error, not a skipped message: dropping part of a transcript
// would let the model answer from a conversation that never happened.
func (m Message) Decode() (llms.MessageContent, error) {
	switch m.Role {
	case RoleUser:
		var c userContent
		if err := json.Unmarshal(m.Content, &c); err != nil {
			return llms.MessageContent{}, fmt.Errorf("assistant: decode user message: %w", err)
		}
		return llms.TextParts(llms.ChatMessageTypeHuman, userTurnText(c)), nil

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

	case RoleNote:
		var c noteContent
		if err := json.Unmarshal(m.Content, &c); err != nil {
			return llms.MessageContent{}, fmt.Errorf("assistant: decode note: %w", err)
		}
		return llms.TextParts(llms.ChatMessageTypeHuman, notePreamble+c.Text), nil

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

// Conversation rebuilds a model history from a stored transcript, in order, without the
// system prompt (the caller prepends its preset's).
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

// minQuoteWords is the shortest citation UserSaid accepts. With no floor, a common short
// word would count as a verbatim quote of anything containing it.
const minQuoteWords = 3

// negationCues are words that, between a sentence boundary and the quote's start, flip the
// sentence into a denial ("I did NOT lead the migration"). Checked within the quote's
// sentence only. Deliberately blunt and high-recall: a false rejection just falls back to
// agent_inferred, a false acceptance would write a denial into a CV.
var negationCues = []string{
	"n't", "n’t", " not ", " not,", " never ", "no longer", "without ever", "cannot ",
	" не ", " нет ", " никогда ", // Russian
	" no ", " não ", " nunca ", // Spanish and Portuguese
	" nicht ", " nie ", " kein", // German, including inflected forms of kein
	" ne ", " n'", " n’", " pas ", " jamais ", // French
}

// UserSaid reports whether quote appears in what the user typed in this conversation,
// compared case-insensitively with whitespace collapsed, and is not negated by its sentence.
//
// It lets a tool verify a citation instead of trusting one. The comparison is literal on
// purpose (a paraphrase is what is being guarded against), and negation is checked because
// "I have NOT worked with Kubernetes" contains "worked with Kubernetes".
func UserSaid(transcript []Message, quote string) bool {
	original := quote
	quote = collapseSpace(quote)
	if quote == "" || len(strings.Fields(quote)) < minQuoteWords {
		return false
	}
	// A confirmed fact stays said after later turns; only merge consent is current-turn-only.
	for i := range transcript {
		if transcript[i].Role == RoleUser && UserConfirmed(transcript[:i+1], original) {
			return true
		}
	}
	for _, m := range transcript {
		if m.Role != RoleUser {
			continue
		}
		var c userContent
		if err := json.Unmarshal(m.Content, &c); err != nil {
			continue
		}
		text := collapseSpace(c.Text)
		for start := 0; ; {
			idx := strings.Index(text[start:], quote)
			if idx < 0 {
				break
			}
			idx += start
			if !sentenceNegatesQuoteAt(text, idx) {
				return true
			}
			start = idx + 1
		}
	}
	return false
}

// sentenceNegatesQuoteAt reports whether the sentence containing text[at:] has a negation
// cue before the quote. text is already collapseSpace-d, so cues match plain bytes.
func sentenceNegatesQuoteAt(text string, at int) bool {
	start := 0
	for _, sep := range []byte{'.', ';', '!', '?'} {
		if p := strings.LastIndexByte(text[:at], sep); p+1 > start {
			start = p + 1
		}
	}
	// Leading space lets a cue like " not " match at the sentence's first word.
	sentence := " " + text[start:at]
	for _, cue := range negationCues {
		if strings.Contains(sentence, cue) {
			return true
		}
	}
	return false
}

// collapseSpace lowercases and reduces whitespace runs to one space, so a quote that
// survived a line wrap still matches.
func collapseSpace(s string) string {
	return strings.ToLower(strings.Join(strings.Fields(s), " "))
}

// firstJSONValue decodes the first complete JSON value in s, skipping leading non-JSON text
// up to the first "{" or "[", and returns it with the remaining text. ok is false if none.
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

// healToolArguments returns args unchanged when already one JSON value; otherwise it carves
// out the first object/array (Haiku has appended "</invoke>" trailers) or falls back to "{}".
//
// Storage/replay only: it keeps the first value even when a second complete one follows, so
// it must never decide what a tool EXECUTES with. See looksConcatenated, which runToolCalls
// checks first.
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

// looksConcatenated reports whether args is more than one complete JSON value run together
// (two tool-call payloads) rather than one value plus non-JSON trailer. A concatenation
// means the model intended a second call that must not be silently dropped.
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
