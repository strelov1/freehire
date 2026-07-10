// Package llm wraps an OpenAI-compatible chat endpoint behind a tiny, provider-
// agnostic surface shared by the enrichment and telegram-extraction workers. It
// owns only the langchaingo client construction, the per-call timeout + generate
// + empty-choices guard, and the markdown-fence cleanup some models add despite
// JSON mode. Callers keep their own prompts and typed-contract parsing.
package llm

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/tmc/langchaingo/llms"
	"github.com/tmc/langchaingo/llms/openai"
)

// DefaultTimeout bounds a single LLM call. Without it a stalled gateway hangs a
// run-once worker indefinitely, holding its cron flock open and stalling the
// whole queue. The caller's lease/retry machinery then reclaims the work.
const DefaultTimeout = 90 * time.Second

// tracerShutdownTimeout bounds the final trace flush so a stuck Langfuse endpoint
// cannot hold a shutting-down process open.
const tracerShutdownTimeout = 15 * time.Second

// Client is a thin wrapper over a langchaingo model with a per-call timeout. An
// optional tracer observes each call; modelID and source label those observations.
type Client struct {
	model   llms.Model
	modelID string
	timeout time.Duration
	tracer  Tracer
	source  string
}

// ModelID returns the configured model id, so a caller can record which model
// produced a result (e.g. a cached analysis's provenance). Empty on a nil client.
func (c *Client) ModelID() string {
	if c == nil {
		return ""
	}
	return c.modelID
}

// Option configures a Client at construction. Options keep tracing opt-in without
// changing the constructors' required parameters, so existing call sites compile
// unchanged.
type Option func(*Client)

// WithTracer attaches a tracer and the source label recorded on each observation.
// A nil tracer is fine — the client simply performs no tracing.
func WithTracer(t Tracer, source string) Option {
	return func(c *Client) {
		c.tracer = t
		c.source = source
	}
}

// New builds a Client against an OpenAI-compatible endpoint. baseURL points at
// the gateway/provider, apiKey is the bearer credential, model is the model id.
// No provider is hard-coded — any OpenAI-compatible backend works.
func New(baseURL, apiKey, model string, opts ...Option) (*Client, error) {
	m, err := openai.New(
		openai.WithBaseURL(baseURL),
		openai.WithToken(apiKey),
		openai.WithModel(model),
	)
	if err != nil {
		return nil, fmt.Errorf("llm: build client: %w", err)
	}
	c := &Client{model: m, modelID: model, timeout: DefaultTimeout}
	for _, o := range opts {
		o(c)
	}
	return c, nil
}

// Settings is the full configuration to build a (optionally traced) LLM client: the
// OpenAI-compatible endpoint plus optional Langfuse credentials. It is the one shape
// every entrypoint (the HTTP server and both LLM workers) maps its env config into,
// so client construction and tracing live in exactly one place.
type Settings struct {
	BaseURL string
	APIKey  string
	Model   string

	// Langfuse tracing is optional: all three set ⇒ every call is traced under the
	// caller's source label; otherwise the client runs untraced.
	LangfuseBaseURL   string
	LangfusePublicKey string
	LangfuseSecretKey string
}

// Enabled reports whether an LLM client can be built (all three core fields set).
func (s Settings) Enabled() bool {
	return s.BaseURL != "" && s.APIKey != "" && s.Model != ""
}

// NewClient is the single construction path for an LLM client. It builds the client
// from s and, when the Langfuse fields are set, attaches a tracer labelled source
// ("verdict"/"enrich"/"telegram"). It returns a flush func that drains buffered
// traces — always defer it (a no-op when tracing is off). When the LLM is
// unconfigured (!s.Enabled()) it returns (nil, no-op, nil) so callers degrade
// uniformly. Because every caller goes through here, no entrypoint can build a
// client and forget to wire tracing.
func NewClient(s Settings, source string) (*Client, func(), error) {
	noop := func() {}
	if !s.Enabled() {
		return nil, noop, nil
	}
	var opts []Option
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

// NewWithModel wraps an already-constructed langchaingo model with the default
// timeout. It is the seam for callers' tests that inject a fake model instead of
// dialing a real endpoint.
func NewWithModel(m llms.Model, opts ...Option) *Client {
	c := &Client{model: m, timeout: DefaultTimeout}
	for _, o := range opts {
		o(c)
	}
	return c
}

// GenerateJSON sends a system+user prompt in JSON mode and returns the model's
// raw response content with any markdown code fence stripped. The call is bounded
// by the client timeout. The returned string is unparsed — the caller unmarshals
// it into its own contract type.
func (c *Client) GenerateJSON(ctx context.Context, system, user string) (string, error) {
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
	// gen builds an observation with the fields common to every outcome, stamping
	// the end time at call. The caller fills in output/usage or the error.
	gen := func() Generation {
		return Generation{Model: c.modelID, System: system, User: user, Start: start, End: time.Now(), Source: c.source}
	}

	resp, err := c.model.GenerateContent(ctx, messages, llms.WithJSONMode())
	if err != nil {
		wrapped := fmt.Errorf("llm: generate: %w", err)
		g := gen()
		g.Err = wrapped
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
	g.Usage = usageFrom(resp.Choices[0])
	c.observe(g)
	return out, nil
}

// observe reports a generation when a tracer is attached; a nil tracer makes this
// a no-op, so an unconfigured client is unchanged.
func (c *Client) observe(g Generation) {
	if c.tracer == nil {
		return
	}
	c.tracer.Observe(g)
}

// usageFrom pulls token counts out of langchaingo's per-choice GenerationInfo.
// Providers vary, so it reads defensively and returns nil when no counts are
// present — an absent usage is reported as absent, never as zeros.
func usageFrom(choice *llms.ContentChoice) *Usage {
	in, ok1 := intFrom(choice.GenerationInfo["PromptTokens"])
	out, ok2 := intFrom(choice.GenerationInfo["CompletionTokens"])
	total, ok3 := intFrom(choice.GenerationInfo["TotalTokens"])
	if !ok1 && !ok2 && !ok3 {
		return nil
	}
	return &Usage{Input: in, Output: out, Total: total}
}

// intFrom coerces a GenerationInfo value (int, int64, or float64 depending on the
// provider/serialization) to an int.
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

// StripJSONFence trims surrounding whitespace and a leading/trailing markdown
// code fence (```json … ```) some models add even in JSON mode.
func StripJSONFence(raw string) string {
	s := strings.TrimSpace(raw)
	s = strings.TrimPrefix(s, "```json")
	s = strings.TrimPrefix(s, "```")
	s = strings.TrimSuffix(s, "```")
	return strings.TrimSpace(s)
}

// TruncateRunes returns s clamped to at most limit runes, never splitting a rune.
func TruncateRunes(s string, limit int) string {
	r := []rune(s)
	if len(r) <= limit {
		return s
	}
	return string(r[:limit])
}
