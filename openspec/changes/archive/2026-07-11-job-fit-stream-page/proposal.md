## Why

The AI fit analysis is rendered inside the ~320px Profile-match sidebar, so the six-dimension
report is cramped while the page has wide empty space. A detailed, recruiter-grade analysis deserves
its own room — and computing it silently for seconds behind one button hides the work. Give it a
dedicated, beautiful page that streams the three-stage chain live (stage progress, the model's
thinking, and each section as it resolves), and keep only a short summary in the sidebar.

## What Changes

- Add a dedicated **fit analysis page** at `/jobs/[slug]/fit`: a full-width, detailed layout —
  overall score + verdict, the six scored dimensions with full rationale, the ATS requirement table,
  strengths/gaps/recommendation.
- **Stream the computation live** over Server-Sent Events: a three-step stage stepper
  (Extract & Match → Recruiter verdict → Adversarial audit), a live **thinking** panel (the model's
  reasoning tokens, best-effort — empty when the model emits none), and each structured section
  rendered as its stage resolves.
- The page **SSR-paints the cached analysis instantly** when one is fresh; a missing/stale cache (or
  an explicit recompute) opens the stream and fills the page progressively, then caches the result.
- **Reduce the sidebar block to a short summary** — overall % + verdict + the top gap — with a
  "View full analysis" link to the page; the sidebar no longer computes inline.
- Streaming is browser-facing (SSE, cookie/key auth); the existing synchronous `POST /jobs/:slug/fit`
  stays for API/CLI clients (both drive the same chain).

## Capabilities

### Modified Capabilities
- `job-fit-analysis`: gains a live-streaming SSE compute path and a dedicated detailed page; the
  sidebar block is reduced to a summary. The scored-verdict contract and cache are unchanged.

## Impact

- `internal/llm` (a streaming `GenerateJSONStream` over `llms.WithStreamingFunc`), `internal/jobfit`
  (an `AnalyzeStream` that emits stage/thinking/section events; `Analyze` becomes a thin collector),
  `internal/handler` (new SSE endpoint `GET /jobs/:slug/fit/stream`, caching on completion).
- `web`: new route `web/src/routes/jobs/[slug]/fit/` (+page.server.ts SSR of cached result, +page.svelte
  with the EventSource client + streamed layout), a reworked `JobFitAnalysis.svelte` summary, shared
  presentation logic + tests.
- No new config or migration. LLM-off / thinking-less models degrade gracefully.
