package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/tmc/langchaingo/llms"
)

// ChatStream carries the per-delta callbacks of a streamed chat. Both are
// optional. OnText receives the answer as it is produced; OnThinking receives the
// model's reasoning, when the provider surfaces any — the two are kept apart
// because the chat renders them differently, and reasoning must never be mistaken
// for the answer.
type ChatStream struct {
	OnText     func(string)
	OnThinking func(string)
}

// Chat is the conversational sibling of GenerateJSON: instead of one system+user
// prompt in JSON mode it sends a whole conversation, offers the model a set of
// tools, and returns the chosen completion — which may be text, tool calls, or
// both. Deltas are forwarded through stream as they arrive (a zero ChatStream is
// fine); the full text is also on the returned choice, so a caller that does not
// stream loses nothing.
//
// The caller owns the loop: it inspects choice.ToolCalls, runs them, appends the
// results to msgs, and calls Chat again. Bounded by the client timeout and
// observed like the JSON helpers.
func (c *Client) Chat(ctx context.Context, msgs []llms.MessageContent, tools []llms.Tool, stream ChatStream) (*llms.ContentChoice, error) {
	if c.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, c.timeout)
		defer cancel()
	}

	start := time.Now()
	// The tracer's flat system/user shape predates conversations; label an
	// observation with the conversation's system prompt and its latest user turn,
	// which is what makes a trace recognisable in Langfuse.
	gen := func() Generation {
		return Generation{
			Model:  c.modelID,
			System: firstMessageText(msgs, llms.ChatMessageTypeSystem),
			User:   lastMessageText(msgs, llms.ChatMessageTypeHuman),
			Start:  start,
			End:    time.Now(),
			Source: c.source,
		}
	}

	opts := []llms.CallOption{llms.WithStreamingFunc(func(_ context.Context, chunk []byte) error {
		if stream.OnText != nil && len(chunk) > 0 && !isToolCallChunk(chunk) {
			stream.OnText(string(chunk))
		}
		return nil
	})}
	// Reasoning arrives on its own callback; the provider calls both, so this one
	// reads only the reasoning half and leaves the content to OnText above.
	if stream.OnThinking != nil {
		opts = append(opts, llms.WithStreamingReasoningFunc(func(_ context.Context, reasoning, _ []byte) error {
			if len(reasoning) > 0 {
				stream.OnThinking(string(reasoning))
			}
			return nil
		}))
	}
	if len(tools) > 0 {
		opts = append(opts, llms.WithTools(tools))
	}

	resp, err := c.model.GenerateContent(ctx, msgs, opts...)
	if err != nil {
		wrapped := fmt.Errorf("llm: chat: %w", err)
		g := gen()
		g.Err = wrapped
		c.observe(g)
		return nil, wrapped
	}
	if len(resp.Choices) == 0 {
		err := errors.New("llm: model returned no choices")
		g := gen()
		g.Err = err
		c.observe(g)
		return nil, err
	}

	choice := resp.Choices[0]
	choice.ToolCalls = mergeToolCallFragments(choice.ToolCalls)
	g := gen()
	g.Output = choice.Content
	g.Usage = UsageFrom(choice)
	c.observe(g)
	return choice, nil
}

// mergeToolCallFragments folds a streamed tool call back into one call.
//
// A gateway streams a tool call as a first delta carrying the id and name, then
// deltas carrying only fragments of the argument JSON. langchaingo appends a
// fragment to the open call ONLY when the delta's `type` is empty
// (llms/openai/internal/openaiclient/chat.go, updateToolCalls) — but litellm sets
// `type: "function"` on every delta, so each fragment is filed as its own
// nameless call. The model's arguments then never reach the tool: it runs with an
// empty object while three bogus "unknown tool" calls burn the step cap.
//
// A call with no function name is therefore a continuation of the one before it.
// A leading nameless call has nothing to continue and is dropped — it cannot be
// dispatched to anything.
func mergeToolCallFragments(calls []llms.ToolCall) []llms.ToolCall {
	if len(calls) == 0 {
		return calls
	}
	out := make([]llms.ToolCall, 0, len(calls))
	for _, call := range calls {
		if call.FunctionCall == nil {
			continue
		}
		if call.FunctionCall.Name == "" {
			if len(out) == 0 {
				continue
			}
			out[len(out)-1].FunctionCall.Arguments += call.FunctionCall.Arguments
			continue
		}
		// Copy the function call so appending fragments never mutates the
		// provider's own response value.
		fc := *call.FunctionCall
		out = append(out, llms.ToolCall{ID: call.ID, Type: call.Type, FunctionCall: &fc})
	}
	return out
}

// isToolCallChunk reports whether a streamed chunk is a tool-call payload rather
// than prose. langchaingo hands the SAME streaming callback the marshalled tool
// calls in place of content when a delta carries them
// (llms/openai/internal/openaiclient/chat.go), so forwarding every chunk verbatim
// prints raw JSON into the user's chat. Nothing is lost by dropping these: the
// tool calls arrive properly typed on the returned choice.
//
// The check is by shape, and deliberately narrow — a JSON array or object whose
// entries carry a `function` member. Prose that happens to look like that is
// still recorded in the choice's full content; only the live delta is suppressed.
func isToolCallChunk(chunk []byte) bool {
	trimmed := bytes.TrimSpace(chunk)
	if len(trimmed) == 0 {
		return false
	}
	type call struct {
		Type     string          `json:"type"`
		Function json.RawMessage `json:"function"`
		Name     json.RawMessage `json:"name"`
	}
	switch trimmed[0] {
	case '[':
		var calls []call
		if err := json.Unmarshal(trimmed, &calls); err != nil || len(calls) == 0 {
			return false
		}
		return calls[0].Function != nil || calls[0].Type == "function"
	case '{':
		// A legacy function_call delta is marshalled as a bare object.
		var one call
		if err := json.Unmarshal(trimmed, &one); err != nil {
			return false
		}
		return one.Function != nil || one.Name != nil
	}
	return false
}

// firstMessageText returns the text of the first message with the given role,
// joining its text parts. Empty when the conversation has no such message.
func firstMessageText(msgs []llms.MessageContent, role llms.ChatMessageType) string {
	for _, m := range msgs {
		if m.Role == role {
			return textOf(m)
		}
	}
	return ""
}

// lastMessageText returns the text of the most recent message with the given role.
func lastMessageText(msgs []llms.MessageContent, role llms.ChatMessageType) string {
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role == role {
			return textOf(msgs[i])
		}
	}
	return ""
}

// textOf joins a message's text parts, ignoring tool calls and other part kinds.
func textOf(m llms.MessageContent) string {
	out := ""
	for _, p := range m.Parts {
		if t, ok := p.(llms.TextContent); ok {
			out += t.Text
		}
	}
	return out
}
