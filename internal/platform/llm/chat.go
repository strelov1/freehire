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

// ChatStream carries the optional per-delta callbacks of a streamed chat. OnText receives
// the answer, OnThinking the model's reasoning; keep them apart, reasoning is not the answer.
type ChatStream struct {
	OnText     func(string)
	OnThinking func(string)
}

// Chat sends a whole conversation with a set of tools and returns the chosen completion
// (text, tool calls, or both). Deltas go through stream as they arrive; the full text is also
// on the returned choice.
//
// The caller owns the tool loop: run choice.ToolCalls, append the results to msgs, call again.
// A gateway that goes silent at our deadline and returns partial output is a failed call
// (contextOverrules), not a completion.
func (c *Client) Chat(ctx context.Context, msgs []llms.MessageContent, tools []llms.Tool, stream ChatStream) (*llms.ContentChoice, error) {
	if c.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, c.timeout)
		defer cancel()
	}

	start := time.Now()
	// The tracer's flat shape gets the system prompt and the latest user turn.
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
	opts = append(opts, c.temperatureOptions()...)
	// The provider calls this with both halves; read only the reasoning.
	if stream.OnThinking != nil {
		opts = append(opts, llms.WithStreamingReasoningFunc(func(_ context.Context, reasoning, _ []byte) error {
			if len(reasoning) > 0 {
				stream.OnThinking(string(reasoning))
			}
			return nil
		}))
	}
	if len(tools) > 0 {
		// The production `flagship` alias resolves to GPT-6 Luna, which accepts tool
		// calls only when reasoning is disabled. Keep this workaround off other models.
		if c.modelID == "flagship" || c.modelID == "gpt-6-luna" {
			ctx = withReasoning(ctx, ReasoningNone)
		}
		opts = append(opts, llms.WithTools(tools))
	}

	resp, err := c.model.GenerateContent(ctx, msgs, opts...)
	err = contextOverrules(ctx, err)
	if err != nil {
		wrapped := fmt.Errorf("llm: chat: %w", err)
		g := gen()
		g.Err = wrapped
		g.Cancelled = callerCancelled(ctx)
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
// langchaingo appends an argument fragment to the open call only when the delta's `type` is
// empty (openaiclient updateToolCalls), but the gateway sets `type: "function"` on every delta,
// so each fragment becomes its own nameless call. A nameless call is therefore a continuation
// of the previous one; a leading nameless call is dropped.
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
		// Copy so appending fragments never mutates the provider's response.
		fc := *call.FunctionCall
		out = append(out, llms.ToolCall{ID: call.ID, Type: call.Type, FunctionCall: &fc})
	}
	return out
}

// isToolCallChunk reports whether a streamed chunk is a marshalled tool-call payload rather
// than prose; langchaingo passes those to the content callback, which would print raw JSON
// into the chat. The calls still arrive typed on the returned choice.
//
// The check is by shape and deliberately narrow: a JSON array or object whose entries carry a
// `function` member. Look-alike prose is only suppressed live; the full content keeps it.
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

// firstMessageText returns the text of the first message with the given role, or "".
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
