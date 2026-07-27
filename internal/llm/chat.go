package llm

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/tmc/langchaingo/llms"
)

// Chat is the conversational sibling of GenerateJSON: instead of one system+user
// prompt in JSON mode it sends a whole conversation, offers the model a set of
// tools, and returns the chosen completion — which may be text, tool calls, or
// both. Content deltas are forwarded to onText as they stream (nil is fine); the
// full text is also on the returned choice, so a caller that does not stream
// loses nothing.
//
// The caller owns the loop: it inspects choice.ToolCalls, runs them, appends the
// results to msgs, and calls Chat again. Bounded by the client timeout and
// observed like the JSON helpers.
func (c *Client) Chat(ctx context.Context, msgs []llms.MessageContent, tools []llms.Tool, onText func(string)) (*llms.ContentChoice, error) {
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
		if onText != nil && len(chunk) > 0 {
			onText(string(chunk))
		}
		return nil
	})}
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
	g := gen()
	g.Output = choice.Content
	g.Usage = usageFrom(choice)
	c.observe(g)
	return choice, nil
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
