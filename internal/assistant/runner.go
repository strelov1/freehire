package assistant

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/tmc/langchaingo/llms"

	"github.com/strelov1/freehire/internal/llm"
)

// EventKind names one frame of a streamed turn. The set deliberately mirrors what
// the chat UI already renders, so a replayed transcript and a live turn fold
// through the same reducer.
type EventKind string

const (
	// EventQueued precedes a turn that had to wait for the session's previous one. It is not
	// produced by the loop — the turn has not started yet — but it travels the same stream, so
	// a client learns it is waiting instead of watching a silent connection and guessing.
	EventQueued           EventKind = "queued"
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

// TurnConfig bounds ONE turn, overriding the runner's own bounds where it is set.
// It exists because turns are not all the same size: a question takes two tool
// rounds, an unattended run over a vacancy's requirements takes dozens. A zero
// field means "use the runner's configured bound", so the default applies to every
// turn that does not ask for something else.
//
// The value is always chosen server-side. A ceiling is a spend limit on a metered
// gateway, and a limit a client can raise is not a limit.
type TurnConfig struct {
	// MaxSteps overrides RunnerConfig.MaxSteps for this turn when positive.
	MaxSteps int
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

// With returns a runner that drives a different model, leaving its bounds, its store and
// everything else alone.
//
// It exists so a turn can run on the caller's own gateway credential: that credential is
// per-user and per-turn, while the runner is built once at boot. A Runner is three fields,
// so cloning one per turn costs nothing beside the model call it is about to make.
//
// Nil-safe both ways: a nil runner stays nil (the assistant is unavailable), and a nil
// model leaves the runner as it was rather than producing one that cannot answer.
func (r *Runner) With(m Model) *Runner {
	if r == nil || m == nil {
		return r
	}
	clone := *r
	clone.model = m

	return &clone
}

const (
	defaultMaxSteps     = 8
	defaultHistoryLimit = 60
	// persistTimeout bounds one transcript write. The writes run on a context
	// detached from the caller's, so they need a deadline of their own.
	persistTimeout = 5 * time.Second
)

// persisting derives the context transcript writes run on: detached from the
// caller's cancellation, bounded by its own deadline. A turn's writes must
// outlive the request, because cancellation is exactly when they matter — the
// user watched the model answer and then closed the tab, and a transcript
// missing that answer looks like the assistant lost work it visibly did.
func persisting(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(ctx), persistTimeout)
}

// ErrNothingToContinue is returned by Continue when the session has no user
// message to resume from. The handler maps it to HTTP 409.
var ErrNothingToContinue = errors.New("assistant: nothing to continue")

// Run executes one turn: it records the prompt, then alternates model calls and
// tool calls until the model answers, the step cap is reached, or the caller goes
// away. Every frame is emitted through emit and persisted as it is produced, so a
// turn that dies halfway still leaves a resumable session.
//
// A tool failure is not a turn failure — it goes back to the model as a result.
// Only a model/transport failure returns an error, and even then the terminal
// event has already been emitted.
func (r *Runner) Run(ctx context.Context, sess Session, reg *Registry, system, prompt string, turn TurnConfig, emit func(Event)) error {
	if err := r.recordPrompt(ctx, sess, prompt, emit); err != nil {
		return err
	}
	return r.runFromHistory(ctx, sess, reg, system, turn, emit)
}

// Continue resumes after a failed turn without appending another user message.
// The transcript already holds the prompt (and any partial tool work); replaying
// a second copy would duplicate the candidate's request in the model's context.
// Dangling tool_use rows from the interrupted turn are healed when history is
// rebuilt — same path every other turn uses — so Bedrock stays provider-legal.
func (r *Runner) Continue(ctx context.Context, sess Session, reg *Registry, system string, turn TurnConfig, emit func(Event)) error {
	stored, err := r.store.Transcript(ctx, sess.ID)
	if err != nil {
		return r.fail(emit, err)
	}
	if !hasUserMessage(stored) {
		return ErrNothingToContinue
	}
	// Activity only — no new prompt, no re-label.
	writeCtx, cancel := persisting(ctx)
	defer cancel()
	if err := r.store.Touch(writeCtx, sess.ID); err != nil {
		log.Printf("assistant: touch session %s: %v", sess.ID, err)
	}
	return r.runFromHistory(ctx, sess, reg, system, turn, emit)
}

func hasUserMessage(msgs []Message) bool {
	for _, m := range msgs {
		if m.Role == RoleUser {
			return true
		}
	}
	return false
}

// runFromHistory is the shared tool-calling loop. Run records a prompt first;
// Continue skips that so a retry does not rewrite the conversation.
func (r *Runner) runFromHistory(ctx context.Context, sess Session, reg *Registry, system string, turn TurnConfig, emit func(Event)) error {
	history, err := r.history(ctx, sess.ID, system)
	if err != nil {
		return r.fail(emit, err)
	}

	maxSteps := r.cfg.MaxSteps
	if turn.MaxSteps > 0 {
		maxSteps = turn.MaxSteps
	}

	for step := 0; step < maxSteps; step++ {
		if ctx.Err() != nil {
			return end(emit, StopCancelled)
		}

		choice, err := r.model.Chat(ctx, history, reg.Definitions(), stream(emit))
		if err != nil {
			// A cancelled context surfaces as a model error; report it as the
			// cancellation it is rather than as a failure the user must act on.
			if ctx.Err() != nil {
				return end(emit, StopCancelled)
			}
			return r.fail(emit, err)
		}

		history = r.appendAssistant(ctx, sess.ID, history, choice)

		if len(choice.ToolCalls) == 0 {
			emitUsage(emit, choice)
			return end(emit, StopEndTurn)
		}

		history = r.runToolCalls(ctx, sess, reg, history, choice.ToolCalls, emit)

		// Cancelled mid-round: the tools that already ran are committed and
		// persisted; stop before spending another model call.
		if ctx.Err() != nil {
			return end(emit, StopCancelled)
		}
	}

	// The cap is reached. Ask once more with no tools offered so the model can
	// only answer. Some providers (Haiku on Bedrock) still emit tool calls when
	// the list is empty — persisting those without running them left a dangling
	// tool_use the UI showed as a hung turn. Execute any that arrive, then stop
	// without spending another model round past the ceiling.
	choice, err := r.model.Chat(ctx, history, nil, stream(emit))
	if err != nil {
		if ctx.Err() != nil {
			return end(emit, StopCancelled)
		}
		return r.fail(emit, err)
	}
	history = r.appendAssistant(ctx, sess.ID, history, choice)
	if len(choice.ToolCalls) > 0 {
		log.Printf("assistant: max_steps wrap-up returned %d tool call(s); running them before stop", len(choice.ToolCalls))
		history = r.runToolCalls(ctx, sess, reg, history, choice.ToolCalls, emit)
	}
	if strings.TrimSpace(choice.Content) == "" {
		r.noteStepLimit(ctx, sess.ID, &history, emit)
	}
	emitUsage(emit, choice)
	return end(emit, StopMaxSteps)
}

// end closes a turn with its terminal event. Every exit from the loop goes
// through here or through fail, so a turn can never finish without telling the
// client why.
func end(emit func(Event), reason string) error {
	emit(Event{Kind: EventResult, StopReason: reason})
	return nil
}

// noteStepLimit records a short prose line when the max_steps wrap-up produced
// tool calls (or nothing) and no answer text. Without it the client only sees a
// silent stop after the last tool chip.
func (r *Runner) noteStepLimit(ctx context.Context, sessionID uuid.UUID, history *[]llms.MessageContent, emit func(Event)) {
	const note = "I hit the step limit for this turn before finishing. Send another message and I will continue."
	emit(Event{Kind: EventAssistantText, Text: note})
	msg, err := EncodeAssistant(note, nil)
	if err != nil {
		log.Printf("assistant: encode step-limit note: %v", err)
		return
	}
	if err := r.persist(ctx, sessionID, msg); err != nil {
		log.Printf("assistant: persist step-limit note: %v", err)
		return
	}
	decoded, err := msg.Decode()
	if err != nil {
		log.Printf("assistant: decode step-limit note: %v", err)
		return
	}
	*history = append(*history, decoded)
}

// recordPrompt persists the user's message, names the session after it when it is
// the first, and emits it so a replay and a live turn look identical.
func (r *Runner) recordPrompt(ctx context.Context, sess Session, prompt string, emit func(Event)) error {
	msg, err := EncodeUser(prompt)
	if err != nil {
		return err
	}
	writeCtx, cancel := persisting(ctx)
	defer cancel()
	if _, err := r.store.Append(writeCtx, sess.ID, msg); err != nil {
		return err
	}
	// Label and activity stamp are conveniences, not correctness: a failure here
	// must not cost the user their turn.
	if err := r.store.LabelSession(writeCtx, sess.ID, prompt); err != nil {
		log.Printf("assistant: label session %s: %v", sess.ID, err)
	}
	if err := r.store.Touch(writeCtx, sess.ID); err != nil {
		log.Printf("assistant: touch session %s: %v", sess.ID, err)
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
		raw := call.FunctionCall.Arguments
		var args json.RawMessage
		if looksConcatenated(raw) {
			// Two complete JSON values with no separator: the model intended a second
			// call. healToolArguments would keep only the first, silently dropping the
			// second — pass the raw string through instead so DecodeArgs's own
			// trailing-content check fails the call loudly (see looksConcatenated).
			args = json.RawMessage(raw)
		} else {
			args = json.RawMessage(healToolArguments(raw))
		}
		emit(Event{Kind: EventToolUse, Name: name, Input: validJSON(args)})

		res := reg.Call(ctx, sess.UserID, name, args)
		emit(Event{Kind: EventToolResult, Name: name, Result: res.Content, IsError: res.Failed})

		msg, err := EncodeToolResult(call.ID, name, res.Content)
		if err != nil {
			log.Printf("assistant: encode tool result: %v", err)
			continue
		}
		if err := r.persist(ctx, sess.ID, msg); err != nil {
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
func (r *Runner) appendAssistant(ctx context.Context, sessionID uuid.UUID, history []llms.MessageContent, choice *llms.ContentChoice) []llms.MessageContent {
	msg, err := EncodeAssistant(choice.Content, choice.ToolCalls)
	if err != nil {
		log.Printf("assistant: encode assistant message: %v", err)
		return history
	}
	if err := r.persist(ctx, sessionID, msg); err != nil {
		log.Printf("assistant: persist assistant message: %v", err)
	}
	decoded, err := msg.Decode()
	if err != nil {
		log.Printf("assistant: decode assistant message: %v", err)
		return history
	}
	return append(history, decoded)
}

// persist writes one message to the transcript, outliving the caller's
// cancellation.
func (r *Runner) persist(ctx context.Context, sessionID uuid.UUID, msg Message) error {
	writeCtx, cancel := persisting(ctx)
	defer cancel()
	_, err := r.store.Append(writeCtx, sessionID, msg)
	return err
}

// history rebuilds the model's conversation: the session's system prompt followed
// by its most recent messages.
func (r *Runner) history(ctx context.Context, sessionID uuid.UUID, system string) ([]llms.MessageContent, error) {
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
// whole turn rather than merely losing context. It then closes any tool_use that
// still has no matching tool_result — a turn that died after persisting the
// calls but before their results leaves exactly that shape, and Bedrock rejects
// the whole next turn for it.
func trim(msgs []Message, limit int) []Message {
	if limit > 0 && len(msgs) > limit {
		msgs = msgs[len(msgs)-limit:]
	}
	for len(msgs) > 0 && msgs[0].Role == RoleTool {
		msgs = msgs[1:]
	}
	return closeDanglingToolCalls(msgs)
}

// closeDanglingToolCalls inserts synthetic error tool results for any assistant
// tool_use that is not followed by its tool_result before the next non-tool
// message (or the end of the transcript). Replay must stay provider-legal even
// when a prior turn was interrupted mid-round.
func closeDanglingToolCalls(msgs []Message) []Message {
	if len(msgs) == 0 {
		return msgs
	}
	out := make([]Message, 0, len(msgs)+4)
	for i := 0; i < len(msgs); i++ {
		msg := msgs[i]
		out = append(out, msg)
		if msg.Role != RoleAssistant {
			continue
		}
		calls := assistantToolCalls(msg)
		if len(calls) == 0 {
			continue
		}
		answered := make(map[string]struct{}, len(calls))
		j := i + 1
		for j < len(msgs) && msgs[j].Role == RoleTool {
			answered[toolResultCallID(msgs[j])] = struct{}{}
			out = append(out, msgs[j])
			j++
		}
		for _, call := range calls {
			if _, ok := answered[call.ID]; ok || call.ID == "" {
				continue
			}
			if syn := syntheticToolError(call); syn.Role != "" {
				out = append(out, syn)
			}
		}
		i = j - 1
	}
	return out
}

func assistantToolCalls(msg Message) []toolCall {
	var c assistantContent
	if err := json.Unmarshal(msg.Content, &c); err != nil {
		return nil
	}
	return c.ToolCalls
}

func toolResultCallID(msg Message) string {
	var c toolResultContent
	if err := json.Unmarshal(msg.Content, &c); err != nil {
		return ""
	}
	return c.ToolCallID
}

func syntheticToolError(call toolCall) Message {
	msg, err := EncodeToolResult(call.ID, call.Name,
		`{"error":"previous turn interrupted before this tool result was recorded; retry if still needed"}`)
	if err != nil {
		log.Printf("assistant: encode synthetic tool result for %s: %v", call.ID, err)
		return Message{}
	}
	return msg
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

// emitUsage reports token counts when the provider surfaced them. It reads them
// through the LLM client's own extraction, so the stream and the tracer never
// disagree about what a turn cost.
func emitUsage(emit func(Event), choice *llms.ContentChoice) {
	usage := llm.UsageFrom(choice)
	if usage == nil {
		return
	}
	emit(Event{Kind: EventUsage, InputTokens: usage.Input, OutputTokens: usage.Output})
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
