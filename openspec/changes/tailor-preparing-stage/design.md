## Context

Full rationale, alternatives considered, and the exploration that led here
already exist in `docs/superpowers/specs/2026-08-11-tailor-preparing-stage-design.md`
(brainstormed and approved with the user before this OpenSpec change was
created). This document summarizes the same decisions in OpenSpec's shape;
treat the superpowers spec as the canonical detail if the two ever diverge.

PR #1754 places a tailored vacancy on the Tracking Kanban by setting
`stage = "applied"` (no `applied_at`) when none exists. That exact shape
(`stage=applied`, `applied_at IS NULL`) already occurs today whenever a
candidate manually drags a card into the Applied column —
`JobBoard.svelte` calls `api.trackApplication(item.id, { stage: 'applied' })`,
which never touches `applied_at`. The two cases are indistinguishable at the
data level, so a UI signal keyed on `applied_at` nullness would mislabel a
candidate's own manual, undated confirmation as "not sent yet."

## Goals / Non-Goals

**Goals:**

- A vacancy with only a tailored CV in progress renders in its own
  "Preparing" Kanban column, never conflated with "Applied".
- The moment a real application is recorded (`MarkApplied` /
  `MarkAppliedAt` / `MarkAppliedOn`, including the mail-reconstruction path),
  the card promotes itself from Preparing to Applied automatically.
- No stage at or beyond `applied` is ever regressed by this change.
- The ledger event that places a vacancy in Preparing is attributed to the
  platform (`SourceSystem`), not the candidate.
- Rows PR #1754 already wrote (once it merges) are healed by a backfill
  where the evidence supports it.

**Non-Goals:**

- No change to the pre-existing ambiguity of a manually-dragged, undated
  "Applied" card — out of scope.
- No new endpoint: `preparing` is a normal member of the stage vocabulary,
  settable like any other stage through the existing generic
  `PATCH /me/tracking/:slug`.
- No schema change to `applications.stage` — it stays untyped `text`; no
  `CHECK` constraint exists today and none is introduced.

## Decisions

### 1. New stage `preparing`, own group, explicit rank 0

`internal/userjob/stages.go` gets `preparing` first in `Stages`;
`groups.go` gets a new first `Group{ID: "preparing", Label: "Preparing",
Stages: []string{"preparing"}}`; `pipeline.go`'s `activeRank` gets
`"preparing": 0`.

The explicit rank matters even though it's the same value Go's map zero
value would give: `TestEveryStageIsRankedOrTerminal` exists specifically to
catch a stage present in `Stages` but silently unranked (the exact historical
bug `pipeline.go`'s own doc comment names) — `preparing` needs a *real* entry,
not an accidental default that happens to match.

`silence.go` needs no change: `SilenceThresholdDays` returns `(0, false)` for
any key absent from `silenceThresholds` (same path terminal stages take), and
`TrackedJob.Silence()` separately short-circuits to `nil` whenever
`AppliedAt == nil` — guaranteed for every `preparing` row. Two independent
reasons the silence timer cannot start; neither requires new code.

**Alternatives considered:** a card-level badge derived from `applied_at`
nullness (rejected — collides with the manual-drag case, see Context); a
stage nested inside the existing `applied` group with no new column
(rejected — the whole point is that "in prep" and "applied" are different
pipeline states, not just different card decorations, per the approved
design).

### 2. Auto-promotion lives in `MarkJobApplied`'s SQL, not a second write path

`internal/db/queries/user_jobs.sql`'s `MarkJobApplied` upsert changes:

```sql
-- before
stage = COALESCE(applications.stage, 'applied')

-- after
stage = CASE
    WHEN applications.stage IS NULL OR applications.stage = 'preparing' THEN 'applied'
    ELSE applications.stage
END
```

`MarkJobApplied` already runs inside the transaction that owns every other
side effect of a real apply (`applied_count` bump, the ledger's `applied`
event) — the query's own doc comment warns that a second, separately-decided
write path for the same transition would eventually disagree with this one.
Promoting here means it happens automatically the instant a real signal
(candidate action or reconstructed mail) fires, with zero new call sites.

### 3. `EnsureOnBoard` sets `preparing` and the correct event source

`internal/handler/cv.go`'s `trackingBoarder.EnsureOnBoard` sets
`stage = "preparing"` instead of `"applied"`, and its `TrackJob` call passes
`appevent.SourceSystem` instead of `appevent.SourceUser`. `appevent.go`
documents `SourceSystem` as "a stage change the platform made on the
candidate's behalf, not one anybody typed" — exactly this case, and the same
tag `internal/nudge` already uses for its own automatic stage change. The
"never overwrite an existing stage" guard is unchanged.

### 4. Backfill by evidence, not by ledger source alone

```sql
UPDATE applications a
   SET stage = 'preparing'
 WHERE a.stage = 'applied'
   AND a.applied_at IS NULL
   AND EXISTS (
     SELECT 1 FROM cvs c
      WHERE c.user_id = a.user_id AND c.job_id = a.job_id AND c.job_id IS NOT NULL
   );
```

"Applied, no date, AND a tailored CV exists for exactly this (user, job)" is
the strongest available evidence that a row was PR #1754's placement rather
than a manual undated drag. Rejected: backfilling from
`application_events.source` alone — rows written before the SourceSystem fix
ships still carry the (wrong) `SourceUser` tag, so the ledger cannot
distinguish them retroactively; the `cvs` join is required regardless and is
sufficient by itself.

## Risks / Trade-offs

- **[Funnel gains a 5th band]** → `PipelineFunnel.svelte`'s "four groups" doc
  comment becomes stale text; fix as part of implementation. The SVG
  geometry already iterates `PIPELINE_BANDS` rather than a hard-coded count,
  but must be checked in a browser once a 5th band exists — unproven at 5.
- **[Ambiguous backfill]** → a genuinely-applied-but-undated row could be
  mislabeled Preparing by the heuristic in Decision 4; accepted because it
  self-heals on the next real apply signal (Decision 2).
- **[`preparing` is generically settable]** → any caller of the existing
  generic stage-set endpoint can set `stage=preparing` on any job, not only
  via tailoring. Accepted: same blast radius as every other stage in the
  vocabulary already being user-settable by hand.

## Migration Plan

1. Ship the vocabulary changes, the `MarkJobApplied` SQL change (regenerate
   sqlc), and the `EnsureOnBoard` change in one deploy — they are only
   mutually consistent together.
2. Regenerate frontend contracts (`cmd/gen-contracts`).
3. Ship the backfill migration in the same release, in either order relative
   to the code deploy — its `WHERE` clause matches only the shape the *old*
   `EnsureOnBoard` wrote, never anything the new code produces.
4. Verify: a fresh tailor bootstrap lands in Preparing; a real apply signal
   on that vacancy moves it to Applied; an `interview`-staged vacancy is
   untouched by a tailor reopen.
5. Rollback: revert the SQL/Go changes; leftover `stage='preparing'` rows are
   harmless orphans until the next deploy reintroduces the promotion path —
   no data loss, no further migration to roll back.

## Open Questions

- **Sequencing against PR #1754.** That PR is not yet merged. This change
  could instead be folded into PR #1754 directly (ship `preparing` and
  `SourceSystem` from the start, skip the backfill and the temporary window
  where the `SourceUser` misattribution reaches prod) rather than landing as
  a follow-up. Left open for the user/reviewer to decide at implementation
  time; the tasks in this change assume the follow-up sequencing (PR #1754
  merges as-is first) unless redirected.
