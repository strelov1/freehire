package llm

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"

	"github.com/tmc/langchaingo/httputil"
	"github.com/tmc/langchaingo/llms"
	"github.com/tmc/langchaingo/llms/openai"

	"github.com/strelov1/freehire/internal/llmschema"
)

// GenOption configures a single JSON generation. Options are variadic so a call site
// that asks for nothing keeps sending exactly what it sent before.
type GenOption func(*genConfig)

type genConfig struct {
	schemaName  string
	schema      llmschema.Schema
	schemaAsked bool
}

func newGenConfig(opts []GenOption) genConfig {
	var cfg genConfig
	for _, o := range opts {
		o(&cfg)
	}

	return cfg
}

// WithSchema constrains the response to schema, sent in strict mode under name. The
// name is the model-facing label for the shape and appears in provider logs, so it
// should read like the contract it came from ("structured_cv", "enrichment").
//
// A nil schema or an empty name is an error at call time rather than a call that
// quietly runs unconstrained: both are the failure this package exists to prevent.
func WithSchema(name string, s llmschema.Schema) GenOption {
	return func(cfg *genConfig) {
		cfg.schemaName = name
		cfg.schema = s
		cfg.schemaAsked = true
	}
}

// validate reports the misconfigurations that would otherwise degrade in silence.
func (cfg genConfig) validate() error {
	if !cfg.schemaAsked {
		return nil
	}
	if cfg.schemaName == "" {
		return errors.New("llm: WithSchema needs a name; most gateways reject an empty one")
	}
	if cfg.schema == nil {
		return errors.New("llm: WithSchema was given a nil schema")
	}

	return nil
}

// modelCache holds the per-schema models behind a pointer, so Client stays copyable —
// WithTimeout clones it, and a mutex embedded by value would be copied with it. A
// clone shares the cache, which is right: it addresses the same endpoint.
type modelCache struct {
	mu     sync.Mutex
	models map[string]llms.Model
}

func newModelCache() *modelCache {
	return &modelCache{models: map[string]llms.Model{}}
}

func (mc *modelCache) get(name string, build func() (llms.Model, error)) (llms.Model, error) {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	if m, ok := mc.models[name]; ok {
		return m, nil
	}

	m, err := build()
	if err != nil {
		return nil, err
	}
	mc.models[name] = m

	return m, nil
}

// modelFor returns the model a call should run against: the plain one when no schema
// was asked for, otherwise one bound to that schema and reused thereafter.
//
// A client built by NewWithModel has no endpoint to rebuild against — that seam exists
// so callers' tests can inject a fake — so a schema there is ignored rather than
// fatal, and the injected model is used as-is.
func (c *Client) modelFor(cfg genConfig) (llms.Model, error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	if cfg.schema == nil || c.baseURL == "" {
		return c.model, nil
	}

	format, err := responseFormat(cfg)
	if err != nil {
		return nil, err
	}

	// Keyed on the name AND the rendered schema: two shapes sharing a name would
	// otherwise silently share the first one's constraint, and a response decoded
	// against the wrong contract is a zero-valued success no one would notice. A
	// multi-stage caller passing one name per stage is exactly that case.
	key := cfg.schemaName + "\x00" + string(format)

	return c.schemaModels.get(key, func() (llms.Model, error) {
		m, err := openai.New(
			openai.WithBaseURL(c.baseURL),
			openai.WithToken(c.apiKey),
			openai.WithModel(c.modelID),
			openai.WithHTTPClient(&http.Client{
				Transport: &schemaInjector{format: format, next: httputil.DefaultTransport},
			}),
		)
		if err != nil {
			return nil, fmt.Errorf("llm: build schema-bound client: %w", err)
		}

		return m, nil
	})
}

// responseFormat renders the strict json_schema response format for cfg.
func responseFormat(cfg genConfig) (json.RawMessage, error) {
	format := map[string]any{
		"type": "json_schema",
		"json_schema": map[string]any{
			"name":   cfg.schemaName,
			"strict": true,
			"schema": cfg.schema,
		},
	}

	raw, err := json.Marshal(format)
	if err != nil {
		return nil, fmt.Errorf("llm: encode response format: %w", err)
	}

	return raw, nil
}

// schemaInjector rewrites `response_format` in the outgoing chat request.
//
// langchaingo's response-format type cannot express what strict mode needs: its
// property Type is a plain string, while an optional field must be typed
// ["string", "null"] — and without that the model has no legal way to decline a
// required field, so it invents a value instead (measured: 3 runs in 5 asserted a
// visa policy for an ad that stated none). Everything else on the request is
// langchaingo's and passes through untouched.
type schemaInjector struct {
	// next is langchaingo's own transport, which stamps the library's User-Agent.
	// Replacing the whole client would otherwise make schema-bound traffic
	// indistinguishable from any other Go program at the gateway.
	next   http.RoundTripper
	format json.RawMessage
}

func (t *schemaInjector) RoundTrip(req *http.Request) (*http.Response, error) {
	next := t.next
	if next == nil {
		next = http.DefaultTransport
	}

	if req.Body == nil {
		return next.RoundTrip(req)
	}

	// A RoundTripper must close the request body on every path, not only the one
	// that reaches the wire.
	defer req.Body.Close()

	body, err := io.ReadAll(req.Body)
	if err != nil {
		return nil, fmt.Errorf("llm: read request body: %w", err)
	}

	patched, err := withResponseFormat(body, t.format)
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

// withResponseFormat replaces the response_format member of a chat request, leaving
// every other member as langchaingo wrote it. A body that is not a JSON object is
// returned unchanged — the transport is installed on one client, but nothing
// guarantees only chat requests reach it.
func withResponseFormat(body, format json.RawMessage) (json.RawMessage, error) {
	fields := map[string]json.RawMessage{}
	if err := json.Unmarshal(body, &fields); err != nil {
		return body, nil //nolint:nilerr // not a JSON object: not ours to rewrite
	}

	fields["response_format"] = format

	patched, err := json.Marshal(fields)
	if err != nil {
		return nil, fmt.Errorf("llm: encode patched request: %w", err)
	}

	return patched, nil
}
