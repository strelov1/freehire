package llm

import (
	"log"
	"net/http"
	"strings"

	"github.com/tmc/langchaingo/httputil"
	"github.com/tmc/langchaingo/llms/openai"
)

// tagHeader is how the gateway is told which feature a call served. It files one spend
// row per tag per day, which is what makes "what does tailoring cost" answerable without
// a table of our own.
const tagHeader = "x-litellm-tags"

// As returns a client that spends under a given credential and labels its calls.
//
// An empty secret keeps the client's own credential — attribution fails open, and a call
// that could not be attributed to a person should still say which feature it was, so the
// tags are applied either way. Passing neither returns the receiver untouched, so a call
// site can hand through values it did not inspect without changing what it sends.
//
// onRefused is called when the gateway rejects the given secret, which means it no longer
// knows that credential. The call itself is re-made on the client's own credential and
// completes, so a gateway that lost its key table costs nobody their answer; onRefused is
// how the stale value gets replaced. It may be nil.
//
// Nil-safe, and a no-op on a client built by NewWithModel: that seam injects a model with
// no endpoint behind it, so there is nothing to rebuild, exactly as a schema there is
// ignored rather than fatal.
//
// A failure to rebuild returns the receiver. Losing attribution is a log line; failing the
// caller's request over bookkeeping is not a trade this package gets to make.
func (c *Client) As(secret string, onRefused func(), tags ...string) *Client {
	if c == nil {
		return nil
	}
	if secret == "" && len(tags) == 0 {
		return c
	}
	if c.baseURL == "" {
		return c
	}

	clone := *c
	if secret != "" {
		// The fallback the retry uses must always be the real service credential,
		// never another user's secret. c.apiKey is only that when c is itself the
		// base client; if c is already bound to a per-user credential (c.fallbackKey
		// != ""), that field already holds the service credential and must be
		// carried forward as-is — otherwise chaining As twice (base.As(userA).As(userB))
		// would leave userB's fallback pointing at userA's key, and a refusal for
		// userB would silently retry, and keep spending, on userA's credential.
		fallback := c.apiKey
		if c.fallbackKey != "" {
			fallback = c.fallbackKey
		}
		clone.fallbackKey = fallback
		clone.apiKey = secret
		clone.onRefused = onRefused
	}
	clone.tags = tags

	// The schema-model cache MUST NOT be shared with the client this was cloned from.
	// It is keyed on the schema's name and rendered shape and on nothing else, so a
	// shared cache would serve this clone the model another credential built — the call
	// would succeed, the response would decode, and the spend would land on the wrong
	// account. The same key blindness applies to tags.
	clone.schemaModels = newModelCache()

	m, err := openai.New(
		openai.WithBaseURL(clone.baseURL),
		openai.WithToken(clone.apiKey),
		openai.WithModel(clone.modelID),
		openai.WithHTTPClient(&http.Client{Transport: clone.transport()}),
	)
	if err != nil {
		log.Printf("llm: could not bind client to a credential: %v", err)
		return c
	}
	clone.model = m

	return &clone
}

// transport is what an outgoing call travels on: langchaingo's own transport, wrapped in
// the attribution stamp when this client carries tags or a per-user credential.
// Schema-bound models build on top of it, so a tagged client's schema calls are tagged
// too.
func (c *Client) transport() http.RoundTripper {
	if len(c.tags) == 0 && c.fallbackKey == "" {
		return httputil.DefaultTransport
	}
	return &attribution{
		tags:      strings.Join(c.tags, ","),
		fallback:  c.fallbackKey,
		onRefused: c.onRefused,
		// next is langchaingo's own transport, which stamps the library's User-Agent.
		// The gateway already groups spend by that, so replacing it outright would lose
		// an axis we get for nothing.
		next: httputil.DefaultTransport,
	}
}

// attribution stamps the feature tags onto an outgoing call and rescues one whose
// credential the gateway has forgotten.
//
// Both live here rather than further up because this is the only layer that sees the
// wire. langchaingo models the request body and has no way to add a header to it, and it
// reports a refusal as an error string — matching on that text would work until the
// library reworded it, silently. The status code does not move.
type attribution struct {
	next      http.RoundTripper
	tags      string
	fallback  string
	onRefused func()
}

func (t *attribution) RoundTrip(req *http.Request) (*http.Response, error) {
	next := t.next
	if next == nil {
		next = http.DefaultTransport
	}
	// A RoundTripper must not mutate the request it is given.
	clone := req.Clone(req.Context())
	if t.tags != "" {
		clone.Header.Set(tagHeader, t.tags)
	}

	resp, err := next.RoundTrip(clone)
	if err != nil || resp.StatusCode != http.StatusUnauthorized || t.fallback == "" {
		return resp, err
	}

	// The gateway does not know this credential. Retry once on the one it does — and
	// only once: a gateway refusing the fallback too is a misconfiguration, and looping
	// on it would turn one bad key into a request storm.
	retry, ok := replay(req, t.fallback, t.tags)
	if !ok {
		return resp, nil
	}
	resp.Body.Close()
	if t.onRefused != nil {
		t.onRefused()
	}

	return next.RoundTrip(retry)
}

// replay rebuilds a request under a different credential. It reports failure rather than
// erroring, because a body that cannot be replayed means the original response — refusal
// and all — is still the honest answer to give back.
func replay(req *http.Request, credential, tags string) (*http.Request, bool) {
	if req.Body != nil && req.GetBody == nil {
		return nil, false
	}
	clone := req.Clone(req.Context())
	if req.GetBody != nil {
		body, err := req.GetBody()
		if err != nil {
			return nil, false
		}
		clone.Body = body
	}
	clone.Header.Set("Authorization", "Bearer "+credential)
	if tags != "" {
		clone.Header.Set(tagHeader, tags)
	}

	return clone, true
}
