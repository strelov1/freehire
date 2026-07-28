package assistant

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/tmc/langchaingo/llms"
)

// Tool is one capability offered to the model. Schema is the JSON Schema of its
// argument object, rendered into the model's tool list; Run executes it as the
// authenticated caller — every tool receives the owner's user id rather than a
// credential, which is what makes the in-process agent unable to act as anyone
// else.
type Tool struct {
	Name        string
	Description string
	Schema      map[string]any
	Run         func(ctx context.Context, userID int64, args json.RawMessage) (any, error)
}

// Result is one tool's answer as the model will see it. Content is always valid
// JSON — either the tool's payload or an error envelope — because a tool failure
// is information the model can act on, not a reason to abandon the turn. Failed
// says which of the two it is, so the UI can render a failed call differently.
type Result struct {
	Content string
	Failed  bool
}

// Registry is the set of tools a session offers, in a fixed order. A session's
// preset builds one; the loop asks it for definitions and dispatches calls to it.
type Registry struct {
	order []string
	tools map[string]Tool

	// MaxResultBytes caps how much of a tool's payload enters the conversation.
	// One search over full descriptions can otherwise fill the context window on
	// its own, and everything after it — including the user's question — falls out.
	// Zero means no cap.
	MaxResultBytes int
}

// NewRegistry registers the tools in the given order. A later tool with the same
// name replaces an earlier one, so a preset can override a default.
func NewRegistry(tools ...Tool) *Registry {
	r := &Registry{tools: make(map[string]Tool, len(tools))}
	for _, t := range tools {
		if _, seen := r.tools[t.Name]; !seen {
			r.order = append(r.order, t.Name)
		}
		r.tools[t.Name] = t
	}
	return r
}

// Names lists the registered tool names in order.
func (r *Registry) Names() []string { return r.order }

// Definitions renders the registry as the model-facing tool list.
func (r *Registry) Definitions() []llms.Tool {
	out := make([]llms.Tool, 0, len(r.order))
	for _, name := range r.order {
		t := r.tools[name]
		out = append(out, llms.Tool{
			Type: "function",
			Function: &llms.FunctionDefinition{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  t.Schema,
			},
		})
	}
	return out
}

// Call runs one tool call and renders its outcome for the model. It never returns
// a Go error: an unknown tool, malformed arguments and a failing tool all come
// back as a failed Result, so the model can correct itself inside the same turn.
// Only the loop's step cap bounds how long it may keep trying.
func (r *Registry) Call(ctx context.Context, userID int64, name string, args json.RawMessage) Result {
	t, ok := r.tools[name]
	if !ok {
		return failed(fmt.Sprintf("unknown tool %q — call one of the tools you were given", name))
	}
	out, err := t.Run(ctx, userID, args)
	if err != nil {
		return failed(err.Error())
	}
	payload, err := json.Marshal(out)
	if err != nil {
		return failed(fmt.Sprintf("tool %q produced a result that could not be encoded: %v", name, err))
	}
	return Result{Content: r.capped(string(payload))}
}

// capped bounds one payload. An oversized result is replaced by an envelope
// carrying its opening bytes and saying it was cut, rather than by a raw prefix:
// a truncated JSON document is not JSON, and a model handed one either reports a
// parse failure or invents the rest. Error envelopes never reach here — a
// truncated correction is worse than none.
func (r *Registry) capped(payload string) string {
	if r.MaxResultBytes <= 0 || len(payload) <= r.MaxResultBytes {
		return payload
	}
	envelope, err := json.Marshal(map[string]any{
		"truncated": true,
		"note": fmt.Sprintf("result was %d bytes, truncated to %d — narrow the call (fewer results, a tighter filter) to see the rest",
			len(payload), r.MaxResultBytes),
		"partial": payload[:r.MaxResultBytes],
	})
	if err != nil {
		return `{"truncated":true,"note":"result too large"}`
	}
	return string(envelope)
}

// failed renders an error as the JSON envelope the model reads.
func failed(msg string) Result {
	payload, err := json.Marshal(map[string]string{"error": msg})
	if err != nil {
		// A map[string]string always marshals; this is unreachable in practice and
		// still must not produce invalid JSON in the conversation.
		return Result{Content: `{"error":"tool failed"}`, Failed: true}
	}
	return Result{Content: string(payload), Failed: true}
}

// DecodeArgs strictly decodes a tool call's arguments into dst. Unknown fields,
// type mismatches and trailing content are rejected, so a mis-addressed call
// fails with a reason the model can act on instead of silently running with
// half the arguments dropped. Empty arguments decode as an empty object, which
// is what models send for a no-argument tool.
func DecodeArgs(args json.RawMessage, dst any) error {
	raw := bytes.TrimSpace(args)
	if len(raw) == 0 {
		raw = []byte("{}")
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return fmt.Errorf("invalid arguments: %w", err)
	}
	// A second value means the model concatenated objects; decoding only the first
	// would run the call with arguments it did not intend.
	if _, err := dec.Token(); err != io.EOF {
		return fmt.Errorf("invalid arguments: expected a single JSON object")
	}
	return nil
}
