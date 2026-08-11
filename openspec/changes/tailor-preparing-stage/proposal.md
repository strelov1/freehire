## Why

PR #1754 places a vacancy on the Tracking Kanban when CV tailoring starts, by
setting `stage = "applied"` (with `applied_at` left unset) when no stage
exists yet. That shape is not unique to tailoring: dragging a card into the
Applied column on the board already produces the identical
`stage=applied`/`applied_at=NULL` state today. A candidate who only opened a
tailoring workspace is therefore indistinguishable, at the data level, from
one who manually confirmed "I applied" without dating it — any UI signal
built on `applied_at` nullness would mislabel the second case. The fix is a
dedicated stage for "in preparation, not yet submitted" that only tailoring
produces, rather than overloading an existing one.

## What Changes

- Add `preparing` to the application-stage vocabulary, as its own Kanban
  group/column, ordered ahead of `applied` (rank 0).
- CV tailoring's board-placement side effect (`EnsureOnBoard`) sets
  `stage = "preparing"` instead of `stage = "applied"` when no stage exists
  yet.
- `MarkJobApplied` (backing `MarkApplied` / `MarkAppliedAt` / `MarkAppliedOn`
  — every path that records a real, dated application) auto-promotes an
  existing `preparing` stage to `applied` the moment a genuine apply signal
  fires. Any stage already at or beyond `applied` is left untouched.
- The ledger event `EnsureOnBoard` writes is attributed to
  `appevent.SourceSystem` (a platform-automatic action) instead of
  `appevent.SourceUser`, correcting a misattribution — matches how
  `internal/nudge` already tags its own automatic stage change.
- One-time backfill migration for `applications` rows already written by the
  shipped PR #1754 shape (`stage='applied'`, `applied_at IS NULL`, a
  tailored CV exists for that vacancy) to `preparing`.
- Frontend contracts regenerated so the board and pipeline funnel pick up
  the new group without a hand-written second copy.

## Capabilities

### New Capabilities

<!-- none -->

### Modified Capabilities

- `tracking-stage-vocabulary`: the pipeline-ordered group list gains
  `preparing` (stages: `preparing`) ahead of `applied`, with an explicit
  `activeRank` entry so the vocabulary's own "ranked or terminal" invariant
  holds for it.
- `user-job-tracking`: recording a real application (`MarkApplied` /
  `MarkAppliedAt` / `MarkAppliedOn`) now promotes an existing `preparing`
  stage to `applied` as part of the same upsert, instead of leaving any
  pre-existing stage untouched.
- `cv-tailoring`: the Kanban board-placement side effect from PR #1754 sets
  `stage = "preparing"` rather than `stage = "applied"`, and its ledger event
  source is `system` rather than `user`. **This delta depends on PR #1754
  (OpenSpec change `tailor-adds-to-tracking`) merging first** — the base
  requirement it modifies does not exist in `openspec/specs/cv-tailoring`
  until that change lands; this proposal describes the target end-state.

## Impact

- `internal/userjob/{stages,groups,pipeline}.go` — vocabulary changes.
- `internal/db/queries/user_jobs.sql` (`MarkJobApplied`) + regenerated sqlc.
- `internal/handler/cv.go` (`trackingBoarder.EnsureOnBoard`).
- A new migration for the one-time backfill.
- `cmd/gen-contracts` output consumed by `web/src/lib/generated/contracts`,
  and `web/src/lib/components/PipelineFunnel.svelte`'s stale "four groups"
  doc comment.
- Depends on PR #1754 merging first (see Modified Capabilities note on
  `cv-tailoring`); no other change is a prerequisite.
- Full rationale, alternatives considered, and migration/testing plan already
  written in `docs/superpowers/specs/2026-08-11-tailor-preparing-stage-design.md`
  (brainstormed and approved with the user before this change was created).
