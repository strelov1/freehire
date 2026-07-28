package llm

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/tmc/langchaingo/llms"
)

// chatModel is a stub llms.Model for the conversational surface: it captures the
// messages and call options it was given, streams its content in chunks, and
// returns a canned choice (text and/or tool calls).
type chatModel struct {
	chunks    []string
	toolCalls []llms.ToolCall
	genInfo   map[string]any
	err       error

	gotMsgs  []llms.MessageContent
	gotTools []llms.Tool
}

func (m *chatModel) GenerateContent(ctx context.Context, msgs []llms.MessageContent, opts ...llms.CallOption) (*llms.ContentResponse, error) {
	m.gotMsgs = msgs
	var co llms.CallOptions
	for _, o := range opts {
		o(&co)
	}
	m.gotTools = co.Tools
	if m.err != nil {
		return nil, m.err
	}
	content := ""
	for _, chunk := range m.chunks {
		if co.StreamingFunc != nil {
			if err := co.StreamingFunc(ctx, []byte(chunk)); err != nil {
				return nil, err
			}
		}
		content += chunk
	}
	return &llms.ContentResponse{Choices: []*llms.ContentChoice{{
		Content:        content,
		ToolCalls:      m.toolCalls,
		GenerationInfo: m.genInfo,
	}}}, nil
}

func (*chatModel) Call(context.Context, string, ...llms.CallOption) (string, error) { return "", nil }

func searchTool() llms.Tool {
	return llms.Tool{Type: "function", Function: &llms.FunctionDefinition{
		Name:        "search_jobs",
		Description: "search vacancies",
	}}
}

func TestChatOffersToolsAndReturnsToolCalls(t *testing.T) {
	m := &chatModel{toolCalls: []llms.ToolCall{{
		ID:           "call_1",
		Type:         "function",
		FunctionCall: &llms.FunctionCall{Name: "search_jobs", Arguments: `{"q":"go"}`},
	}}}
	c := &Client{model: m, timeout: time.Second}

	choice, err := c.Chat(context.Background(), []llms.MessageContent{
		llms.TextParts(llms.ChatMessageTypeSystem, "sys"),
		llms.TextParts(llms.ChatMessageTypeHuman, "find go jobs"),
	}, []llms.Tool{searchTool()}, ChatStream{})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if len(choice.ToolCalls) != 1 || choice.ToolCalls[0].FunctionCall.Name != "search_jobs" {
		t.Fatalf("tool calls = %+v, want the model's single search_jobs call", choice.ToolCalls)
	}
	if len(m.gotTools) != 1 || m.gotTools[0].Function.Name != "search_jobs" {
		t.Errorf("tools passed to the model = %+v, want the registered tool", m.gotTools)
	}
	if len(m.gotMsgs) != 2 {
		t.Errorf("sent %d messages, want the whole conversation", len(m.gotMsgs))
	}
}

func TestChatStreamsTextDeltas(t *testing.T) {
	m := &chatModel{chunks: []string{"Here are ", "three roles."}}
	c := &Client{model: m, timeout: time.Second}

	var streamed string
	choice, err := c.Chat(context.Background(),
		[]llms.MessageContent{llms.TextParts(llms.ChatMessageTypeHuman, "hi")},
		nil,
		ChatStream{OnText: func(s string) { streamed += s }},
	)
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if streamed != "Here are three roles." {
		t.Errorf("streamed = %q, want the joined deltas", streamed)
	}
	if choice.Content != "Here are three roles." {
		t.Errorf("content = %q, want the full text", choice.Content)
	}
}

func TestChatWithoutCallbackDoesNotPanic(t *testing.T) {
	m := &chatModel{chunks: []string{"ok"}}
	c := &Client{model: m, timeout: time.Second}
	if _, err := c.Chat(context.Background(),
		[]llms.MessageContent{llms.TextParts(llms.ChatMessageTypeHuman, "hi")}, nil, ChatStream{}); err != nil {
		t.Fatalf("Chat with a nil callback: %v", err)
	}
}

func TestChatEmptyChoicesIsAnError(t *testing.T) {
	c := &Client{model: emptyChoiceModel{}, timeout: time.Second}
	if _, err := c.Chat(context.Background(),
		[]llms.MessageContent{llms.TextParts(llms.ChatMessageTypeHuman, "hi")}, nil, ChatStream{}); err == nil {
		t.Fatal("Chat returned nil error for a response with no choices")
	}
}

// emptyChoiceModel answers with a well-formed response that carries no choice —
// the shape a gateway returns when a provider refuses mid-stream.
type emptyChoiceModel struct{}

func (emptyChoiceModel) GenerateContent(context.Context, []llms.MessageContent, ...llms.CallOption) (*llms.ContentResponse, error) {
	return &llms.ContentResponse{}, nil
}
func (emptyChoiceModel) Call(context.Context, string, ...llms.CallOption) (string, error) {
	return "", nil
}

func TestChatObservesGenerationWithUsage(t *testing.T) {
	m := &chatModel{
		chunks:  []string{"done"},
		genInfo: map[string]any{"PromptTokens": 900, "CompletionTokens": 30, "TotalTokens": 930},
	}
	ct := &captureTracer{}
	c := &Client{model: m, timeout: time.Second, modelID: "chat-model", tracer: ct, source: "assistant"}

	if _, err := c.Chat(context.Background(), []llms.MessageContent{
		llms.TextParts(llms.ChatMessageTypeSystem, "sys"),
		llms.TextParts(llms.ChatMessageTypeHuman, "hi"),
	}, nil, ChatStream{}); err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if len(ct.got) != 1 {
		t.Fatalf("observed %d generations, want 1", len(ct.got))
	}
	g := ct.got[0]
	if g.Model != "chat-model" || g.Source != "assistant" || g.Output != "done" {
		t.Errorf("generation fields wrong: %+v", g)
	}
	if g.Usage == nil || g.Usage.Input != 900 || g.Usage.Total != 930 {
		t.Errorf("usage = %+v, want the model's reported counts", g.Usage)
	}
}

func TestChatObservesModelError(t *testing.T) {
	sentinel := errors.New("gateway boom")
	ct := &captureTracer{}
	c := &Client{model: &chatModel{err: sentinel}, timeout: time.Second, tracer: ct, source: "assistant"}

	_, err := c.Chat(context.Background(),
		[]llms.MessageContent{llms.TextParts(llms.ChatMessageTypeHuman, "hi")}, nil, ChatStream{})
	if !errors.Is(err, sentinel) {
		t.Errorf("returned error %v does not wrap the model error", err)
	}
	if len(ct.got) != 1 || ct.got[0].Err == nil {
		t.Fatalf("expected one error generation, got %+v", ct.got)
	}
}

func TestChatTimesOutOnStalledModel(t *testing.T) {
	c := &Client{model: blockingModel{}, timeout: 20 * time.Millisecond}
	done := make(chan error, 1)
	go func() {
		_, err := c.Chat(context.Background(),
			[]llms.MessageContent{llms.TextParts(llms.ChatMessageTypeHuman, "hi")}, nil, ChatStream{})
		done <- err
	}()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("Chat returned nil error, want a timeout error")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Chat did not return; the per-call timeout did not fire")
	}
}

// reasoningModel emits reasoning deltas alongside its content, as a thinking
// model behind the gateway does.
type reasoningModel struct{ thoughts, content []string }

func (m *reasoningModel) GenerateContent(ctx context.Context, _ []llms.MessageContent, opts ...llms.CallOption) (*llms.ContentResponse, error) {
	var co llms.CallOptions
	for _, o := range opts {
		o(&co)
	}
	full := ""
	for i, c := range m.content {
		thought := ""
		if i < len(m.thoughts) {
			thought = m.thoughts[i]
		}
		if co.StreamingFunc != nil {
			if err := co.StreamingFunc(ctx, []byte(c)); err != nil {
				return nil, err
			}
		}
		if co.StreamingReasoningFunc != nil {
			if err := co.StreamingReasoningFunc(ctx, []byte(thought), []byte(c)); err != nil {
				return nil, err
			}
		}
		full += c
	}
	return &llms.ContentResponse{Choices: []*llms.ContentChoice{{Content: full}}}, nil
}

func (*reasoningModel) Call(context.Context, string, ...llms.CallOption) (string, error) {
	return "", nil
}

func TestChatSeparatesThinkingFromAnswer(t *testing.T) {
	m := &reasoningModel{
		thoughts: []string{"They want ", "remote work. "},
		content:  []string{"Here are ", "two roles."},
	}
	c := &Client{model: m, timeout: time.Second}

	var text, thinking string
	choice, err := c.Chat(context.Background(),
		[]llms.MessageContent{llms.TextParts(llms.ChatMessageTypeHuman, "hi")},
		nil,
		ChatStream{
			OnText:     func(s string) { text += s },
			OnThinking: func(s string) { thinking += s },
		})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if text != "Here are two roles." {
		t.Errorf("text = %q, want only the answer deltas", text)
	}
	if thinking != "They want remote work. " {
		t.Errorf("thinking = %q, want only the reasoning deltas", thinking)
	}
	if choice.Content != "Here are two roles." {
		t.Errorf("content = %q, want the answer without the reasoning", choice.Content)
	}
}
