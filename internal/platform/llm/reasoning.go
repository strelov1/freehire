package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// ReasoningEffort is how much deliberation a call asks the model for, sent as the
// OpenAI-compatible `reasoning_effort`. Reasoning costs time, unpredictably, and a
// transcription task like CV extraction gains nothing from it.
//
// It is per call, not per client: the assistant reasons, the extractor does not.
type ReasoningEffort string

const (
	// ReasoningDefault sends nothing, leaving the model or gateway default. The zero value.
	ReasoningDefault ReasoningEffort = ""
	// ReasoningNone asks the model not to deliberate. Honoured per provider (zai yes, gemini
	// ignores it), so sending it is harmless everywhere.
	ReasoningNone ReasoningEffort = "none"
)

// WithReasoning sets how much the model should deliberate over this one call.
//
// langchaingo's OpenAI client has a ReasoningEffort field but the code populating it from call
// options is commented out upstream (v0.1.14), so the value rides the request context to the
// transport, which writes it onto the body, like schemaInjector.
func WithReasoning(e ReasoningEffort) GenOption {
	return func(cfg *genConfig) { cfg.reasoning = e }
}

// reasoningKey is the context key for a call's effort.
type reasoningKey struct{}

// withReasoning carries one call's effort to the transport; the default adds nothing.
func withReasoning(ctx context.Context, e ReasoningEffort) context.Context {
	if e == ReasoningDefault {
		return ctx
	}
	return context.WithValue(ctx, reasoningKey{}, e)
}

// reasoningOf reads the effort a call asked for, or the default when it asked for nothing.
func reasoningOf(ctx context.Context) ReasoningEffort {
	e, _ := ctx.Value(reasoningKey{}).(ReasoningEffort)
	return e
}

// reasoningInjector writes `reasoning_effort` onto the outgoing chat request when the call
// asked for one (see WithReasoning). It sits OUTSIDE the attribution stamp, so a credential
// retry re-sends the rewritten body.
type reasoningInjector struct {
	next http.RoundTripper
}

func (t *reasoningInjector) RoundTrip(req *http.Request) (*http.Response, error) {
	next := t.next
	if next == nil {
		next = http.DefaultTransport
	}

	effort := reasoningOf(req.Context())
	if effort == ReasoningDefault || req.Body == nil {
		return next.RoundTrip(req)
	}

	// A RoundTripper must close the request body on every path.
	defer req.Body.Close()

	body, err := io.ReadAll(req.Body)
	if err != nil {
		return nil, fmt.Errorf("llm: read request body: %w", err)
	}

	patched, err := withReasoningEffort(body, effort)
	if err != nil {
		return nil, err
	}

	clone := req.Clone(req.Context())
	clone.Body = io.NopCloser(bytes.NewReader(patched))
	clone.ContentLength = int64(len(patched))
	clone.GetBody = func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(patched)), nil
	}

	return next.RoundTrip(clone)
}

// withReasoningEffort sets the reasoning_effort member of a chat request, leaving the rest as
// written. A body that is not a JSON object is returned unchanged.
func withReasoningEffort(body json.RawMessage, effort ReasoningEffort) (json.RawMessage, error) {
	fields := map[string]json.RawMessage{}
	if err := json.Unmarshal(body, &fields); err != nil {
		return body, nil //nolint:nilerr // not a JSON object: not ours to rewrite
	}
	// A literal `null` unmarshals without error into a nil map, and the write below would
	// panic, with no error path in a RoundTripper.
	if fields == nil {
		return body, nil
	}

	encoded, err := json.Marshal(string(effort))
	if err != nil {
		return nil, fmt.Errorf("llm: encode reasoning effort: %w", err)
	}
	fields["reasoning_effort"] = encoded

	patched, err := json.Marshal(fields)
	if err != nil {
		return nil, fmt.Errorf("llm: encode patched request: %w", err)
	}

	return patched, nil
}
