## Why

An application has one state, stored in one column, and the tracker names it three different
ways at once: the drawer calls it `Rejected`, the board files it under a column called
`Closed`, and the pipeline counts it in a bucket called `rejected` — while two bucket names
(`in_progress`, `declined`) appear nowhere else in the product. None of the three vocabularies
is a subset of another, and each is defined in a different file, including a fourth copy inside
`HomeFunnel.svelte`. A reader cannot tell whether they are looking at one concept or three.

A fourth vocabulary — the mail classifier's nine status signals — maps into stages through a
private table, invisible in the UI. Mail that plainly announces a rejection moves nothing (by
design: that call belongs to the candidate) and says so nowhere, so the candidate sets the
stage by hand without ever learning why.

## What Changes

- `internal/userjob` gains the group + label table, joining the three tables it already keys on
  the stage vocabulary (`activeRank`, `terminalStages`, `silenceThresholds`). One test binds
  groups to stages in both directions.
- **BREAKING**: `GET /api/v1/me/tracking/pipeline` returns per-stage counts (`stages`) and no
  longer returns `buckets`. A repo-wide grep found no consumer outside this repository.
- `internal/userjob/buckets.go` is deleted — `BucketCounts`, `Pipeline`, `Aggregate`. It is the
  third vocabulary, and the point of the change is that it stops existing.
- `cmd/gen-contracts` emits `STAGE_LABELS` and `STAGE_GROUPS` beside the existing
  `STAGE_VALUES`. The board, the funnel, the drawer's selector and `HomeFunnel.svelte` all read
  the generated tables instead of their own four copies.
- `mailclassify` exports `StageFor(signal)` in place of its private `signalStage` table, and the
  mapping is emitted into the contracts. Each message in the drawer's Emails tab shows what its
  signal implies.
- A classified linked email that disagrees with the current stage produces a one-click
  suggestion. Automatic advancement is **not** changed: a rejection still never advances by
  itself, and the suggestion is how that rule becomes visible rather than silent.

## Capabilities

### New Capabilities
- `tracking-stage-vocabulary`: one owner for the stage vocabulary, its human labels and its
  group membership; generated into the frontend contracts so no surface may hold a second copy.
- `mail-stage-suggestion`: the mail signal → stage relationship shown where mail is read, and
  the candidate-confirmed resolution when a message and the stage disagree.

### Modified Capabilities
- `application-pipeline`: the response shape changes from seven buckets to per-stage counts;
  the stage→bucket mapping requirement is replaced by stage→group membership owned by
  `internal/userjob`; the two rates are derived from stages; the funnel renders four bands with
  a per-stage breakdown instead of seven buckets.

## Impact

- **Go**: `internal/userjob` (new group table, `buckets.go` deleted), `internal/jobtracking`
  (per-stage counts, the suggestion), `internal/mailclassify` (`StageFor` exported),
  `internal/handler/me_tracking.go`, `cmd/gen-contracts`.
- **SQL**: one new query, `LastStageSetAt(user_id, job_id)`, over the existing
  `application_events` ledger. No migration — no schema change.
- **Frontend**: `web/src/lib/stages.ts`, `board.ts`, `pipeline.ts`, `PipelineFunnel.svelte`,
  `HomeFunnel.svelte`, `JobDrawer.svelte`, `web/src/lib/types.ts`.
- **Docs**: `docs/API.md`, `web/src/lib/docs/api-spec.ts`, `internal/userjob/AGENTS.md`.
- **Tests**: `internal/handler/me_pipeline_integration_test.go` asserts the new shape.
- Out of scope: tracker load performance (its own change, gated on a measurement) and the
  drawer's `Viewed → Saved → Applied` strip, which reads as a timeline without being one.
