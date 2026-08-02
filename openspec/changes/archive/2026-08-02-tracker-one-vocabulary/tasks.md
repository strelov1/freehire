## 1. The vocabulary gains its group table

- [x] 1.1 Write the failing binding test in `internal/userjob`: every stage in `Stages` belongs
      to exactly one entry of `Groups`, and every stage named by a group exists in the
      vocabulary. Verify it fails by mutation (drop a stage from the table).
- [x] 1.2 Add `Group`, `Groups`, `GroupOf` and `Label` to `internal/userjob` beside `activeRank`
      and `terminalStages`, with the four groups in pipeline order.
- [x] 1.3 Move the stage labels out of `web/src/lib/stages.ts` into the Go table as the single
      definition; leave the TS module reading generated values only.

## 2. The endpoint returns per-stage counts

- [x] 2.1 Change `internal/handler/me_pipeline_integration_test.go` to assert the `stages`
      envelope, every stage key present, and the counts summing to `applications`. Watch it
      fail.
- [x] 2.2 Return per-stage counts from `internal/jobtracking` (the existing `CountMyJobsByStage`
      query already groups by stage; the folding step is what goes away).
- [x] 2.3 Change `internal/handler/me_tracking.go` to serialize `stages` and drop `buckets`.
- [x] 2.4 Delete `internal/userjob/buckets.go` and `buckets_test.go` — `BucketCounts`,
      `Pipeline`, `Aggregate`. Confirm nothing references them.

## 3. Generation carries the vocabulary to the SPA

- [x] 3.1 Emit `STAGE_LABELS` and `STAGE_GROUPS` from `cmd/gen-contracts` beside the existing
      `STAGE_VALUES`; regenerate `web/src/lib/generated/contracts.ts`.
- [x] 3.2 Add the type-level check that `STAGE_GROUPS` covers every `STAGE_VALUES` entry, in the
      `satisfies Record<K, V>` form that runs in the required `pnpm run check` gate. Verify it
      fails by mutation (remove a stage from the emitted groups).

## 4. Every surface reads the generated tables

- [x] 4.1 `web/src/lib/board.ts`: build `BOARD_COLUMNS` and `columnOf` from `STAGE_GROUPS`,
      deleting the local `STAGE_COLUMN`. Keep the saved-only → no-column behaviour and its test.
- [x] 4.2 `web/src/lib/pipeline.ts`: derive the funnel's bands from `STAGE_GROUPS`, delete
      `PIPELINE_BUCKETS`, and compute `interviewRate` / `offerRate` from per-stage counts.
- [x] 4.3 `PipelineFunnel.svelte`: render four bands, each with its per-stage breakdown.
- [x] 4.4 `HomeFunnel.svelte`: replace the fourth copy of the bucket vocabulary with the
      generated groups, keeping its hardcoded marketing numbers.
- [x] 4.5 `JobDrawer.svelte`: group the stage selector's options with `<optgroup>` by the four
      groups.
- [x] 4.6 Update `web/src/lib/types.ts` for the new pipeline response shape.

## 5. Mail says what a signal implies

- [x] 5.1 Export `StageFor(sig) (stage string, advances bool)` from `internal/mailclassify` in
      place of the private `signalStage`, keeping `AdvanceStage` behaviour identical; assert
      that with a test over every signal in the vocabulary.
- [x] 5.2 Emit the signal→stage mapping from `cmd/gen-contracts` beside
      `EMAIL_STATUS_SIGNAL_VALUES`.
- [x] 5.3 Render each message's signal and what it implies in the drawer's Emails tab —
      including the explicit "does not move the stage" case.

## 6. A disagreement is offered for resolution

- [x] 6.1 Add the `LastStageSetAt(user_id, job_id)` query to
      `internal/db/queries/application_events.sql` and run `make sqlc`.
- [x] 6.2 Write the failing service tests in `internal/jobtracking` for the suggestion rules: a
      rejection on an `interview` application offers `rejected`; an agreeing signal offers
      nothing; an unclassified message offers nothing; a `stage_set` newer than the message
      silences it; a message newer than the last `stage_set` asks again.
- [x] 6.3 Compute `stage_suggestion` in `internal/jobtracking` and carry it on the tracked
      application; serialize it in the handler.
- [x] 6.4 Show the offer in the drawer and apply it through the existing stage-change path, so
      the ledger records the candidate as the source.

## 7. Documentation and specs

- [x] 7.1 Update `docs/API.md` and `web/src/lib/docs/api-spec.ts` for the new pipeline shape.
- [x] 7.2 Update `internal/userjob/AGENTS.md` — the group table, and the removal of the bucket
      vocabulary it currently describes.
- [x] 7.3 Run both suites before pushing: `go test ./...` and `go test -tags=integration
      ./internal/db/`, plus `pnpm run check`.
- [x] 7.4 Offer a `/blog` changelog entry for the shipped change.
