## 1. Stage vocabulary (`internal/userjob`)

- [x] 1.1 Add `"preparing"` to `Stages` in `stages.go`, first in pipeline order.
- [x] 1.2 Add `{ID: "preparing", Label: "Preparing", Stages: []string{"preparing"}}` as the
      first entry of `Groups` in `groups.go`; add `"preparing": "Preparing"` to `stageLabels`.
- [x] 1.3 Add `"preparing": 0` to `activeRank` in `pipeline.go` (below `applied`'s `1`).
- [x] 1.3a (found running the suite, not planned) Add `"preparing": 21` to
      `silenceThresholds` in `silence.go` — `TestSilenceThresholdsCoverExactlyTheActiveStages`
      requires every ranked stage to have a threshold entry; see design.md Decision 1's
      "Revised during implementation" note for why the value is inert in practice.
- [x] 1.4 Ran the existing `TestEveryStageBelongsToExactlyOneGroup`,
      `TestEveryStageIsRankedOrTerminal`, and `TestSilenceThresholdsCoverExactlyTheActiveStages` —
      all pass. `TestStagesOrder`'s pinned literal list needed a deliberate update (new stage,
      new order) — not a weakened invariant, the same kind of one-line diff the test exists to
      force on any vocabulary change.
- [x] 1.5 (found in code review, not planned) Updated `internal/userjob/AGENTS.md` — the
      controlled-vocabulary list, the "four groups" line, and the silence-threshold provenance
      paragraph all named stale counts/lists that this task orphaned.

## 2. Auto-promotion on a real apply signal

- [ ] 2.1 In `internal/db/queries/user_jobs.sql`, change `MarkJobApplied`'s upsert `stage` clause
      from `COALESCE(applications.stage, 'applied')` to the `CASE` that also promotes an
      existing `'preparing'` stage to `'applied'`, leaving any other existing stage untouched.
- [ ] 2.2 Regenerate sqlc (`make sqlc`) and confirm the generated code compiles.
- [ ] 2.3 Integration test: an application at stage `preparing` is promoted to `applied` by
      `MarkApplied` (and separately by `MarkAppliedAt` / `MarkAppliedOn`), with `applied_at` set
      exactly as it is for a job with no prior stage.
- [ ] 2.4 Integration test: an application already at `interview` (or any stage ranked at or
      above `applied`) is left unchanged by a `MarkApplied` call.

## 3. Tailor bootstrap sets `preparing`, correct event source

- [ ] 3.1 In `internal/handler/cv.go`, change `trackingBoarder.EnsureOnBoard` to set
      `stage := "preparing"` instead of `"applied"`.
- [ ] 3.2 In the same method, change the `TrackJob` call's event source from
      `appevent.SourceUser` to `appevent.SourceSystem`.
- [ ] 3.3 Update `cv_tailor_integration_test.go`'s `assertVacancyOnKanban` (and any other
      assertion keyed on `stage == "applied"` for the tailor-bootstrap path) to expect
      `"preparing"` instead.
- [ ] 3.4 Add/extend an integration test asserting the `stage_set` ledger event written by
      `EnsureOnBoard` carries `source = "system"`.
- [ ] 3.5 Confirm the existing "an advanced stage survives a tailor reopen" test still holds
      (it should require no logic change, only stage-literal updates if it asserted `"applied"`
      anywhere incidentally).

## 4. Frontend

- [ ] 4.1 Regenerate frontend contracts (`cmd/gen-contracts`) so `STAGE_GROUPS` / `PIPELINE_BANDS`
      pick up the new `preparing` group and label.
- [ ] 4.2 Update `PipelineFunnel.svelte`'s doc comment ("one Applications source fanning into the
      four groups") to reflect five groups.
- [ ] 4.3 Run the app locally, open the Tracking board, and visually verify: the Preparing column
      renders correctly positioned ahead of Applied, and the pipeline funnel's five bands don't
      break the existing SVG geometry (ribbon widths/labels at the new band count).

## 5. Backfill migration

- [ ] 5.1 Add a new migration file with the one-time backfill:
      `UPDATE applications SET stage='preparing' WHERE stage='applied' AND applied_at IS NULL
      AND EXISTS (a tailored cv for that user/job)` (see design.md Decision 4 for the exact
      query and rationale).
- [ ] 5.2 Verify the migration is a no-op against a database that never ran the old
      (`stage='applied'`-on-tailor) code path.

## 6. Verification

- [ ] 6.1 `go build ./...` and `go vet ./...`.
- [ ] 6.2 `go vet -tags=integration ./...`.
- [ ] 6.3 `go test -tags=integration ./internal/handler/... ./internal/db/... ./internal/userjob/...`
      (or the narrower set covering the files touched in tasks 1-3).
- [ ] 6.4 `pnpm run check` in `web/` (the vocabulary spec's own required gate: every generated
      stage value must be covered by the generated group table).
- [ ] 6.5 Manual end-to-end pass: fresh tailor bootstrap → card in Preparing; mark that same
      vacancy applied (via the SPA's "mark applied" action or the API directly) → card moves to
      Applied; a vacancy already at `interview` → tailor reopen leaves it at `interview`.
