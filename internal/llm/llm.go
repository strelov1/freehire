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
	"strings"
	"time"

	"github.com/tmc/langchaingo/llms"
	"github.com/tmc/langchaingo/llms/openai"
)

// DefaultTimeout bounds a single LLM call. Without it a stalled gateway hangs a
// run-once worker indefinitely, holding its cron flock open and stalling the
// whole queue. The caller's lease/retry machinery then reclaims the work.
const DefaultTimeout = 90 * time.Second

// Client is a thin wrapper over a langchaingo model with a per-call timeout.
type Client struct {
	model   llms.Model
	timeout time.Duration
}

// New builds a Client against an OpenAI-compatible endpoint. baseURL points at
// the gateway/provider, apiKey is the bearer credential, model is the model id.
// No provider is hard-coded — any OpenAI-compatible backend works.
func New(baseURL, apiKey, model string) (*Client, error) {
	m, err := openai.New(
		openai.WithBaseURL(baseURL),
		openai.WithToken(apiKey),
		openai.WithModel(model),
	)
	if err != nil {
		return nil, fmt.Errorf("llm: build client: %w", err)
	}
	return &Client{model: m, timeout: DefaultTimeout}, nil
}

// NewWithModel wraps an already-constructed langchaingo model with the default
// timeout. It is the seam for callers' tests that inject a fake model instead of
// dialing a real endpoint.
func NewWithModel(m llms.Model) *Client {
	return &Client{model: m, timeout: DefaultTimeout}
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
	resp, err := c.model.GenerateContent(ctx, messages, llms.WithJSONMode())
	if err != nil {
		return "", fmt.Errorf("llm: generate: %w", err)
	}
	if len(resp.Choices) == 0 {
		return "", errors.New("llm: model returned no choices")
	}
	return StripJSONFence(resp.Choices[0].Content), nil
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
