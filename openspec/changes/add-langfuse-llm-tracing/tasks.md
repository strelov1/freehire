## 1. Configuration

- [x] 1.1 Add optional `LangfuseBaseURL`, `LangfusePublicKey`, `LangfuseSecretKey` fields to `internal/config` `Config`, loaded from the matching env vars
- [x] 1.2 Add a helper reporting tracing enabled only when all three are non-empty (mirrors the `MEILI_MASTER_KEY` gate), covered by a test

## 2. Tracer core (`internal/llm/langfuse.go`)

- [x] 2.1 Define the `Generation` value (model, system+user input, output, optional usage, start/end time, error, source label) and a `Tracer` interface (`Observe(Generation)`, `Shutdown(context.Context) error`)
- [x] 2.2 Implement the Langfuse Ingestion payload mapping (generation → ingestion event JSON: input, output, usage, level DEFAULT/ERROR, statusMessage, metadata.source), unit-tested against a captured JSON shape
- [x] 2.3 Implement the HTTP send: `POST {base}/api/public/ingestion` with Basic auth, verified against an `httptest` server (asserts path, auth header, body)
- [x] 2.4 Implement async buffering: `Observe` enqueues without blocking; a background goroutine batches and sends; a full buffer drops with a warn instead of blocking — tested for non-blocking behaviour
- [x] 2.5 Implement `Shutdown` draining and flushing the buffer before returning; tested that a queued generation is sent before `Shutdown` completes
- [x] 2.6 Make all send/serialization failures best-effort: logged and swallowed, never surfaced to the caller — tested with a failing transport
- [x] 2.7 Add the config-driven constructor returning a live tracer when configured and `nil` (no-op) otherwise, unit-tested both ways

## 3. Wire into the LLM client (`internal/llm/llm.go`)

- [x] 3.1 Add an optional `tracer` and a `source` label to `Client`; extend `New`/`NewWithModel` to accept them without breaking existing call sites
- [x] 3.2 In `GenerateJSON`, measure latency and `Observe` a success generation (extract usage tokens defensively from `resp.Choices[0].GenerationInfo`; omit usage when absent) — tested via a fake model + fake tracer
- [x] 3.3 In `GenerateJSON`, `Observe` an ERROR generation on generate error / no choices, then return the unchanged error — tested that the returned error is identical and a trace was still recorded
- [x] 3.4 Guard every tracer touch on nil so the unconfigured client is unchanged — tested that a nil-tracer client behaves exactly as before

## 4. Wire the workers

- [x] 4.1 Build the tracer from config in `cmd/enrich` and pass it (with source label `enrich`) into the LLM client; call `Shutdown` on worker exit
- [x] 4.2 Build the tracer from config in `cmd/tg-extract` and pass it (with source label `telegram`) into the LLM client; call `Shutdown` on worker exit

## 5. Verify

- [x] 5.1 `go build ./...` and `go vet ./...` clean; `go test ./...` green
- [x] 5.2 Live smoke: run `cmd/enrich` (or a minimal harness) with the real `LANGFUSE_*` env against one job and confirm a generation appears in the Langfuse project, with tokens and latency populated
