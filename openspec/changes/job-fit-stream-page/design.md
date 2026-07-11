## Context

The fit analysis (six-dimension verdict, three-stage chain, per-(user,job) cache) currently computes
synchronously in `POST /jobs/:slug/fit` and renders in the cramped Profile-match sidebar. We want a
dedicated page that streams the computation live. The building blocks: langchaingo supports
`llms.WithStreamingFunc` (token streaming), Fiber supports SSE via `SetBodyStreamWriter`, SvelteKit
SSRs via `+page.server.ts`, and the browser consumes SSE via `EventSource`.

## Goals / Non-Goals

**Goals:** a beautiful, full-width detailed page; live stage progress + best-effort thinking +
progressive section fill over SSE; instant SSR paint of a fresh cached result; a compact sidebar
summary linking to the page. Same cache and final analysis as today.

**Non-Goals:** no change to the scored-verdict contract, the weights, or the cache schema. No
true SSR token-streaming (SvelteKit deferred streaming paints the *cached* result, not live LLM
tokens — live is SSE). No background precompute.

## Decisions

**1. SSE event protocol (named events).** `GET /jobs/:slug/fit/stream` emits:
- `stage` — `{n:1|2|3, phase:"start"|"done", label}` drives the stepper.
- `thinking` — `{stage, text}` reasoning-token deltas; best-effort (absent on non-reasoning models).
- `requirements` — the sanitized Stage-1 requirement match (after Stage 1).
- `dimensions` — the (interim) six dimensions + overall/verdict after Stage 2.
- `analysis` — the final, audited full analysis (after Stage 3); the terminal payload.
- `error` — `{message}` then close; no partial cache.
The stream caches the final analysis on `analysis`, identically to the sync path (same upsert +
triple-stamp). Rationale: named events keep the client a simple switch; the structured per-stage
events let the page fill sections without parsing partial JSON.

**2. Streaming is the core; sync is a collector.** `internal/jobfit` gains
`AnalyzeStream(ctx, in, emit func(Event)) (*Analysis, error)` that runs the three stages, calling
`emit` for stage/thinking/section events; it returns the final analysis. `Analyze` becomes a thin
wrapper that calls `AnalyzeStream` with a no-op emitter — one chain implementation, no duplication.
*Alternative:* two parallel implementations — rejected (drift risk).

**3. `llm.Client.GenerateJSONStream`.** A streaming sibling of `GenerateJSON` using
`llms.WithStreamingFunc`: content tokens accumulate into the returned JSON string (parsed by the
caller as today); a separate `onThinking` callback receives reasoning deltas when the provider
surfaces them (best-effort — many OpenAI-compatible endpoints put reasoning in a `reasoning`/
`reasoning_content` delta; when absent, `onThinking` is simply never called). The raw JSON tokens are
NOT surfaced to the UI (partial JSON is noise) — only thinking text is shown live; structured results
appear when each stage's JSON parses.

**4. SSR paints cache; client streams cold.** `+page.server.ts` loads the job + the cached analysis
(`GET /fit`). Fresh cache → the page is fully server-rendered, no stream. No/stale cache (or
`?recompute`) → on mount the client opens `EventSource('/api/v1/jobs/:slug/fit/stream')` and renders
progressively. SSE rides the session cookie (same-origin); auth is `RequireAuthOrKey`.

**5. Sidebar becomes a summary.** `JobFitAnalysis.svelte` drops its inline compute/expander and shows
overall % + verdict + top gap (from `GET /fit`) with a link to `/jobs/[slug]/fit`; when no cache, a
button that navigates there. The heavy render moves to the page.

**6. Beautiful, accessible detailed layout.** Full-width two-column report (dimensions + rationale on
one side, ATS requirements on the other; strengths/gaps/recommendation below), a prominent
overall+verdict header, a stage stepper and a collapsible thinking panel shown only while streaming.
Design-system tokens, dark-mode parity, reduced-motion respected.

## Risks / Trade-offs

- **JSON-mode + streaming interplay** → we stream for `onThinking` only; the JSON answer still
  accumulates and is parsed at stage end, so JSON-mode correctness is unchanged. If a provider rejects
  streaming+JSON, `AnalyzeStream` falls back to a non-streaming call per stage (still emits stage +
  section events, just no thinking).
- **SSE connection dropped mid-stream** (navigation away, proxy timeout) → the server ctx cancels;
  nothing is cached on a partial run; reopening restarts (or hits the cache if a prior run finished).
- **nginx buffering SSE** → set `X-Accel-Buffering: no` + flush per event so events reach the browser
  promptly (host-2 nginx note).
- **Thinking absent on the prod budget model** → the panel is simply hidden; the stepper + progressive
  sections still deliver the "live" feel.

## Migration Plan

No DB migration. Ship backend (stream endpoint dormant-safe) + frontend together; deploy via the
normal release. Rollback: the sidebar link + page are additive; the old sync path is untouched.

## Open Questions

- Whether to auto-recompute on opening the page when the cache is stale, or show the stale result with
  a recompute button. Default: show stale instantly (SSR) + a visible "recompute" that opens the stream.
