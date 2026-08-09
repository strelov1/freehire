## 1. Backend: drop the bootstrap's fit-analysis gate

- [x] 1.1 Remove the `cachedAnalysisCtx` 409 check from `TailorCV`'s bootstrap path
      (`internal/handler/cv_tailor.go:~61`); leave the separate résumé-less 409
      (`cv_tailor.go:84-87`, `cv.ErrNoResume`) untouched.
- [x] 1.2 Add a `ColdStartRunning bool` field to `tailorCVResponse` (`cv_tailor.go`), true exactly
      when this call just created the tailored CV. `internal/cv.Store.Tailor` now returns this as an
      explicit third value (`created bool`) instead of the caller guessing from timestamps — a
      timestamp-equality heuristic was tried first and proven unreliable by a test (it can't tell
      "just created" from "reused, never edited since").

## 2. Backend: the autopilot endpoint satisfies its own analysis precondition

- [x] 2.1 In `PostAssistantAutopilot` (`internal/handler/assistant.go:744`), before starting the
      turn, check `cachedAnalysisCtx` for the session's vacancy; on a genuine miss (no row, not a
      transient error), compute the fit analysis via the existing `internal/matchanalysis` compute
      path (the same one `PostMatchAnalysis` in `match_analysis.go` uses — factor a reusable
      `matchHandlers` method rather than duplicating it) and write it to the same cache row, using
      the request's own context throughout (never a detached one), tagged `tagMatchAnalysis` — the
      same attribution tag `PostMatchAnalysis` already uses, since this is the same feature
      regardless of who triggers it. When a cached analysis already exists, this is a no-op —
      today's behavior. No credits debit for this inline compute (see design.md Decision 4 — the
      credits system is being phased out; this path attributes spend by tag only, like everything
      else in this change).
- [x] 2.2 On the analysis compute failing (no LLM configured, transient error), let the turn fail the
      way an autopilot run failing today already fails — no new error path. Confirm (via test) the
      tailored CV is left exactly as `Seed()` produced it.

## 3. Frontend: remove the superseded "analyzing" flow

- [x] 3.1 Remove the `status: 'analyzing'` branch (`web/src/routes/tailor/[slug]/+page.svelte:50,
      340, 554-569`).
- [x] 3.2 Remove `retryBootstrapAfterAnalysis` (`+page.svelte:263-287`, wired at `:569`).
- [x] 3.3 Remove the inline `MatchAnalysisFull` render and its now-unused import
      (`+page.svelte:22, 569`).

## 4. Frontend: cold-start auto-trigger

- [x] 4.1 When the bootstrap response carries `cold_start_running: true`, skip the two-action empty
      chat and immediately call the SAME action the workspace's own "tailor it for me" button
      already calls (`POST /assistant/sessions/:id/autopilot`), consuming its SSE response exactly
      as a manually-triggered run does today — no new streaming plumbing.
- [x] 4.2 Update `web/src/lib/tailor/autopilot.ts`'s `openingActions()` / the empty-chat rendering so
      the two-action menu is skipped specifically when `cold_start_running` was true on bootstrap;
      re-opening an existing CV (`?cv=<id>`) keeps today's resume behavior unchanged.

## 5. Frontend: live document refresh during a run

- [x] 5.1 In `AssistantChat.svelte`'s `tool_result` handling (`~516-597`), add a callback fired when
      the resolved tool is `cv_edit`, calling the same `loadCv()` that `onTurnComplete`
      (`+page.svelte:493-504`) already calls.
- [x] 5.2 Leave `onTurnComplete`'s end-of-turn refresh as the safety net, unchanged.

## 6. Job Match tab

- [x] 6.1 Update the tab to show an in-progress state while a cold-start-triggered autopilot run's
      inline analysis step (task 2.1) is pending, and render the analysis once it lands — no
      client-triggered stream, no retry loop.

## 7. Verification

- [x] 7.1 `go vet -tags=integration ./...` and `go test -tags=integration ./internal/handler/...`
      (bootstrap, autopilot, and cv_tailor call `newCVHandlers`/similar unexported constructors).
- [x] 7.2 Manual QA: first-time `/tailor/[slug]` open for a vacancy builds a curated CV live, the Job
      Match tab populates without any action, and no "analyzing" screen appears.
- [x] 7.3 Manual QA: re-opening an existing tailored CV (`?cv=<id>`) is unaffected — no auto-trigger,
      no two-action menu change.
- [x] 7.4 Manual QA: simulate an autopilot/analysis failure (e.g. no LLM configured) and confirm the
      candidate lands on the unfiltered seeded CV with no error state blocking the workspace.
