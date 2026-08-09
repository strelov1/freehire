Full code snippets, exact test bodies, and file:line targets for every task below are written
out in `docs/superpowers/plans/2026-08-09-fit-analysis-post-autopilot-verify.md` (Tasks 1-5) —
use it as the reference during `/opsx:apply`; this checklist tracks the same work at task
granularity.

## 1. Prerequisite refactor

- [ ] 1.1 Extract `buildAnalysisInput` out of `runAnalysis` in `internal/handler/match_analysis.go` (pure, behavior-preserving — no new test needed, existing `TestMatchAnalysisEndpoints` is the safety net)
- [ ] 1.2 Run `go test -tags=integration ./internal/handler/ -run TestMatchAnalysisEndpoints -v` and confirm PASS
- [ ] 1.3 Run `go build ./...` and `go vet -tags=integration ./...`, confirm clean

## 2. Guaranteed post-run fit-analysis refresh

- [ ] 2.1 Rewrite `TestAutopilotReusesAnExistingCachedAnalysis` in `internal/handler/assistant_autopilot_integration_test.go` as `TestAutopilotRefreshesAnalysisAfterEveryRun`, asserting the analysis is recomputed (not skipped) even when already cached
- [ ] 2.2 Add an `fitM.n` call-count assertion to `TestAutopilotComputesAnalysisWhenMissing` locking in "runs before AND after" (6 calls: 3 pre-run + 3 post-run)
- [ ] 2.3 Run both tests, confirm they FAIL against today's code
- [ ] 2.4 Add `prepareAutopilotRun` to `internal/handler/match_analysis.go` (returns a closure over a pre-built `Input`/`Analyzer`, unconditional recompute + overwrite, never debits credits)
- [ ] 2.5 Replace `ensureAnalysisForRun` with `prepareAutopilotAnalysis` in `internal/handler/assistant.go`, and rewire `PostAssistantAutopilot` to call `streamSSE` directly with a `start` closure that runs the refresh after `runner.Run` returns
- [ ] 2.6 Run the two tests from 2.1/2.2 again, confirm PASS
- [ ] 2.7 Run the full autopilot integration suite (`go test -tags=integration ./internal/handler/ -run TestAutopilot -v`) for regressions
- [ ] 2.8 Run `go vet -tags=integration ./...`

## 3. Autopilot self-check prompt

- [ ] 3.1 Add `TestTailorPromptSelfChecksWithJobMatchBeforeReporting` to `internal/assistant/prompt_test.go`, asserting the `UNATTENDED RUNS` section mentions `job_match`/`missing_have`/`missing_gap` before `tailor_report`
- [ ] 3.2 Run it, confirm FAIL
- [ ] 3.3 Add the self-check bullet to `tailorPrompt`'s `UNATTENDED RUNS` section in `internal/assistant/prompt.go`, between the "ask nothing" bullet and the "finish by calling tailor_report" bullet
- [ ] 3.4 Run it, confirm PASS
- [ ] 3.5 Run `go test ./internal/assistant/...` for regressions (in particular `TestTailorPromptDescribesTheUnattendedRun`)

## 4. Job Match tab copy

- [ ] 4.1 Update the three comments and the on-screen copy in `web/src/lib/tailor/ArtifactPanel.svelte` (header comment, `autopilotReport` prop doc, and the Fit analysis panel's text + its comment) to stop calling the fit analysis a frozen snapshot
- [ ] 4.2 `grep -rn "snapshot" web/src/lib/tailor/ web/src/lib/components/MatchAnalysisFull.svelte` and confirm no leftover references
- [ ] 4.3 Visual check: `cd web && pnpm dev`, open a tailored CV's Job Match tab, confirm the new copy renders
