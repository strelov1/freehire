# LLM-client conventions

## Scope
A thin provider-agnostic wrapper over langchaingo for OpenAI-compatible endpoints: JSON
generation with fence-stripping and optional schema constraints, streaming, tool-calling
chat, Langfuse tracing, and per-user spend attribution via gateway credentials and
`x-bf-dim-<dimension>` headers (credential.go:19). Callers keep their own prompts and typed-contract
parsing. Consumers: `cmd/server`, `cmd/enrich`, `cmd/tg-extract`, `cmd/classify-mail`, the
backfills, and `internal/ai/enrich`, `matchanalysis`, `resumeextract`, `atscheck`,
`mailclassify`, `mailrecall`, `telegram`, `autofillagent`, `assistant`, `cv`.

## Always true
- **`NewClient(Settings, source)` is the single construction path** (llm.go:158). It builds
  the client, attaches a Langfuse tracer when the three `Langfuse*` fields are set, and
  returns `(client, flush, err)`. **Always defer the flush func** — it drains buffered
  traces and is a no-op when tracing is off. An unconfigured `Settings` (`!Enabled()`)
  returns `(nil, no-op, nil)` so callers degrade uniformly. `NewWithModel` is the tests-only
  seam (injects a fake model; no endpoint behind it).
- **No env is read here.** The package takes `llm.Settings`; env lives in `internal/platform/config`
  (`LLM_BASE_URL`/`LLM_API_KEY`/`LLM_MODEL`, `LANGFUSE_*`). No vendor or model is
  hard-coded — any OpenAI-compatible backend works.
- **`DefaultTimeout = 90s` bounds every call** (llm.go:30). Without it a stalled gateway
  hangs a run-once worker indefinitely. `WithTimeout` (llm.go:81) returns a shallow copy
  with a different bound so a slow use case (multi-stage fit analysis) can allow longer
  calls without raising the shared default.
- **`GenerateJSONStream` overrules the provider's nil error when our own deadline fired**
  (llm.go:292-300). A gateway that stops emitting when our context expires can hand back
  whatever it accumulated and call it success — production logged exactly that as
  `dur=3m0.018s err=<nil>`, and the truncated JSON failed downstream as "unexpected end of
  JSON input", naming neither the deadline nor the stage. Our context is the authority on
  whether the call had time to finish.
- **A schema is bound per model client, and `As` MUST NOT share the schema-model cache**
  (credential.go:55-60). langchaingo binds the response format to the client, so one model
  carries at most one schema and the cache is keyed on the schema's name and shape — nothing
  else. A shared cache would serve the re-credentialed clone a model another credential
  built: the call succeeds, the response decodes, and the spend lands on the wrong account.
  The same key blindness applies to tags.
- **Attribution fails open** (`As`, credential.go:35). An empty secret keeps the client's
  own credential but still tags the call; a rebuild failure returns the receiver — losing
  attribution is a log line, failing the caller's request over bookkeeping is not a trade
  this package gets to make. A 401 on a per-user key retries ONCE on the service credential
  and calls `onRefused` (credential.go:121-139): the gateway no longer knows that key, the
  user's answer still completes, and `onRefused` is how the stale value gets replaced. Only
  once — a gateway refusing the fallback too is a misconfiguration, and looping would turn
  one bad key into a request storm. The rescue lives in a RoundTripper because that is the
  only layer that sees the wire: langchaingo cannot add a header to a request, and it
  reports a refusal as an error string whose wording could change silently — the status
  code does not move (credential.go:96-108).
- **A schema is a first line, never a proof** (llm.go:7-11). `WithSchema` (schema.go:43,
  built via `internal/platform/llmschema`) constrains generation, but a gateway that stops honouring
  a schema answers 200 with ordinary JSON — every caller-side sanitiser and validator stays
  exactly where it was.
- **Background work never re-credentials.** Only calls made FOR a signed-in user go through
  `As`; enrichment, Telegram, and embeddings keep the service credential. See
  internal/ai/llmkey/AGENTS.md for the credential-resolution side (lazy minting, feature tags,
  the test enforcing the background rule).

## Surface
- `GenerateJSON` (llm.go:203) / `GenerateJSONStream` (llm.go:257) — system+user prompt in
  JSON mode; the stream sibling forwards reasoning tokens to `onThinking` and returns the
  accumulated content. Output is raw — the caller unmarshals into its own contract type.
- `Chat` (chat.go:34) — tool-calling conversation with optional streaming; the assistant's
  turn loop runs on this.
- `StripJSONFence` (llm.go:370) — recovers the first JSON value from fenced/preambled model
  output. `TrimTruncateRunes` (llm.go:419) — the one "trim + truncate by runes" helper for
  bounding untrusted/model text.
- `UsageFrom` (llm.go:376) — reads token counts defensively; an absent usage is reported as
  absent (nil), never as zeros. It also reads `PromptCachedTokens`, the part of the prompt
  the provider served from its cache — the figure that decides what a REPLAYED
  conversation costs, and the one a price page cannot tell you. **A cached count alone is
  never a usage**: a provider that named it and nothing else said nothing about the call's
  size, and a zero `Input` invented beside it would read as a free request.
  **Zero does not mean "no cache".** langchaingo writes that key unconditionally from a
  zero-valued struct, so a provider that reports nothing and a request that hit nothing
  both arrive as `0`, and nothing at this layer can tell them apart. A caller needing the
  distinction draws it across a whole run — every round reporting zero against a prefix
  that should have been warm is evidence about the provider; one call is not.
- `Tracer`/`NewTracer` (langfuse.go) — Langfuse observations labelled with the entrypoint's
  source; a nil tracer makes every observation a no-op. Token counts travel as Langfuse's
  `usageDetails`, whose contract is that **every key is a non-overlapping bucket** — each
  token counted under exactly one. The provider does not report them that way: an
  OpenAI-shaped `prompt_tokens` counts the cached prefix inside itself, so `usageBuckets`
  (langfuse.go:245) subtracts before sending. Copying both figures across unadjusted would
  bill the cached prefix twice, once at the input rate and once at the cache-read rate —
  the direction that makes a cheap model look expensive. A cached count larger than the
  prompt containing it is not a reading that can be split, so the prompt goes through
  whole rather than as a negative bucket Langfuse would price as written. No total is
  sent: Langfuse derives it, and one of ours would be a fourth number free to disagree
  with the three it was built from. Requires Langfuse v3 (self-hosted on hydra —
  `freehire-ops/provision/langfuse/`, image `langfuse/langfuse:3`).
