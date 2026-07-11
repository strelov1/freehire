## 1. LLM streaming primitive

- [x] 1.1 RED: test `llm.Client.GenerateJSONStream` — content tokens accumulate into the returned JSON; `onThinking` receives reasoning deltas when the fake model emits them; nil-safe
- [x] 1.2 GREEN: add `GenerateJSONStream(ctx, system, user, onThinking func(string)) (string, error)` using `llms.WithStreamingFunc` (content → buffer, reasoning delta → onThinking); keep `GenerateJSON` intact

## 2. Streaming analyzer (`internal/jobfit`)

- [x] 2.1 RED: test `AnalyzeStream` emits events in order (stage start/done ×3, requirements, dimensions, analysis) and returns the same final analysis as `Analyze`; a stage error emits nothing partial and returns the error
- [x] 2.2 GREEN: add `Event` type + `AnalyzeStream(ctx, in, emit)` running the three stages with per-stage streaming; emit stage/thinking/requirements/dimensions/analysis
- [x] 2.3 REFACTOR: reimplement `Analyze` as a thin `AnalyzeStream` collector (no-op emitter), delete the duplicated stage loop

## 3. SSE endpoint (`internal/handler`)

- [x] 3.1 RED: `job_fit_stream_integration_test.go` — the stream yields ordered events ending in `analysis`, and the analysis is cached; unauthenticated 401; no CV → a terminal event indicating has_cv=false
- [x] 3.2 GREEN: `StreamJobFit` on `GET /jobs/:slug/fit/stream` (Fiber `SetBodyStreamWriter`, `text/event-stream`, `X-Accel-Buffering: no`, flush per event); build the input (job geo + profile prefs, like PostJobFit), run `AnalyzeStream`, write SSE events, cache on completion, emit `error` on failure
- [x] 3.3 Wire the route (`RequireAuthOrKey`) in `handler.go`

## 4. Frontend — dedicated page

- [x] 4.1 `web/src/routes/jobs/[slug]/fit/+page.server.ts`: SSR the job + the cached analysis (`getJobFit`); 404 on unknown slug
- [x] 4.2 Shared presentation in `web/src/lib/jobFit.ts` (+ vitest): the SSE event reducer (accumulate stage/thinking/section events into page state) and stepper state — pure, DOM-free
- [x] 4.3 `+page.svelte`: full-width detailed layout (overall+verdict, six dimensions with rationale, ATS requirement table, strengths/gaps/recommendation); when no fresh cache/`?recompute`, open `EventSource` and render the stepper + thinking panel + progressive sections; beautiful, accessible, dark-mode + reduced-motion
- [x] 4.4 `api.ts`: a small `jobFitStreamUrl(slug)` helper (EventSource needs a URL, not fetch)

## 5. Sidebar summary

- [x] 5.1 Rework `web/src/lib/components/JobFitAnalysis.svelte` into a compact summary (overall % + verdict + top gap from `getJobFit`) with a link to `/jobs/[slug]/fit`; no inline compute
- [x] 5.2 Update `jobFit.test.ts` for any changed pure helpers

## 6. Verify & finish

- [x] 6.1 `go build ./... && go vet ./... && go test ./...`; integration `-tags=integration` (fit + stream)
- [x] 6.2 Frontend: `svelte-check` + `vitest` + eslint + prod build; visual-verify the page (headless screenshot) — it must look polished
- [x] 6.3 Update `AGENT.md`: the streaming SSE path, the dedicated page, the sidebar summary
