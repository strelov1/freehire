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
// OpenAI-compatible `reasoning_effort`.
//
// It exists because deliberation is not free and is not always worth its price. Measured on
// production 2026-09-08, structured-résumé extraction — a transcription task, not a
// judgement — spent between a third and two thirds of every response on reasoning tokens
// that changed nothing: over four paired runs, on a clean CV and on a deliberately
// column-mangled one, both settings recovered the same 10 of 10 employments, the same
// titles, dates, skills and education, and invented nothing extra. What the reasoning DID
// buy was time, and unpredictably: the same input answered in 22.6s and 47.2s on one
// provider, against a steady 15-20s with it off. That tail is what crossed the extraction's
// timeout and left 191 candidates with an unparsed CV, no profile links, and no fit
// analysis.
//
// It is per CALL and not per client on purpose. The assistant reasons for a living; the
// extractor does not. One client serves both.
type ReasoningEffort string

const (
	// ReasoningDefault sends nothing, leaving whatever the model or the gateway would do.
	// The zero value, so a caller that says nothing keeps sending exactly what it sent.
	ReasoningDefault ReasoningEffort = ""
	// ReasoningNone asks the model not to deliberate.
	//
	// Honoured per provider, not universally: on the gateway's zai models it takes the
	// reasoning tokens from ~1500 to under 30, while the gemini ones ignore it and answer
	// fast regardless. Sending it is therefore right on both — it helps where the cost is
	// and is inert where it is not.
	ReasoningNone ReasoningEffort = "none"
)

// WithReasoning sets how much the model should deliberate over this one call.
//
// The field cannot travel through langchaingo. Its OpenAI client HAS a ReasoningEffort
// field, but the code that would populate it from call options is commented out upstream
// (v0.1.14 — the current release, not a version we are behind on), so nothing a caller
// passes reaches the wire. The value therefore rides the request context down to the
// transport, which writes it onto the body — the same seam, and for the same reason, as
// schemaInjector next to it.
func WithReasoning(e ReasoningEffort) GenOption {
	return func(cfg *genConfig) { cfg.reasoning = e }
}

// reasoningKey addresses the effort a single call asked for. A private type, so nothing
// outside this package can set or read it by guessing the key.
type reasoningKey struct{}

// withReasoning carries one call's effort to the transport. A default effort adds nothing
// to the context, so an untouched call is untouched all the way down.
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
// asked for one, and is a pass-through otherwise.
//
// On the transport rather than in the request builder because langchaingo owns that builder
// and cannot express the field (see WithReasoning). It sits OUTSIDE the attribution stamp,
// so the body is rewritten once and a credential retry re-sends the rewritten one.
type reasoningInjector struct {
	// next is whatever this client already travels on — langchaingo's own transport, or
	// the attribution stamp wrapping it.
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

	// A RoundTripper must close the request body on every path, not only the one that
	// reaches the wire.
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

// withReasoningEffort sets the reasoning_effort member of a chat request, leaving every
// other member as langchaingo wrote it. A body that is not a JSON object is returned
// unchanged — the transport is installed on every client, and nothing guarantees only chat
// requests reach it.
func withReasoningEffort(body json.RawMessage, effort ReasoningEffort) (json.RawMessage, error) {
	fields := map[string]json.RawMessage{}
	if err := json.Unmarshal(body, &fields); err != nil {
		return body, nil //nolint:nilerr // not a JSON object: not ours to rewrite
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
