## Context

Today, `Store.Tailor` (`internal/cv/store.go:291`) 409s with "run the fit analysis first" when no
cached `matchanalysis.Analysis` exists for the (user, vacancy) pair (`cachedAnalysisCtx`,
`internal/handler/cv_tailor.go:333-352`). PR #1658 makes the frontend absorb that 409 by running the
fit-analysis SSE stream inline, then retrying the bootstrap — the candidate still sits on an
"analyzing" screen before the workspace opens.

Crucially, the autopilot run's OWN first move depends on that same cache: its system prompt tells
the model to call `cv_context` first (`internal/assistant/prompt.go:123,156`), and `cv_context`
(`internal/handler/assistant_cv_tools.go:182-201`) calls the identical `cachedAnalysisCtx` — so an
autopilot run started against a vacancy with no cached analysis fails its first tool call today.
"Kick off autopilot immediately, run analysis in parallel" is not implementable as two literally
concurrent, mutually-unaware operations: autopilot has a hard, structural read-dependency on the
analysis existing. This design's central decision is how to reconcile that dependency with the
proposal's "no analysis-gated screen" goal.

Two other facts from the codebase shape this design:
- Autopilot is currently **unthrottled**: no rate-limiter middleware on
  `POST /assistant/sessions/:id/autopilot` (`internal/handler/assistant.go:151`) and no
  `credits.Debit` call anywhere in its handler, unlike the fit analysis (30/hour via
  `matchAnalysisLimiter`, `internal/handler/match_analysis_limit.go:24`, plus a
  `credits.FeatureMatch` debit, `internal/handler/match_analysis.go:268`). Making autopilot the
  automatic default on every first-time `/tailor/[slug]` open turns an opt-in cost into an
  unconditional one.
- The frontend only ever refetches+replaces the whole `Document` once, at turn end
  (`onTurnComplete`, `web/src/routes/tailor/[slug]/+page.svelte:493-504`). Per-tool-call SSE events
  already reach `AssistantChat.svelte` (`~516-597`) for chat rendering, but never touch the
  document. "Show the CV assembling live" needs a new wire from there to `loadCv()`.

## Goals / Non-Goals

**Goals:**
- Every first-time `/tailor/[slug]` open builds a curated, JD-aware CV without the candidate ever
  seeing a screen gated on "run the analysis first."
- The fit analysis still gets computed and lands in the Job Match tab without a separate,
  candidate-visible step.
- The automatic autopilot trigger's spend is attributed to its owning user, matching how every
  other assistant turn is already tracked — no new metering mechanism.
- The CV preview shows visible progress while the cold-start run is in flight, not a silent wait
  followed by a single swap.

**Non-Goals:**
- Making the analysis and the autopilot run literally concurrent at the LLM-call level. `cv_context`
  keeps reading a single source of truth (the cached analysis); this design changes *when that cache
  gets populated*, not the read path.
- Per-bullet or per-token streaming into the CV preview. The increment is per tool call
  (`cv_edit`), matching the document's own unit of change (a batch of path operations).
- Capping `experienceFromBank()` for the base CV (explicitly deferred in the proposal).

## Decisions

### 1. Cold start is the frontend auto-invoking the existing autopilot endpoint, not a new backend-owned background job

Rejected first: a background goroutine started from the bootstrap handler, decoupled from the HTTP
response, running analysis-then-autopilot on its own. Two problems killed it. First, correctness —
it would have to avoid the triggering request's `c.Context()` (a `*fiber.Ctx`-backed `fasthttp`
context that gets recycled once the handler returns; a detached goroutine reading it afterward races
a future, unrelated request reusing the same object) by carrying its own `context.Background()`-based
timeout instead, which is doable but is exactly the class of bug a codebase-wide search for `go
func(` in `internal/handler`/`internal/cv` found **zero** existing precedent for. Second, streaming —
`PostAssistantAutopilot` (`internal/handler/assistant.go:744`) both starts a run AND is its own SSE
response in one request/response cycle; there is no separate "subscribe to a session's live run"
endpoint today. A goroutine started before any request asks to watch it would have nowhere to stream
its events, and a race between "goroutine starts firing tool-call events" and "frontend's next
request opens a listener" would silently drop the run's early events — undermining the exact "watch
it build live" goal this design exists for.

**What ships instead**: the bootstrap response gains a boolean, `cold_start_running`, true exactly
when this call just created a new tailored CV. `internal/cv.Store.Tailor` is the only place that
actually knows whether it inserted a new row or reused an existing one (`tailoredForJob` hit vs.
`CreateTailored`), so it now returns that as an explicit `created bool` rather than making the
caller guess — a first implementation attempt reused `tailored.CreatedAt.Equal(tailored.UpdatedAt)`
(the signal `TailorCV` already computed for seeding the revision history) and a test proved it
unreliable: it stays true across every idempotent reload until something else edits the CV, so it
can't tell "just created" from "reused, never edited since." The frontend, seeing the flag,
immediately calls the EXISTING
`POST /assistant/sessions/:id/autopilot` itself — no user click, no two-action menu — and consumes
the SSE response exactly as a manually-triggered run does today. Nothing new is invented for
streaming; the existing endpoint's request/response IS the live-build channel, with a real, live
listener from the moment the run starts, because starting the run and opening the stream are the
same HTTP call.

`PostAssistantAutopilot` itself gains one precondition step, run inline, on the SAME request's real
context (no detached lifecycle to get wrong): before starting the turn, it ensures a cached fit
analysis exists for (user, job) — if `cachedAnalysisCtx` reports none, it computes one synchronously
(the same three-stage chain the old bootstrap 409 used to require upfront) and writes it to the cache
`cv_context` reads, THEN proceeds to start/stream the turn exactly as it does today. If a cached
analysis already exists (a later manual re-run, or one computed separately), this step is a no-op —
today's behavior, unchanged. This satisfies `cv_context`'s real, unavoidable dependency on a
populated cache (autopilot's first tool call would otherwise fail — see Context) without a detached
background job: the ordering (analysis, then autopilot) is still real and sequential, it just now
happens inside the one request the candidate is already watching stream, instead of before any
request exists to watch it.

*Alternative considered*: loosen `cv_context` to degrade to reading the raw job description when no
cache exists. Rejected — it would give autopilot a second, materially different requirements source
(raw JD text vs. the structured, sanitized `missing-have`/`missing-gap` split the analysis produces),
diverging from `tailor-autopilot`'s existing "walks every requirement of the vacancy's fit analysis"
contract and duplicating analysis logic instead of reusing it.

*Alternative considered*: route the sequence through this codebase's established outbox convention
(`enrichment_outbox`, `semantic_outbox`, `search_outbox` — a claimed row, drained by a separate cron
worker) instead of an inline step in the request. Rejected for THIS job specifically: those outboxes
exist for eventually-consistent housekeeping a candidate never watches, drained on a cron cadence
(minutes). A cron-drained queue would reintroduce the exact "stare at a gate" experience this change
removes, just with a longer, unpredictable wait.

### 2. The workspace opens straight into the auto-triggered run, not a blank chat

A bootstrap response with `cold_start_running: true` tells the frontend to skip
`openingActions()`'s two-action menu entirely and immediately POST to the autopilot endpoint (see
Decision 1), rendering the CV preview with the existing document (the unbounded `Seed()` output,
until autopilot starts trimming it) plus a live-build affordance driven by that same request's SSE
events. This replaces `tailor-workspace`'s "bootstrapped session MUST NOT start talking on its own"
requirement for the cold-start case specifically — re-opening an *existing* tailored CV (`?cv=<id>`)
is unaffected and keeps today's resume-without-kickoff behavior; so does a CV whose cold-start run
already ran once (the flag is only ever true on the call that just created the CV).

### 3. Document refresh becomes per-`cv_edit`-tool-call, not just per-turn

`AssistantChat.svelte`'s existing `tool_result` handling gains a callback invoked when the resolved
tool was `cv_edit`, calling the same `loadCv()` `onTurnComplete` already calls. `onTurnComplete`
itself is unchanged — it remains the end-of-turn safety net (covers any edit whose per-call event was
missed, and keeps flushing pending human edits). This is additive, not a rewrite: same fetch, same
merge logic, called from one more place.

### 4. The automatic autopilot run needs no new metering at all

No new `credits` debit, cap, or distinguishing spend tag is added for the cold-start-triggered run.
The credits/balance system is being phased out in favor of the plain LLM spend attribution the
codebase already has for every assistant turn (`internal/llmkey`, fail-open, "measures and does not
bound" per `AGENTS.md`) — `boundRunner`/`userLLM` (`assistant.go:652`) already attributes every
autopilot call to its owning user under `"preset:"+sess.Preset`, the SAME tag a manual run carries.
`user_llm.go`'s own tag-list comment already states the reasoning this design follows: "CV tailoring
is deliberately absent [from the feature-tag list]... the work is an assistant turn under the
`tailor` preset — already tagged as such. A second tag would double-count the same spend." A
cold-start-triggered run is not a different kind of spend from a manual one; it needs no tag of its
own, just the attribution every turn already gets.

### 5. Failure falls back to the unfiltered `Seed()`, surfaced as an ordinary run failure

If the inline analysis step fails (no LLM configured, transient error) or the autopilot run itself
errors out (empty bank, agent error), `PostAssistantAutopilot`'s stream ends the way any failed
autopilot turn already ends today — no new error path is invented. The CV is left exactly where
`Seed()` left it, since autopilot only ever trims and rewrites on top of that seed and never got the
chance to. The frontend's live-build affordance resolves to "workspace ready" the same way a
successful run's does, just with the unfiltered document, rather than presenting the failure as a
blocking error state — this is a presentation choice on top of the SAME stream-ended/error event the
frontend already has to handle for a manually-triggered run failing today. The fit analysis, if its
own step failed, is retried lazily the next time the Job Match tab is opened without one cached —
matching how a stale/missing analysis is already handled elsewhere (`MatchAnalysisFull.svelte`'s
`isStale` re-run affordance).

### 6. `cv-tailoring`'s "surfaced only after analysis" beta-gating clause is dropped, not replaced

Grepping the frontend for a CTA conditioned on a cached/non-stale analysis (the phrasing the current
spec text uses) finds none post-#1658 — `job-fit-analysis`'s "Sidebar reduced to a summary" delta
already links unconditionally to `/tailor/[slug]`, cached or not. That clause was already dead text
drifted from before #1658, same category of drift #1658's own proposal called out elsewhere in this
spec. This design formally removes it rather than rewriting it to a new condition, since none exists
to describe.

## Risks / Trade-offs

- **[Risk]** The candidate's first CV preview briefly shows the unbounded `Seed()` output (before
  autopilot trims it), which is the exact "unbounded and uncurated" experience this change exists to
  fix. → **Mitigation**: it's a transient state lasting one analysis-chain-plus-agent-run, replacing
  a WORSE transient state today (a blocking "analyzing" screen with no CV visible at all). Capping
  the base seed itself is explicitly deferred, not reopened here.
- **[Risk]** Sequencing analysis-then-autopilot inline in one request means an analysis failure now
  also fails the automatic autopilot run, where today a failed analysis only blocked a manual retry.
  → **Mitigation**: covered by Decision 5 — the seed stands in either way, and the fit analysis
  independently retries lazily when the Job Match tab is next opened.
- **[Risk]** The inline analysis compute adds real latency (a three-stage LLM chain) to the very
  first SSE bytes of the autopilot stream the frontend just auto-triggered, so "live build" starts a
  few seconds later than a manually-triggered run against an already-cached analysis would.
  → **Mitigation**: this is strictly better than what it replaces — PR #1658's flow ran the SAME
  analysis chain, plus a full extra bootstrap retry round-trip, before the workspace was even
  interactive. Here the workspace is interactive immediately (Decision 2); the wait is folded into a
  stream the candidate is already watching, not a gate before it.
- **[Risk]** A new per-`cv_edit` document refetch multiplies `loadCv()` calls across a 30-step
  autopilot run (`autopilotMaxSteps`, `assistant.go:729`) — up to one fetch per edit instead of one
  per turn. → **Mitigation**: `cv_edit` calls are a minority of an autopilot run's tool calls (most
  steps are bank searches), and the fetch is already cheap enough to run at turn end for every
  ordinary conversational edit today; no batching is added pre-emptively without a measured problem.

## Migration Plan

No data migration. Deploy order: backend (drop the 409, add `cold_start_running` to the bootstrap
response, add the inline analysis-precondition step to `PostAssistantAutopilot`) before frontend (drop the `'analyzing'` branch and
`retryBootstrapAfterAnalysis`, wire the auto-trigger and per-`cv_edit` refresh) — an old frontend
against a new backend simply never sees the 409 it used to handle, ignores the new
`cold_start_running` field it doesn't know about, and falls back to its existing two-action menu,
which is backward compatible (the candidate would have to click, same as today). Rollback is a plain
revert; no persisted state depends on the new behavior in a way that would need cleanup.

## Open Questions

None outstanding — the four questions raised in the design doc are resolved by Decisions 1-6 above.
