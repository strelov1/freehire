package llm

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"

	"github.com/tmc/langchaingo/llms"
	"github.com/tmc/langchaingo/llms/openai"

	"github.com/strelov1/freehire/internal/platform/llmschema"
)

// SchemaMode selects how a schema is asked for on the wire. It is a property of the gateway,
// so it lives on Settings, not on a GenOption.
type SchemaMode string

const (
	// SchemaModeStrict sends `response_format: json_schema` with strict: true. The zero value.
	SchemaModeStrict SchemaMode = ""

	// SchemaModeJSONObject sends `response_format: json_object` and no schema, for a gateway
	// whose model answers strict schemas worse than none (z.ai glm-4.7-flash invented postings).
	// The schema is appended to the system prompt (systemFor) and still checked by the
	// caller's validator; only the provider-side guarantee is lost.
	SchemaModeJSONObject SchemaMode = "json_object"
)

// GenOption configures a single JSON generation.
type GenOption func(*genConfig)

type genConfig struct {
	schemaName  string
	schema      llmschema.Schema
	schemaAsked bool
	// reasoning: see WithReasoning.
	reasoning ReasoningEffort
}

func newGenConfig(opts []GenOption) genConfig {
	var cfg genConfig
	for _, o := range opts {
		o(&cfg)
	}

	return cfg
}

// WithSchema constrains the response to schema, sent in strict mode under name. The name is
// the model-facing label and appears in provider logs ("structured_cv", "enrichment").
//
// A nil schema or empty name is an error at call time, not a silently unconstrained call.
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

// modelCache holds the per-schema models behind a pointer so Client stays copyable
// (WithTimeout clones share it, which is right: same endpoint, credential and headers).
//
// As must NOT share it. The key is the schema name and rendered shape, not the credential or
// tags, so a shared cache would send one user's call on another's key, or a tagged call
// untagged, invisibly. TestAsDoesNotShareSchemaBoundModelsAcrossCredentials guards it.
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

// modelFor returns the plain model when no schema was asked for, otherwise one bound to that
// schema and cached. A NewWithModel client has no endpoint to rebuild against, so a schema
// there is ignored and the injected model used as-is.
func (c *Client) modelFor(cfg genConfig) (llms.Model, error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	if cfg.schema == nil || c.baseURL == "" {
		return c.model, nil
	}

	format, err := responseFormat(cfg, c.schemaMode)
	if err != nil {
		return nil, err
	}

	// Keyed on the name AND the rendered schema: two shapes sharing a name would silently share
	// the first one's constraint.
	key := cfg.schemaName + "\x00" + string(format)

	return c.schemaModels.get(key, func() (llms.Model, error) {
		m, err := openai.New(
			openai.WithBaseURL(c.baseURL),
			openai.WithToken(c.apiKey),
			openai.WithModel(c.modelID),
			// On top of the client's own transport so schema-bound calls keep their tags.
			openai.WithHTTPClient(&http.Client{
				Transport: &schemaInjector{format: format, next: c.transport()},
			}),
		)
		if err != nil {
			return nil, fmt.Errorf("llm: build schema-bound client: %w", err)
		}

		return m, nil
	})
}

// responseFormat renders the strict json_schema response format for cfg.
func responseFormat(cfg genConfig, mode SchemaMode) (json.RawMessage, error) {
	format := map[string]any{
		"type": "json_schema",
		"json_schema": map[string]any{
			"name":   cfg.schemaName,
			"strict": true,
			"schema": cfg.schema,
		},
	}
	if mode == SchemaModeJSONObject {
		format = map[string]any{"type": "json_object"}
	}

	raw, err := json.Marshal(format)
	if err != nil {
		return nil, fmt.Errorf("llm: encode response format: %w", err)
	}

	return raw, nil
}

// systemFor returns the system prompt a call should send. In json_object mode the schema is
// not on the wire as response_format, so it rides on the system prompt: the model needs the
// field names, and OpenAI-compatible providers reject json_object unless a message says "json".
func (c *Client) systemFor(cfg genConfig, system string) (string, error) {
	if c.schemaMode != SchemaModeJSONObject || cfg.schema == nil {
		return system, nil
	}
	raw, err := json.Marshal(cfg.schema)
	if err != nil {
		return "", fmt.Errorf("llm: encode schema for prompt: %w", err)
	}
	return system + "\n\nRespond with a single JSON object that matches this JSON Schema:\n" + string(raw), nil
}

// schemaInjector rewrites `response_format` in the outgoing chat request. langchaingo's type
// cannot express strict mode: an optional field must be typed ["string", "null"], or the model
// has no legal way to decline it and invents a value.
type schemaInjector struct {
	// next is langchaingo's own transport, which stamps the User-Agent.
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

	// A RoundTripper must close the request body on every path.
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

// withResponseFormat replaces the response_format member of a chat request, leaving the rest
// as written. A body that is not a JSON object is returned unchanged.
func withResponseFormat(body, format json.RawMessage) (json.RawMessage, error) {
	fields := map[string]json.RawMessage{}
	if err := json.Unmarshal(body, &fields); err != nil {
		return body, nil //nolint:nilerr // not a JSON object: not ours to rewrite
	}
	// A literal `null` unmarshals without error into a nil map; the write below would panic.
	if fields == nil {
		return body, nil
	}

	fields["response_format"] = format

	patched, err := json.Marshal(fields)
	if err != nil {
		return nil, fmt.Errorf("llm: encode patched request: %w", err)
	}

	return patched, nil
}
