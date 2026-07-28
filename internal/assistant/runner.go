package assistant

import (
	"context"
	"encoding/json"
	"log"

	"github.com/tmc/langchaingo/llms"

	"github.com/strelov1/freehire/internal/llm"
)

// EventKind names one frame of a streamed turn. The set deliberately mirrors what
// the chat UI already renders, so a replayed transcript and a live turn fold
// through the same reducer.
type EventKind string

const (
	EventUserPrompt       EventKind = "user_prompt"
	EventAssistantText    EventKind = "assistant_text"
	EventAssistantThought EventKind = "assistant_thought"
	EventToolUse          EventKind = "tool_use"
	EventToolResult       EventKind = "tool_result"
	EventUsage            EventKind = "usage"
	EventResult           EventKind = "result"
)

// Why a turn ended. Every turn emits exactly one result event carrying one of
// these, so a client is never left waiting on a stream that quietly stopped.
const (
	StopEndTurn   = "end_turn"
	StopMaxSteps  = "max_steps"
	StopCancelled = "cancelled"
	StopError     = "error"
)

// Event is one streamed frame. The JSON tags are the wire contract the SSE
// endpoint and the web client share.
type Event struct {
	Kind EventKind `json:"type"`

	// Text carries the prompt (user_prompt) or one streamed delta
	// (assistant_text, assistant_thought).
	Text string `json:"text,omitempty"`

	// Name and Input describe a tool invocation; Result and IsError its outcome.
	Name    string          `json:"name,omitempty"`
	Input   json.RawMessage `json:"input,omitempty"`
	Result  string          `json:"result,omitempty"`
	IsError bool            `json:"is_error,omitempty"`

	// Token counts, when the provider reported them (usage).
	InputTokens  int `json:"input_tokens,omitempty"`
	OutputTokens int `json:"output_tokens,omitempty"`

	// StopReason is set on the terminal result event.
	StopReason string `json:"stop_reason,omitempty"`
}

// Model is the slice of the LLM client a turn needs. *llm.Client satisfies it;
// tests script it.
type Model interface {
	Chat(ctx context.Context, msgs []llms.MessageContent, tools []llms.Tool, stream llm.ChatStream) (*llms.ContentChoice, error)
}

// RunnerConfig bounds a turn.
type RunnerConfig struct {
	// MaxSteps is how many tool-calling rounds a turn may take before the model is
	// asked for a final answer with no tools offered.
	MaxSteps int
	// HistoryLimit is how many of a session's most recent messages are replayed
	// into the model. A long session otherwise grows past the context window.
	HistoryLimit int
}

// Runner executes turns: it owns the loop, its bounds, and the persistence of
// everything the loop produces.
type Runner struct {
	model Model
	store *Store
	cfg   RunnerConfig
}

// NewRunner wires a runner. Zero or negative bounds fall back to conservative
// defaults rather than meaning "unbounded" — an unbounded agent loop on a metered
// gateway is a runaway bill.
func NewRunner(m Model, s *Store, cfg RunnerConfig) *Runner {
	if cfg.MaxSteps <= 0 {
		cfg.MaxSteps = defaultMaxSteps
	}
	if cfg.HistoryLimit <= 0 {
		cfg.HistoryLimit = defaultHistoryLimit
	}
	return &Runner{model: m, store: s, cfg: cfg}
}

const (
	defaultMaxSteps     = 8
	defaultHistoryLimit = 60
)

// Run executes one turn: it records the prompt, then alternates model calls and
// tool calls until the model answers, the step cap is reached, or the caller goes
// away. Every frame is emitted through emit and persisted as it is produced, so a
// turn that dies halfway still leaves a resumable session.
//
// A tool failure is not a turn failure — it goes back to the model as a result.
// Only a model/transport failure returns an error, and even then the terminal
// event has already been emitted.
func (r *Runner) Run(ctx context.Context, sess Session, reg *Registry, system, prompt string, emit func(Event)) error {
	if err := r.recordPrompt(ctx, sess, prompt, emit); err != nil {
		return err
	}

	history, err := r.history(ctx, sess.ID, system)
	if err != nil {
		return r.fail(emit, err)
	}

	for step := 0; step < r.cfg.MaxSteps; step++ {
		if ctx.Err() != nil {
			emit(Event{Kind: EventResult, StopReason: StopCancelled})
			return nil
		}

		choice, err := r.model.Chat(ctx, history, reg.Definitions(), stream(emit))
		if err != nil {
			// A cancelled context surfaces as a model error; report it as the
			// cancellation it is rather than as a failure the user must act on.
			if ctx.Err() != nil {
				emit(Event{Kind: EventResult, StopReason: StopCancelled})
				return nil
			}
			return r.fail(emit, err)
		}

		history = r.appendAssistant(ctx, sess.ID, history, choice)

		if len(choice.ToolCalls) == 0 {
			emitUsage(emit, choice)
			emit(Event{Kind: EventResult, StopReason: StopEndTurn})
			return nil
		}

		history = r.runToolCalls(ctx, sess, reg, history, choice.ToolCalls, emit)

		// Cancelled mid-round: the tools that already ran are committed and
		// persisted; stop before spending another model call.
		if ctx.Err() != nil {
			emit(Event{Kind: EventResult, StopReason: StopCancelled})
			return nil
		}
	}

	// The cap is reached. Ask once more with no tools offered: the model can only
	// answer, so the turn always ends in prose rather than another tool request.
	choice, err := r.model.Chat(ctx, history, nil, stream(emit))
	if err != nil {
		if ctx.Err() != nil {
			emit(Event{Kind: EventResult, StopReason: StopCancelled})
			return nil
		}
		return r.fail(emit, err)
	}
	r.appendAssistant(ctx, sess.ID, history, choice)
	emitUsage(emit, choice)
	emit(Event{Kind: EventResult, StopReason: StopMaxSteps})
	return nil
}

// recordPrompt persists the user's message, names the session after it when it is
// the first, and emits it so a replay and a live turn look identical.
func (r *Runner) recordPrompt(ctx context.Context, sess Session, prompt string, emit func(Event)) error {
	msg, err := EncodeUser(prompt)
	if err != nil {
		return err
	}
	if _, err := r.store.Append(ctx, sess.ID, msg); err != nil {
		return err
	}
	// Label and activity stamp are conveniences, not correctness: a failure here
	// must not cost the user their turn.
	if err := r.store.LabelSession(ctx, sess.ID, prompt); err != nil {
		log.Printf("assistant: label session %d: %v", sess.ID, err)
	}
	if err := r.store.Touch(ctx, sess.ID); err != nil {
		log.Printf("assistant: touch session %d: %v", sess.ID, err)
	}
	emit(Event{Kind: EventUserPrompt, Text: prompt})
	return nil
}

// runToolCalls executes one round of tool calls, emitting and persisting each
// invocation and its result, and returns the history with the results appended.
func (r *Runner) runToolCalls(ctx context.Context, sess Session, reg *Registry, history []llms.MessageContent, calls []llms.ToolCall, emit func(Event)) []llms.MessageContent {
	for _, call := range calls {
		if call.FunctionCall == nil {
			continue
		}
		name := call.FunctionCall.Name
		args := json.RawMessage(call.FunctionCall.Arguments)
		emit(Event{Kind: EventToolUse, Name: name, Input: validJSON(args)})

		res := reg.Call(ctx, sess.UserID, name, args)
		emit(Event{Kind: EventToolResult, Name: name, Result: res.Content, IsError: res.Failed})

		msg, err := EncodeToolResult(call.ID, name, res.Content)
		if err != nil {
			log.Printf("assistant: encode tool result: %v", err)
			continue
		}
		if _, err := r.store.Append(ctx, sess.ID, msg); err != nil {
			log.Printf("assistant: persist tool result: %v", err)
		}
		decoded, err := msg.Decode()
		if err != nil {
			log.Printf("assistant: decode tool result: %v", err)
			continue
		}
		history = append(history, decoded)
	}
	return history
}

// appendAssistant persists one model turn and appends it to the working history.
func (r *Runner) appendAssistant(ctx context.Context, sessionID int64, history []llms.MessageContent, choice *llms.ContentChoice) []llms.MessageContent {
	msg, err := EncodeAssistant(choice.Content, choice.ToolCalls)
	if err != nil {
		log.Printf("assistant: encode assistant message: %v", err)
		return history
	}
	if _, err := r.store.Append(ctx, sessionID, msg); err != nil {
		log.Printf("assistant: persist assistant message: %v", err)
	}
	decoded, err := msg.Decode()
	if err != nil {
		log.Printf("assistant: decode assistant message: %v", err)
		return history
	}
	return append(history, decoded)
}

// history rebuilds the model's conversation: the session's system prompt followed
// by its most recent messages.
func (r *Runner) history(ctx context.Context, sessionID int64, system string) ([]llms.MessageContent, error) {
	stored, err := r.store.Transcript(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	msgs, err := Conversation(trim(stored, r.cfg.HistoryLimit))
	if err != nil {
		return nil, err
	}
	out := make([]llms.MessageContent, 0, len(msgs)+1)
	if system != "" {
		out = append(out, llms.TextParts(llms.ChatMessageTypeSystem, system))
	}
	return append(out, msgs...), nil
}

// trim keeps the most recent limit messages, then drops any leading tool results
// whose originating call was trimmed away. Providers reject a tool result that
// answers no call in the conversation, so an orphan at the head would fail the
// whole turn rather than merely losing context.
func trim(msgs []Message, limit int) []Message {
	if limit > 0 && len(msgs) > limit {
		msgs = msgs[len(msgs)-limit:]
	}
	for len(msgs) > 0 && msgs[0].Role == RoleTool {
		msgs = msgs[1:]
	}
	return msgs
}

// fail emits the terminal error event and returns the cause. The event goes out
// first so a streaming client stops waiting even though the request also errors.
func (r *Runner) fail(emit func(Event), err error) error {
	emit(Event{Kind: EventResult, StopReason: StopError, IsError: true})
	return err
}

// stream routes the model's two delta channels to their own event kinds: the
// answer as it is written, and the reasoning behind it, which the chat shows in a
// separate panel rather than inline.
func stream(emit func(Event)) llm.ChatStream {
	return llm.ChatStream{
		OnText:     func(delta string) { emit(Event{Kind: EventAssistantText, Text: delta}) },
		OnThinking: func(delta string) { emit(Event{Kind: EventAssistantThought, Text: delta}) },
	}
}

// emitUsage reports token counts when the provider surfaced them.
func emitUsage(emit func(Event), choice *llms.ContentChoice) {
	in, okIn := usageInt(choice.GenerationInfo["PromptTokens"])
	out, okOut := usageInt(choice.GenerationInfo["CompletionTokens"])
	if !okIn && !okOut {
		return
	}
	emit(Event{Kind: EventUsage, InputTokens: in, OutputTokens: out})
}

func usageInt(v any) (int, bool) {
	switch n := v.(type) {
	case int:
		return n, true
	case int64:
		return int(n), true
	case float64:
		return int(n), true
	}
	return 0, false
}

// validJSON renders tool arguments for the UI, falling back to a JSON string when
// the model emitted something unparseable — the frame is for display, so it must
// never carry invalid JSON into the stream.
func validJSON(args json.RawMessage) json.RawMessage {
	if len(args) > 0 && json.Valid(args) {
		return args
	}
	quoted, err := json.Marshal(string(args))
	if err != nil {
		return json.RawMessage(`{}`)
	}
	return quoted
}
