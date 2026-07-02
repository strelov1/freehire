## Context

Both LLM workers reach the model through one method: `llm.Client.GenerateJSON`
in `internal/llm/llm.go`. It builds a system+user message, calls
`model.GenerateContent(..., WithJSONMode())`, and returns the fence-stripped
string — discarding `resp`, which is where langchaingo puts token usage
(`resp.Choices[0].GenerationInfo`, keys `PromptTokens` / `CompletionTokens` /
`TotalTokens`). The client is provider-agnostic (`openai.New` with any base URL),
so a budget model and Claude are the same code path.

Langfuse has no official Go SDK. It exposes a batch **Ingestion API**
(`POST /api/public/ingestion`, HTTP Basic auth `public_key:secret_key`) that
accepts events; a "generation" observation carries `model`, `input`, `output`,
`usage`, timestamps, `level`, and `metadata`. The user has chosen Langfuse Cloud
(US region) — no self-hosting.

## Goals / Non-Goals

**Goals:**
- Capture every model call (prompt, response, tokens, latency, error level) from
  a single seam, covering `enrich` and `tg-extract` at once.
- Zero behavioural change when unconfigured; zero impact on the caller's result
  or latency when configured.
- No heavy dependencies — standard library `net/http` only.

**Non-Goals:**
- OpenTelemetry / vendor-neutral export (rejected below; can be revisited).
- Nested trace trees, sessions, scores, prompt-management, or datasets — a flat
  generation per call is enough for the four target questions.
- Tracing anything other than LLM calls (no HTTP/DB spans).
- Retrying or persisting traces across a crash — best-effort only.

## Decisions

**1. Thin HTTP client to the Ingestion API, not OpenTelemetry.**
The target is specifically Langfuse Cloud, and the Langfuse generation model
(native `usage` + `cost` + `level`) maps directly onto the Ingestion API. An OTel
path would add the otel-sdk + exporter + semconv dependencies and require hand
mapping `gen_ai.*` attributes onto Langfuse's model — abstraction for a
portability need we do not have (YAGNI). Chosen: ~100-line `net/http` client.
*Alternative considered:* OTel OTLP export — deferred; revisit only if a second
observability backend appears.

**2. Instrument inside `GenerateJSON`, not around it.**
Usage tokens live on `resp`, which `GenerateJSON` currently drops. Wrapping the
call from outside cannot see tokens — the whole point for the cost goal. So the
`Client` gains an optional `tracer` field, and `GenerateJSON` measures latency
and reports the generation before returning. The method signature and its
return/error semantics are unchanged.

**3. New sibling type `Tracer` in `internal/llm`.**
A `Tracer` interface (`Observe(Generation)` + `Shutdown(ctx)`) with a Langfuse
HTTP implementation, kept in `internal/llm/langfuse.go`. Construction is
config-driven: a constructor returns a live tracer when all three vars are set,
and `nil` otherwise. A `nil` tracer is the no-op — `GenerateJSON` guards with a
nil check, so the disabled path adds one branch and nothing else. This keeps the
tracer a distinct responsibility from the model-calling client.

**4. Asynchronous, buffered send with explicit flush.**
`Observe` hands the generation to a buffered channel and returns immediately, so
reporting is off the caller's critical path. A background goroutine batches
events and POSTs them; `Shutdown(ctx)` drains the buffer and flushes before the
run-once worker exits. Callers (`cmd/enrich`, `cmd/tg-extract`) already have a
clean shutdown point to call it.

**5. Config via `internal/config`.**
Three optional fields (`LangfuseBaseURL`, `LangfusePublicKey`,
`LangfuseSecretKey`) join the existing `Config`. The enrich/telegram wiring builds
the tracer and passes it to `llm.New`. Empty values → `config` reports tracing
disabled, exactly like the `MEILI_MASTER_KEY` pattern.

**6. Payload shape.** Each generation carries: `model` (from client config),
`input` = {system, user prompts}, `output` = raw response, `usage` =
{input, output, total} when present, `startTime`/`endTime` for latency,
`level` = `DEFAULT` on success / `ERROR` on failure (with the error string as
`statusMessage`), and `metadata.source` = the caller label (`enrich` /
`telegram`). The caller label is passed into `llm.New` so the client knows its
own workload.

## Risks / Trade-offs

- **Secret key handling** → It is a credential; it lives only in env / `.env`
  (gitignored) and is never logged or embedded in a trace payload.
- **Async buffer can drop traces on crash / if full** → Acceptable: tracing is
  best-effort observability, not data of record. A full buffer drops the
  generation (and logs a warn) rather than blocking the LLM call.
- **Prompts/descriptions in `input` may contain scraped PII** → The data is
  already stored in `jobs`; sending it to Langfuse Cloud is the same trust
  boundary as the LLM provider itself. Description length is already capped
  upstream (`maxDescriptionRunes`).
- **langchaingo usage keys are provider-dependent** → Read them defensively;
  when absent, omit usage rather than reporting zeros (a spec scenario).
- **Extra goroutine in a run-once worker** → Bounded and joined by `Shutdown`;
  the worker's existing lease/flock lifecycle is unaffected.

## Migration Plan

1. Ship the code; unconfigured environments are unchanged (no tracer).
2. Add the three `LANGFUSE_*` vars to the `enrich` / `tg-extract` cron env to
   turn tracing on. No DB migration, no reindex, no API change.
3. Rollback: unset the vars (tracer goes no-op) or revert the commit.

## Open Questions

- None blocking. Cost display in Langfuse depends on the model being registered
  in the Langfuse project's model list; if a custom model id is unknown to
  Langfuse, tokens still show but cost may not — a project-side config, not a
  code concern.
