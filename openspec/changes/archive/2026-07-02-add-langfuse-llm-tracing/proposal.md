## Why

The LLM workers (`cmd/enrich`, `cmd/tg-extract`) are a black box: we cannot see
what a model was actually sent, what it returned, how many tokens (and thus how
much money) each call burned, how slow the gateway was, or why a given posting
produced invalid JSON or out-of-vocabulary facets. This blindness blocks four
concrete decisions — controlling token cost, debugging enrichment quality,
comparing a budget model against Claude, and diagnosing latency/dead-lettering.

## What Changes

- Add a **Langfuse tracer** that records every LLM call — prompt, response,
  token usage, latency, and error level — to Langfuse Cloud.
- Wire the tracer into `llm.Client.GenerateJSON` (the single seam both LLM
  workers already share), so `enrich` and `tg-extract` are covered without
  touching either worker package.
- Gate tracing on `LANGFUSE_BASE_URL` / `LANGFUSE_PUBLIC_KEY` /
  `LANGFUSE_SECRET_KEY`; when any is unset, the tracer is a no-op and behaviour
  is unchanged (mirrors how `MEILI_MASTER_KEY` gates search).
- Tracing is **best-effort**: a Langfuse failure is logged and swallowed, never
  failing an LLM call or a worker run (mirrors best-effort incremental
  indexing).

## Capabilities

### New Capabilities
- `llm-observability`: Tracing of every model call made through the shared LLM
  client to Langfuse — captured prompt/response, token usage, latency, and error
  level — gated on configuration and non-blocking to the caller.

### Modified Capabilities
<!-- None: GenerateJSON's contract (inputs, return value, error semantics) is
     unchanged; tracing is a passive side-channel. No existing spec's
     requirements change. -->

## Impact

- **New code**: a Langfuse tracer (thin HTTP client to the Langfuse Ingestion
  API) alongside `internal/llm`; tracer construction wired from `internal/config`.
- **Modified code**: `internal/llm/llm.go` — `Client` gains an optional tracer,
  observed inside `GenerateJSON` where the model response (with usage tokens) is
  still in scope. `internal/config` — three new optional env vars.
- **Config/ops**: `.env.example` documents the three `LANGFUSE_*` vars (done);
  cron worker environments (`enrich`, `tg-extract`) get them to enable tracing.
- **Dependencies**: none required beyond the standard library `net/http`; no
  OpenTelemetry SDK.
- **No API/DB/schema changes**; no change to the `Enrichment` contract or any
  HTTP surface.
