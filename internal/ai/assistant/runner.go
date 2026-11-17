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

	"github.com/strelov1/freehire/internal/platform/llm"
)

// EventKind names one frame of a streamed turn. The set mirrors what the chat UI
// renders, so a replayed transcript and a live turn fold through the same reducer.
type EventKind string

const (
	// EventQueued precedes a turn that had to wait for the session's previous one. The loop
	// does not produce it, but it travels the same stream so a client knows it is waiting.
	EventQueued           EventKind = "queued"
	EventUserPrompt       EventKind = "user_prompt"
	EventAssistantText    EventKind = "assistant_text"
	EventAssistantThought EventKind = "assistant_thought"
	EventToolUse          EventKind = "tool_use"
	EventToolResult       EventKind = "tool_result"
	EventUsage            EventKind = "usage"
	EventResult           EventKind = "result"
)

// Why a turn ended. Every turn emits exactly one result event carrying one of these.
const (
	StopEndTurn   = "end_turn"
	StopMaxSteps  = "max_steps"
	StopToolLoop  = "tool_loop"
	StopCancelled = "cancelled"
	StopError     = "error"
)

// Event is one streamed frame. The JSON tags are the wire contract shared with the web client.
type Event struct {
	Kind EventKind `json:"type"`

	// Text carries the prompt (user_prompt) or one streamed delta (assistant_text, assistant_thought).
	Text string `json:"text,omitempty"`
	// Attachments names the documents attached to the prompt (user_prompt). Names only: the
	// extracted text is not echoed back down the stream.
	Attachments []string `json:"attachments,omitempty"`

	// Name and Input describe a tool invocation; Result and IsError its outcome.
	Name    string          `json:"name,omitempty"`
	Input   json.RawMessage `json:"input,omitempty"`
	Result  string          `json:"result,omitempty"`
	IsError bool            `json:"is_error,omitempty"`

	// Token counts, when the provider reported them (usage). CachedInputTokens is the part of
	// InputTokens the provider served from its prompt cache; zero is also what a provider that
	// reports no cache sends (see llm.UsageFrom).
	InputTokens       int `json:"input_tokens,omitempty"`
	CachedInputTokens int `json:"cached_input_tokens,omitempty"`
	OutputTokens      int `json:"output_tokens,omitempty"`

	// StopReason is set on the terminal result event.
	StopReason string `json:"stop_reason,omitempty"`
}

// Model is the slice of the LLM client a turn needs. *llm.Client satisfies it.
type Model interface {
	Chat(ctx context.Context, msgs []llms.MessageContent, tools []llms.Tool, stream llm.ChatStream) (*llms.ContentChoice, error)
}

// RunnerConfig bounds a turn.
type RunnerConfig struct {
	// MaxSteps is how many tool-calling rounds a turn may take before the model is asked for
	// a final answer with tool calls forbidden.
	MaxSteps int
	// HistoryLimit is how many of a session's most recent messages are replayed into the model.
	HistoryLimit int
}

// TurnConfig bounds ONE turn, overriding the runner's bounds where set. A zero field means
// "use the runner's configured bound".
//
// Chosen server-side only: a ceiling is a spend limit on a metered gateway, and a limit a
// client can raise is not a limit.
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

// NewRunner wires a runner. Zero or negative bounds fall back to defaults, never
// "unbounded": an unbounded loop on a metered gateway is a runaway bill.
func NewRunner(m Model, s *Store, cfg RunnerConfig) *Runner {
	if cfg.MaxSteps <= 0 {
		cfg.MaxSteps = defaultMaxSteps
	}
	if cfg.HistoryLimit <= 0 {
		cfg.HistoryLimit = defaultHistoryLimit
	}
	return &Runner{model: m, store: s, cfg: cfg}
}

// With returns a runner that drives a different model, keeping its bounds and store. It
// lets a turn run on the caller's per-user gateway credential.
//
// Nil-safe: a nil runner stays nil, and a nil model leaves the runner unchanged.
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
	// historyBlock caps how far the replayed window's oldest message moves at a time.
	// A window of "the newest HistoryLimit messages" slides every turn, which breaks the
	// prompt-cache prefix match on every request. Quantising the window start to a multiple of
	// the block keeps the prefix byte-identical for a block's worth of turns, at the cost of up
	// to one block of extra (cached) history.
	historyBlock = 20
	// persistTimeout bounds one transcript write, which runs on a context detached from the
	// caller's.
	persistTimeout = 5 * time.Second
)

// persisting derives the context transcript writes run on: detached from the caller's
// cancellation, bounded by its own deadline. Writes must outlive the request, because
// cancellation (the user closing the tab) is exactly when they matter.
func persisting(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(ctx), persistTimeout)
}

// ErrNothingToContinue is returned by Continue when the session has no user message to
// resume from. The handler maps it to HTTP 409.
var ErrNothingToContinue = errors.New("assistant: nothing to continue")

// Run executes one turn: it records the prompt, then alternates model calls and tool calls
// until the model answers, the step cap is reached, or the caller goes away. Every frame is
// emitted and persisted as produced, so a turn that dies halfway leaves a resumable session.
//
// A tool failure goes back to the model as a result. Only a model/transport failure returns
// an error, and the terminal event has already been emitted by then.
func (r *Runner) Run(ctx context.Context, sess Session, reg *Registry, system, prompt string, turn TurnConfig, emit func(Event)) error {
	return r.RunWithAttachments(ctx, sess, reg, system, prompt, nil, turn, emit)
}

// RunWithAttachments is Run for a prompt with attached documents, stored in the prompt's
// own transcript row so every later turn replays them.
func (r *Runner) RunWithAttachments(ctx context.Context, sess Session, reg *Registry, system, prompt string, docs []Attachment, turn TurnConfig, emit func(Event)) error {
	if err := r.recordPrompt(ctx, sess, prompt, docs, emit); err != nil {
		// Through fail so the client still gets the terminal result; a bare return left the web
		// client with a clean-looking end and a session stuck "turn in flight".
		return r.fail(emit, err)
	}
	return r.runFromHistory(ctx, sess, reg, system, turn, emit)
}

// RunNote executes one turn whose input is a status note from freehire, not a message from
// the candidate. It is stored as RoleNote, does not name the session, and the client is not
// told the candidate said it.
//
// A note identical to the transcript's last message is not stored again, so retries while
// the model is down do not grow the conversation.
func (r *Runner) RunNote(ctx context.Context, sess Session, reg *Registry, system, note string, turn TurnConfig, emit func(Event)) error {
	msg, err := EncodeNote(note)
	if err != nil {
		return r.fail(emit, err)
	}
	stored, err := r.store.Transcript(ctx, sess.ID)
	if err != nil {
		return r.fail(emit, err)
	}
	writeCtx, cancel := persisting(ctx)
	defer cancel()
	if n := len(stored); n == 0 || stored[n-1].Role != RoleNote || string(stored[n-1].Content) != string(msg.Content) {
		if _, err := r.store.Append(writeCtx, sess.ID, msg); err != nil {
			return r.fail(emit, err)
		}
	}
	if err := r.store.Touch(writeCtx, sess.ID, sess.UserID); err != nil {
		log.Printf("assistant: touch session %s: %v", sess.ID, err)
	}
	return r.runFromHistory(ctx, sess, reg, system, turn, emit)
}

// Continue resumes after a failed turn without appending another user message, which
// would duplicate the request in the model's context. Dangling tool_use rows from the
// interrupted turn are healed when history is rebuilt.
func (r *Runner) Continue(ctx context.Context, sess Session, reg *Registry, system string, turn TurnConfig, emit func(Event)) error {
	stored, err := r.store.Transcript(ctx, sess.ID)
	if err != nil {
		return r.fail(emit, err)
	}
	if !hasUserMessage(stored) {
		return ErrNothingToContinue
	}
	// Activity only: no new prompt, no re-label.
	writeCtx, cancel := persisting(ctx)
	defer cancel()
	if err := r.store.Touch(writeCtx, sess.ID, sess.UserID); err != nil {
		log.Printf("assistant: touch session %s: %v", sess.ID, err)
	}
	return r.runFromHistory(ctx, sess, reg, system, turn, emit)
}

// hasUserMessage reports whether a transcript holds a turn to continue: a prompt, or a
// status note (an apply conversation's first turn is one).
func hasUserMessage(msgs []Message) bool {
	for _, m := range msgs {
		if m.Role == RoleUser || m.Role == RoleNote {
			return true
		}
	}
	return false
}

// runFromHistory is the shared tool-calling loop; Run records a prompt first, Continue
// does not.
func (r *Runner) runFromHistory(ctx context.Context, sess Session, reg *Registry, system string, turn TurnConfig, emit func(Event)) error {
	history, err := r.history(ctx, sess.ID, system)
	if err != nil {
		return r.fail(emit, err)
	}

	maxSteps := r.cfg.MaxSteps
	if turn.MaxSteps > 0 {
		maxSteps = turn.MaxSteps
	}

	failures := make(map[string]Result)
	stopReason := StopMaxSteps
	for step := 0; step < maxSteps; step++ {
		if ctx.Err() != nil {
			return end(emit, StopCancelled)
		}

		choice, err := r.model.Chat(ctx, history, reg.Definitions(), stream(emit))
		if err != nil {
			// A cancelled context surfaces as a model error; report it as cancellation.
			if ctx.Err() != nil {
				return end(emit, StopCancelled)
			}
			return r.fail(emit, err)
		}

		history = r.appendAssistant(ctx, sess.ID, history, choice)

		if len(choice.ToolCalls) == 0 {
			// A provider can report success with a completely empty completion after a large context.
			// Without a note the transcript and UI show no trace that the turn ran.
			if strings.TrimSpace(choice.Content) == "" {
				r.noteEmptyReply(ctx, sess.ID, emptyReplyNote, &history, emit)
			}
			emitUsage(emit, choice)
			return end(emit, StopEndTurn)
		}

		var repeated bool
		history, repeated = r.runToolCalls(ctx, sess, reg, history, choice.ToolCalls, failures, emit)

		// Cancelled mid-round: finished tools are persisted; stop before another model call.
		if ctx.Err() != nil {
			return end(emit, StopCancelled)
		}
		if repeated {
			stopReason = StopToolLoop
			break
		}
	}

	// Cap or repeated failure: ask once more for text only. The tools stay in the request so the provider's
	// cached prefix (tools, system, history) still matches; llm.WithoutToolCalls forbids calling
	// them. A provider may emit tool calls anyway (Haiku on Bedrock did with no tools at all);
	// run them so no tool_use is left dangling, then stop.
	choice, err := r.model.Chat(llm.WithoutToolCalls(ctx), history, reg.Definitions(), stream(emit))
	if err != nil {
		if ctx.Err() != nil {
			return end(emit, StopCancelled)
		}
		return r.fail(emit, err)
	}
	history = r.appendAssistant(ctx, sess.ID, history, choice)
	if len(choice.ToolCalls) > 0 {
		log.Printf("assistant: %s wrap-up returned %d tool call(s); running them before stop", stopReason, len(choice.ToolCalls))
		history, _ = r.runToolCalls(ctx, sess, reg, history, choice.ToolCalls, failures, emit)
	}
	if strings.TrimSpace(choice.Content) == "" {
		note := stepLimitNote
		if stopReason == StopToolLoop {
			note = "I stopped after the same tool call failed again. The rejected operation was not applied."
		}
		r.noteEmptyReply(ctx, sess.ID, note, &history, emit)
	}
	emitUsage(emit, choice)
	return end(emit, stopReason)
}

// end closes a turn with its terminal event. Every loop exit goes through here or fail.
func end(emit func(Event), reason string) error {
	emit(Event{Kind: EventResult, StopReason: reason})
	return nil
}

const (
	// stepLimitNote explains a max_steps wrap-up that produced no answer text.
	stepLimitNote = "I hit the step limit for this turn before finishing. Send another message and I will continue."
	// emptyReplyNote covers the model answering with nothing well short of the step cap.
	emptyReplyNote = "I didn't have anything to add there. Could you rephrase, or ask me to continue?"
)

// noteEmptyReply records a short prose line in place of a model turn that produced neither
// an answer nor a tool call.
func (r *Runner) noteEmptyReply(ctx context.Context, sessionID uuid.UUID, note string, history *[]llms.MessageContent, emit func(Event)) {
	emit(Event{Kind: EventAssistantText, Text: note})
	msg, err := EncodeAssistant(note, nil)
	if err != nil {
		log.Printf("assistant: encode empty-reply note: %v", err)
		return
	}
	if err := r.persist(ctx, sessionID, msg); err != nil {
		log.Printf("assistant: persist empty-reply note: %v", err)
		return
	}
	decoded, err := msg.Decode()
	if err != nil {
		log.Printf("assistant: decode empty-reply note: %v", err)
		return
	}
	*history = append(*history, decoded)
}

// recordPrompt persists the user's message, labels the session after it when it is the
// first, and emits it so a replay and a live turn look identical.
func (r *Runner) recordPrompt(ctx context.Context, sess Session, prompt string, docs []Attachment, emit func(Event)) error {
	msg, err := EncodeUser(prompt, docs...)
	if err != nil {
		return err
	}
	writeCtx, cancel := persisting(ctx)
	defer cancel()
	if _, err := r.store.Append(writeCtx, sess.ID, msg); err != nil {
		return err
	}
	// Label and activity stamp are conveniences: a failure must not cost the user their turn.
	// A message that is only a document labels the session by the document's name.
	label := prompt
	if label == "" && len(docs) > 0 {
		label = docs[0].Name
	}
	if err := r.store.LabelSession(writeCtx, sess.ID, sess.UserID, label); err != nil {
		log.Printf("assistant: label session %s: %v", sess.ID, err)
	}
	if err := r.store.Touch(writeCtx, sess.ID, sess.UserID); err != nil {
		log.Printf("assistant: touch session %s: %v", sess.ID, err)
	}
	emit(Event{Kind: EventUserPrompt, Text: prompt, Attachments: attachmentNames(docs)})
	return nil
}

// runToolCalls executes one round of tool calls, emitting and persisting each, and returns
// the history with the results appended.
func (r *Runner) runToolCalls(ctx context.Context, sess Session, reg *Registry, history []llms.MessageContent, calls []llms.ToolCall, failures map[string]Result, emit func(Event)) ([]llms.MessageContent, bool) {
	repeated := false
	for _, call := range calls {
		if call.FunctionCall == nil {
			continue
		}
		name := call.FunctionCall.Name
		raw := call.FunctionCall.Arguments
		var args json.RawMessage
		if looksConcatenated(raw) {
			// Two JSON values with no separator: healToolArguments would keep only the first and
			// silently drop the second, so pass the raw string through and let DecodeArgs fail loudly
			// (see looksConcatenated).
			args = json.RawMessage(raw)
		} else {
			args = json.RawMessage(healToolArguments(raw))
		}
		emit(Event{Kind: EventToolUse, Name: name, Input: validJSON(args)})

		key := rejectedCallKey(name, args)
		res, seen := failures[key]
		if seen {
			repeated = true
			res = failed("This call already failed with the same arguments: " + res.Content +
				". It was not executed again. Explain what remains unfinished; do not claim it succeeded.")
		} else {
			res = reg.Call(ctx, sess.UserID, name, args)
			if res.Failed {
				failures[key] = res
			} else {
				// A successful tool may have changed the state that caused the refusal.
				clear(failures)
			}
		}
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
	return history, repeated
}

func rejectedCallKey(name string, args []byte) string {
	var fields map[string]json.RawMessage
	if json.Unmarshal(args, &fields) != nil {
		return name + "\x00" + string(args)
	}
	if name == "cv_edit" {
		delete(fields, "note")
	}
	canonical, _ := json.Marshal(fields)
	return name + "\x00" + string(canonical)
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

// persist writes one message to the transcript, outliving the caller's cancellation.
func (r *Runner) persist(ctx context.Context, sessionID uuid.UUID, msg Message) error {
	writeCtx, cancel := persisting(ctx)
	defer cancel()
	_, err := r.store.Append(writeCtx, sessionID, msg)
	return err
}

// history rebuilds the model's conversation: the system prompt plus the most recent messages.
//
// It fetches only the tail, HistoryLimit+historyBlock rows: trim cuts at a block boundary,
// and the widest window that leaves is one block past the limit. Fetching exactly
// HistoryLimit would put the cut back on every turn.
func (r *Runner) history(ctx context.Context, sessionID uuid.UUID, system string) ([]llms.MessageContent, error) {
	stored, err := r.store.RecentTranscript(ctx, sessionID, r.cfg.HistoryLimit+historyBlock)
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

// trim keeps roughly the most recent limit messages, cutting at a block boundary so the
// replayed prefix survives the provider's prompt cache (see historyBlock). It drops leading
// tool results whose call was cut away (providers reject orphans) and closes any tool_use
// with no tool_result (Bedrock rejects the next turn otherwise).
//
// The caller must pass limit+historyBlock messages, else the cut is a no-op.
func trim(msgs []Message, limit int) []Message {
	if limit > 0 && len(msgs) > 0 {
		anchor := blockAnchor(int(msgs[len(msgs)-1].Seq), limit)
		cut := 0
		for cut < len(msgs) && int(msgs[cut].Seq) < anchor {
			cut++
		}
		msgs = msgs[cut:]
	}
	for len(msgs) > 0 && msgs[0].Role == RoleTool {
		msgs = msgs[1:]
	}
	return closeDanglingToolCalls(msgs)
}

// blockAnchor is the oldest seq the replayed window keeps: the start of the newest `limit`
// messages rounded DOWN to a multiple of the block, so it stands still while newest
// advances.
//
// A session shorter than the window answers 0, which keeps everything.
func blockAnchor(newestSeq, limit int) int {
	block := historyBlockFor(limit)
	anchor := ((newestSeq - limit + 1) / block) * block
	if anchor < 0 {
		return 0
	}
	return anchor
}

// historyBlockFor is the block for a window: a third of it, capped at historyBlock. It is
// proportional so the overshoot stays a fraction of the window at any limit; at a limit of
// 3 or less it is 1.
func historyBlockFor(limit int) int {
	block := limit / 3
	if block > historyBlock {
		return historyBlock
	}
	if block < 1 {
		return 1
	}
	return block
}

// closeDanglingToolCalls inserts synthetic error tool results for any assistant tool_use not
// followed by its tool_result, keeping replay provider-legal after an interrupted turn.
func closeDanglingToolCalls(msgs []Message) []Message {
	if len(msgs) == 0 {
		return msgs
	}
	out := make([]Message, 0, len(msgs)+4)
	for i := 0; i < len(msgs); i++ {
		msg := msgs[i]
		if msg.Role == RoleTool {
			continue
		}
		if msg.Role == RoleAssistant {
			var content assistantContent
			if json.Unmarshal(msg.Content, &content) == nil && content.Text == "" && len(content.ToolCalls) == 0 {
				continue
			}
		}
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
			id := toolResultCallID(msgs[j])
			for _, call := range calls {
				if call.ID == id && id != "" {
					if _, duplicate := answered[id]; !duplicate {
						answered[id] = struct{}{}
						out = append(out, msgs[j])
					}
					break
				}
			}
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

// fail emits the terminal error event and returns the cause. The event goes out first so a
// streaming client stops waiting.
func (r *Runner) fail(emit func(Event), err error) error {
	emit(Event{Kind: EventResult, StopReason: StopError, IsError: true})
	return err
}

// stream routes the model's answer deltas and its reasoning deltas to their own event
// kinds; the chat shows reasoning in a separate panel.
func stream(emit func(Event)) llm.ChatStream {
	return llm.ChatStream{
		OnText:     func(delta string) { emit(Event{Kind: EventAssistantText, Text: delta}) },
		OnThinking: func(delta string) { emit(Event{Kind: EventAssistantThought, Text: delta}) },
	}
}

// emitUsage reports token counts when the provider surfaced them, via the LLM client's own
// extraction so the stream and tracer agree.
func emitUsage(emit func(Event), choice *llms.ContentChoice) {
	usage := llm.UsageFrom(choice)
	if usage == nil {
		return
	}
	emit(Event{Kind: EventUsage, InputTokens: usage.Input, CachedInputTokens: usage.CachedInput, OutputTokens: usage.Output})
}

// validJSON renders tool arguments for the UI, falling back to a JSON string when the model
// emitted something unparseable; the stream must never carry invalid JSON.
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
