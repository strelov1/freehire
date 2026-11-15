// Package llm wraps an OpenAI-compatible chat endpoint behind a provider-agnostic surface.
// It owns langchaingo client construction, the per-call timeout, the empty-choices guard and
// markdown-fence cleanup. Callers keep their own prompts and contract parsing.
//
// A call may constrain its response to a JSON schema (WithSchema; internal/platform/llmschema
// derives one from a contract type). That is a first line, not proof: a gateway that stops
// honouring a schema still answers 200, so caller-side validators stay.
package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/tmc/langchaingo/llms"
	"github.com/tmc/langchaingo/llms/openai"
)

// DefaultTimeout bounds a single LLM call; without it a stalled gateway hangs a run-once
// worker and holds its cron flock.
const DefaultTimeout = 90 * time.Second

// tracerShutdownTimeout bounds the final trace flush so a stuck Langfuse endpoint
// cannot hold a shutting-down process open.
const tracerShutdownTimeout = 15 * time.Second

// Client is a thin wrapper over a langchaingo model with a per-call timeout. An
// optional tracer observes each call; modelID and source label those observations.
type Client struct {
	model       llms.Model
	modelID     string
	timeout     time.Duration
	tracer      Tracer
	source      string
	temperature *float64

	// baseURL and apiKey let a schema-bound model be built on the same endpoint (langchaingo
	// binds the response format to the model, one schema each). Empty under NewWithModel.
	baseURL string
	apiKey  string

	// schemaMode is how this gateway wants a schema asked for (empty is strict). It describes
	// the endpoint, and is part of the schema-bound model's cache key via the rendered format.
	schemaMode SchemaMode

	// dims label each call for the gateway's spend rollup, one header per dimension. Set by As.
	dims []Dimension

	// fallbackKey and onRefused belong to a per-user client: the credential to retry on when the
	// gateway no longer knows the user's, and how to report that. Empty otherwise.
	fallbackKey string
	onRefused   func()

	// schemaModels is a pointer so cloning the Client does not copy its mutex. As must not
	// share it; see the comment there.
	schemaModels *modelCache
}

// ModelID returns the configured model id. Empty on a nil client.
func (c *Client) ModelID() string {
	if c == nil {
		return ""
	}
	return c.modelID
}

// Timeout reports the per-call bound this client was built with, so a caller can derive a
// variant from it instead of restating the number. Zero on a nil client.
func (c *Client) Timeout() time.Duration {
	if c == nil {
		return 0
	}
	return c.timeout
}

// temperatureOptions is the call option a configured temperature adds. Nil adds none, which is
// NOT the same as omitting the field: langchaingo (v0.1.15) serialises its zero value, so a
// non-reasoning model is sent "temperature":0 unless this sets one.
func (c *Client) temperatureOptions() []llms.CallOption {
	if c.temperature == nil {
		return nil
	}
	return []llms.CallOption{llms.WithTemperature(*c.temperature)}
}

// WithoutTracing returns a client that does not send request or response content
// to the optional generation tracer. Use it for ephemeral inputs.
func (c *Client) WithoutTracing() *Client {
	if c == nil {
		return nil
	}
	clone := *c
	clone.tracer = nil
	return &clone
}

// WithTimeout returns a shallow copy of the client with a different per-call timeout. Nil-safe.
func (c *Client) WithTimeout(d time.Duration) *Client {
	if c == nil {
		return nil
	}
	clone := *c
	clone.timeout = d
	return &clone
}

// Option configures a Client at construction.
type Option func(*Client)

// WithSchemaMode sets how WithSchema asks for its shape on this endpoint; zero is strict.
func WithSchemaMode(mode SchemaMode) Option {
	return func(c *Client) {
		c.schemaMode = mode
	}
}

// WithTemperature sets an optional sampling temperature on every call made by this client.
func WithTemperature(temperature float64) Option {
	return func(c *Client) {
		value := temperature
		c.temperature = &value
	}
}

// WithTracer attaches a tracer and the source label recorded on each observation; nil is fine.
func WithTracer(t Tracer, source string) Option {
	return func(c *Client) {
		c.tracer = t
		c.source = source
	}
}

// New builds a Client against an OpenAI-compatible endpoint (baseURL, bearer apiKey, model id).
func New(baseURL, apiKey, model string, opts ...Option) (*Client, error) {
	c := &Client{
		modelID:      model,
		timeout:      DefaultTimeout,
		baseURL:      baseURL,
		apiKey:       apiKey,
		schemaModels: newModelCache(),
	}
	for _, o := range opts {
		o(c)
	}
	// Built after the options: the model travels on the client's own transport.
	m, err := openai.New(
		openai.WithBaseURL(baseURL),
		openai.WithToken(apiKey),
		openai.WithModel(model),
		openai.WithHTTPClient(&http.Client{Transport: c.transport()}),
	)
	if err != nil {
		return nil, fmt.Errorf("llm: build client: %w", err)
	}
	c.model = m

	return c, nil
}

// Settings is the configuration to build a (optionally traced) LLM client: the endpoint plus
// optional Langfuse credentials. Every entrypoint maps its env config into it.
type Settings struct {
	BaseURL     string
	APIKey      string
	Model       string
	Temperature *float64

	// SchemaMode is how WithSchema asks for its shape; zero is strict.
	SchemaMode SchemaMode

	// Langfuse tracing is optional: with all three set every call is traced, else untraced.
	LangfuseBaseURL   string
	LangfusePublicKey string
	LangfuseSecretKey string
}

// Enabled reports whether an LLM client can be built (all three core fields set).
func (s Settings) Enabled() bool {
	return s.BaseURL != "" && s.APIKey != "" && s.Model != ""
}

// NewClient is the single construction path for an LLM client: it builds it from s and, when
// the Langfuse fields are set, attaches a tracer labelled source. Always defer the returned
// flush func (a no-op when untraced). An unconfigured LLM (!s.Enabled()) returns (nil, no-op, nil).
func NewClient(s Settings, source string) (*Client, func(), error) {
	noop := func() {}
	if !s.Enabled() {
		return nil, noop, nil
	}
	if s.Temperature != nil && (math.IsNaN(*s.Temperature) || math.IsInf(*s.Temperature, 0) || *s.Temperature < 0 || *s.Temperature > 2) {
		return nil, noop, fmt.Errorf("llm: temperature must be a number between 0 and 2")
	}
	opts := []Option{WithSchemaMode(s.SchemaMode)}
	if s.Temperature != nil {
		opts = append(opts, WithTemperature(*s.Temperature))
	}
	flush := noop
	if tracer := NewTracer(s.LangfuseBaseURL, s.LangfusePublicKey, s.LangfuseSecretKey); tracer != nil {
		opts = append(opts, WithTracer(tracer, source))
		flush = func() {
			ctx, cancel := context.WithTimeout(context.Background(), tracerShutdownTimeout)
			defer cancel()
			if err := tracer.Shutdown(ctx); err != nil {
				log.Printf("llm: tracer shutdown: %v", err)
			}
		}
	}
	c, err := New(s.BaseURL, s.APIKey, s.Model, opts...)
	if err != nil {
		flush() // don't leak the tracer goroutine when the client fails to build
		return nil, noop, err
	}
	return c, flush, nil
}

// NewWithModel wraps an already-constructed langchaingo model with the default timeout; the
// seam for tests that inject a fake model.
func NewWithModel(m llms.Model, opts ...Option) *Client {
	c := &Client{model: m, timeout: DefaultTimeout, schemaModels: newModelCache()}
	for _, o := range opts {
		o(c)
	}
	return c
}

// GenerateJSON sends a system+user prompt in JSON mode and returns the raw response content
// with any markdown fence stripped, bounded by the client timeout. The caller unmarshals it.
// Given WithSchema the response is constrained to that schema, which is not a guarantee.
func (c *Client) GenerateJSON(ctx context.Context, system, user string, opts ...GenOption) (string, error) {
	cfg := newGenConfig(opts)
	model, err := c.modelFor(cfg)
	if err != nil {
		return "", err
	}
	if system, err = c.systemFor(cfg, system); err != nil {
		return "", err
	}
	// Reasoning effort rides the context to the transport; see WithReasoning.
	ctx = withReasoning(ctx, cfg.reasoning)

	if c.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, c.timeout)
		defer cancel()
	}
	messages := []llms.MessageContent{
		llms.TextParts(llms.ChatMessageTypeSystem, system),
		llms.TextParts(llms.ChatMessageTypeHuman, user),
	}

	start := time.Now()
	// gen builds an observation with the common fields, stamping End at call.
	gen := func() Generation {
		return Generation{Model: c.modelID, System: system, User: user, Start: start, End: time.Now(), Source: c.source}
	}

	callOpts := append([]llms.CallOption{llms.WithJSONMode()}, c.temperatureOptions()...)
	resp, err := model.GenerateContent(ctx, messages, callOpts...)
	if err != nil {
		wrapped := fmt.Errorf("llm: generate: %w", err)
		g := gen()
		g.Err = wrapped
		g.Cancelled = callerCancelled(ctx)
		c.observe(g)
		return "", wrapped
	}
	if len(resp.Choices) == 0 {
		err := errors.New("llm: model returned no choices")
		g := gen()
		g.Err = err
		c.observe(g)
		return "", err
	}

	out := StripJSONFence(resp.Choices[0].Content)
	g := gen()
	g.Output = out
	g.Usage = UsageFrom(resp.Choices[0])
	c.observe(g)
	return out, nil
}

// contextOverrules replaces a provider's claim of success with our own context's error when
// that context had already expired. A gateway that stops emitting at our deadline can return
// partial output as success; for Chat that is half a sentence stored as the finished answer,
// and tool calls run with `{}` arguments from healToolArguments. Our context decides.
func contextOverrules(ctx context.Context, err error) error {
	if err == nil && ctx.Err() != nil {
		return ctx.Err()
	}
	return err
}

// callerCancelled reports whether a failed call was abandoned by our own caller rather than
// failed by the provider. Only a cancelled parent yields Canceled; the call's own deadline
// yields DeadlineExceeded. The error cannot tell: langchaingo replaces context.Canceled with a
// bare "request cancelled".
func callerCancelled(ctx context.Context) bool {
	return errors.Is(ctx.Err(), context.Canceled)
}

// GenerateJSONStream is GenerateJSON with a streamed response. Content accumulates into the
// returned JSON; reasoning tokens go to onThinking as they arrive, if the provider surfaces
// any. Partial JSON is never surfaced.
func (c *Client) GenerateJSONStream(
	ctx context.Context,
	system, user string,
	onThinking func(string),
	opts ...GenOption,
) (string, error) {
	cfg := newGenConfig(opts)
	model, err := c.modelFor(cfg)
	if err != nil {
		return "", err
	}
	if system, err = c.systemFor(cfg, system); err != nil {
		return "", err
	}
	// As in GenerateJSON, the effort rides the context to the transport.
	ctx = withReasoning(ctx, cfg.reasoning)

	if c.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, c.timeout)
		defer cancel()
	}
	messages := []llms.MessageContent{
		llms.TextParts(llms.ChatMessageTypeSystem, system),
		llms.TextParts(llms.ChatMessageTypeHuman, user),
	}

	start := time.Now()
	gen := func() Generation {
		return Generation{Model: c.modelID, System: system, User: user, Start: start, End: time.Now(), Source: c.source}
	}

	// Content deltas are ignored; the full content is read from the final response.
	stream := llms.WithStreamingReasoningFunc(func(_ context.Context, reasoningChunk, _ []byte) error {
		if onThinking != nil && len(reasoningChunk) > 0 {
			onThinking(string(reasoningChunk))
		}
		return nil
	})

	callOpts := append([]llms.CallOption{llms.WithJSONMode(), stream}, c.temperatureOptions()...)
	resp, err := model.GenerateContent(ctx, messages, callOpts...)
	err = contextOverrules(ctx, err)
	// Log the duration so slow stages stay observable without a tracer.
	log.Printf("llm: stream model=%s dur=%s err=%v", c.modelID, time.Since(start).Round(time.Millisecond), err)
	if err != nil {
		wrapped := fmt.Errorf("llm: generate stream: %w", err)
		g := gen()
		g.Err = wrapped
		g.Cancelled = callerCancelled(ctx)
		c.observe(g)
		return "", wrapped
	}
	if len(resp.Choices) == 0 {
		err := errors.New("llm: model returned no choices")
		g := gen()
		g.Err = err
		c.observe(g)
		return "", err
	}

	out := StripJSONFence(resp.Choices[0].Content)
	g := gen()
	g.Output = out
	g.Usage = UsageFrom(resp.Choices[0])
	c.observe(g)
	return out, nil
}

// observe reports a generation when a tracer is attached.
func (c *Client) observe(g Generation) {
	if c.tracer == nil {
		return
	}
	c.tracer.Observe(g)
}

// UsageFrom pulls token counts out of langchaingo's per-choice GenerationInfo, defensively.
// It returns nil when no counts are present, never zeros. Exported so streaming callers
// report the same counts the tracer records.
func UsageFrom(choice *llms.ContentChoice) *Usage {
	in, ok1 := intFrom(choice.GenerationInfo["PromptTokens"])
	out, ok2 := intFrom(choice.GenerationInfo["CompletionTokens"])
	total, ok3 := intFrom(choice.GenerationInfo["TotalTokens"])
	if !ok1 && !ok2 && !ok3 {
		return nil
	}
	// The cached count alone never makes a usage: a zero Input would read as a free request.
	cached, _ := intFrom(choice.GenerationInfo["PromptCachedTokens"])

	return &Usage{Input: in, Output: out, CachedInput: cached, Total: total}
}

// intFrom coerces a GenerationInfo value (int, int64 or float64, by provider) to an int.
func intFrom(v any) (int, bool) {
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

// StripJSONFence recovers a single JSON value from model output wrapped in a markdown fence,
// with stray trailing backticks, or after a short preamble.
func StripJSONFence(raw string) string {
	s := strings.TrimSpace(raw)
	if strings.HasPrefix(s, "```") {
		if nl := strings.IndexByte(s, '\n'); nl >= 0 {
			s = s[nl+1:]
		} else {
			s = strings.TrimPrefix(s, "```")
		}
		s = strings.TrimSpace(s)
	}
	if i := strings.LastIndex(s, "\n```"); i >= 0 {
		s = strings.TrimSpace(s[:i])
	} else {
		s = strings.TrimSpace(strings.TrimSuffix(s, "```"))
	}
	if cut := firstJSONValue(s); cut != "" {
		return cut
	}
	return s
}

// firstJSONValue returns the first top-level JSON object or array in s, ignoring
// any trailing junk a model left after it. Empty when none decodes.
func firstJSONValue(s string) string {
	start := strings.IndexAny(s, "{[")
	if start < 0 {
		return ""
	}
	dec := json.NewDecoder(strings.NewReader(s[start:]))
	var raw json.RawMessage
	if err := dec.Decode(&raw); err != nil {
		return ""
	}
	return string(raw)
}

// TruncateRunes returns s clamped to at most limit runes, never splitting a rune.
func TruncateRunes(s string, limit int) string {
	r := []rune(s)
	if len(r) <= limit {
		return s
	}
	return string(r[:limit])
}

// ClipRunesEllipsis clamps s to at most limit runes and, if it cut, trims the end and appends
// "…". Text that fits is returned untouched.
func ClipRunesEllipsis(s string, limit int) string {
	r := []rune(s)
	if len(r) <= limit {
		return s
	}
	return strings.TrimSpace(string(r[:limit])) + "…"
}

// TrimTruncateRunes trims s, clamps it to at most limit runes, and trims again so a cut never
// leaves trailing whitespace. Use it to bound untrusted or model text.
func TrimTruncateRunes(s string, limit int) string {
	return strings.TrimSpace(TruncateRunes(strings.TrimSpace(s), limit))
}

// Limit returns at most n elements of s (nil-safe, preserves order).
func Limit[T any](s []T, n int) []T {
	if len(s) > n {
		return s[:n]
	}
	return s
}

// NonEmpty drops blank strings, preserving order; returns nil when nothing remains.
func NonEmpty(in []string) []string {
	var out []string
	for _, v := range in {
		if v != "" {
			out = append(out, v)
		}
	}
	return out
}
