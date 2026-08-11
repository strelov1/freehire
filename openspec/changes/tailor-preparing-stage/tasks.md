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

- [x] 2.1 In `internal/db/queries/user_jobs.sql`, change `MarkJobApplied`'s upsert `stage` clause
      from `COALESCE(applications.stage, 'applied')` to the `CASE` that also promotes an
      existing `'preparing'` stage to `'applied'`, leaving any other existing stage untouched.
- [x] 2.2 Regenerated sqlc (`make sqlc`); `go build ./...` clean.
- [x] 2.3 Integration test added in `internal/db/user_jobs_stage_integration_test.go`
      (`TestTrackJobAndStageSeeding/MarkApplied_promotes_a_preparing_stage_to_applied`):
      `preparing` promotes to `applied` with `applied_at` set. Exercised via `q.MarkJobApplied`
      directly rather than once per `MarkApplied`/`MarkAppliedAt`/`MarkAppliedOn` — all three
      share this exact statement (`internal/jobtracking/repository.go`'s `markApplied` helper),
      differing only in the `at` timestamp they pass, so one test at the SQL layer covers the
      promotion logic for all three; the existing `TestMarkAppliedAt_PassesTheDateThrough` /
      `TestMarkAppliedOn_*` suites (unrelated to this change, still green) confirm the date
      handling per variant is unaffected.
- [x] 2.4 Integration test already added alongside 2.3
      (`.../MarkApplied_does_not_regress_a_stage_beyond_applied`): an application already at
      `interview` (a stage ranked above `applied`) is left unchanged by a `MarkApplied` call.

## 3. Tailor bootstrap sets `preparing`, correct event source

- [x] 3.1 In `internal/handler/cv.go`, changed `trackingBoarder.EnsureOnBoard` to set
      `stage := "preparing"` instead of `"applied"`.
- [x] 3.2 In the same method, changed the `TrackJob` call's event source from
      `appevent.SourceUser` to `appevent.SourceSystem`.
- [x] 3.3 Updated `cv_tailor_integration_test.go`'s `assertVacancyOnKanban` to expect
      `"preparing"` in both the direct row read and the `filter=board` listing.
- [x] 3.4 Extended `assertVacancyOnKanban` itself (called by both `TestTailorCVBootstrap` and
      `TestTailorCVBootstrap_HealsMissingSave`) with a query against `application_events` for
      the vacancy's most recent `stage_set` event, asserting `source = 'system'`. RED confirmed
      first (`source = "user"`, `stage = "applied"`) before the `cv.go` fix, GREEN after.
- [x] 3.5 `TestTailorCVBootstrap_HealsMissingSave`'s "an advanced stage survives a tailor
      reopen" assertion needed no change — it already keyed on `"interview"`, not `"applied"`.
      Confirmed still green.

## 4. Frontend

- [x] 4.1 Regenerated frontend contracts, but by hand-editing `generated/contracts.ts`'s three
      `preparing`-related lines rather than running `go run ./cmd/gen-contracts` verbatim: a full
      regen pulled in unrelated drift (new sources, role aliases, a matchanalysis `Bounds` type
      from other already-merged PRs this branch's `main` base predates) that isn't this task's to
      carry. The three lines match exactly what a full regen produces for `STAGE_VALUES`,
      `STAGE_LABELS`, and `STAGE_GROUPS`.
- [x] 4.1a (found running `pnpm run check`, not planned) `JobBoard.svelte` failed to compile:
      two `Record<BoardColumnId, BoardItem[]>` object literals (`emptyColumns()`, the `shown`
      search-filter branch) were missing the new `preparing` key — exactly the type error
      `tracking-stage-vocabulary`'s own spec exists to force. Fixed both.
- [x] 4.1b (found by inspection, not planned) `persistMove`'s `switch (to)` in `JobBoard.svelte`
      had no `case 'preparing'`: dragging a card into the new column would have updated local
      state optimistically and then silently done nothing on the backend (no `default` case to
      catch it, so `pnpm run check` couldn't see this one — a switch gap, not a type gap). Added
      the case.
- [x] 4.1c (found by inspection, not planned) `pipeline.ts`'s `BAND_COLORS` had no entry for
      `preparing`, so it fell back to the same colour as `applied` (`?? '#cbd5e1'`) — two adjacent
      Sankey bands would have rendered indistinguishable. Added a distinct colour (`#fde68a`).
- [x] 4.1d (found running `pnpm test`, not planned) Three vitest suites pin the generated
      group/stage order as a literal array (`board.test.ts`'s `BOARD_COLUMNS`,
      `pipeline.test.ts`'s `PIPELINE_BANDS`, `stages.test.ts`'s `groupedStages`) — the same
      deliberate-pin pattern as Go's `TestStagesOrder`. Updated all three to lead with
      `'preparing'`.
- [x] 4.2 Updated `PipelineFunnel.svelte`'s doc comment ("one Applications source fanning into
      the four groups") to name five groups and note the count is whatever `STAGE_GROUPS`
      currently holds, not a fixed number.
- [x] 4.2a (found in code review, not planned) Two hand-typed vocabulary lists outside
      generation's reach — `web/src/lib/docs/api-spec.ts`'s `PATCH /jobs/{slug}/track`
      description and `CliView.svelte`'s CLI help text — both named the stage list without
      `preparing`. Neither is caught by `pnpm run check` (plain strings, not typed against
      `Stage`). Added it to both.
- [ ] 4.3 NOT DONE as originally scoped. No `preparing` row exists anywhere yet — its only
      producer (`EnsureOnBoard`, task 3) is deferred behind PR #1754 — so there is no real card
      to open the Tracking board and look at; seeding one would mean faking data the app cannot
      yet produce itself. What IS verified: `pnpm run check` (0 errors) proves no card can render
      in an unhandled state; the vitest suite (916 passed, including "every band gets a real
      colour") proves the funnel math and the colour table are complete for 5 bands; reading
      `PipelineFunnel.svelte`'s geometry code (task group 4 investigation) confirms the SVG height
      and ribbon layout are computed from `visible.length`, not a hard-coded band count. A real
      pixel-level look, per this repo's own "test the UI in a browser" convention, still needs
      task 3 done first.

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
