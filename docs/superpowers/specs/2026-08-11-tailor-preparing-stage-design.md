## Context

PR #1754 ("Put tailored vacancies on the Tracking Kanban") makes starting or
reopening CV tailoring for a vacancy place it on the Tracking Kanban: bookmark
plus `stage = "applied"` when no stage exists yet, `applied_at` left unset so
tailoring is never mistaken for a submitted application.

Review of that PR surfaced a real ambiguity: `stage = "applied"` with
`applied_at IS NULL` is **not** a signal unique to tailoring. Dragging a card
into the Applied column on the board already produces exactly that state
today (`JobBoard.svelte` calls `api.trackApplication(item.id, { stage:
'applied' })`, which never touches `applied_at`). So a candidate who manually
confirms "yes, I applied" but doesn't bother dating it is indistinguishable,
at the data level, from one who only opened a tailoring workspace. Any UI
badge keyed off `applied_at` nullness would mislabel the first case as "not
sent yet."

This spec designs the long-term-correct fix: a distinct `preparing` stage
that tailoring alone produces, in its own Kanban column ahead of Applied,
which auto-promotes to `applied` the moment a real apply signal fires. It
supersedes reusing `stage = "applied"` for the tailoring side-effect.

Also rolled in: `internal/handler/cv.go`'s `trackingBoarder.EnsureOnBoard`
currently tags its automatic stage-set with `appevent.SourceUser`, which the
`appevent` package documents as "the two ways a candidate records something
themselves." An automatic platform action belongs under `appevent.SourceSystem`
instead (exactly what `internal/nudge` already does for its own automatic
stage change) — this PR fixes that misattribution at the same call site.

## Goals / Non-Goals

**Goals:**

- A vacancy that only has a tailored CV in progress renders in its own
  "Preparing" Kanban column, never in "Applied".
- The moment the candidate (or reconstructed mail) confirms a real
  application, the card promotes itself from Preparing to Applied
  automatically — no manual drag required.
- A stage more advanced than `applied` is never regressed by any of this.
- The ledger event that places a vacancy in Preparing is attributed to the
  platform (`SourceSystem`), not the candidate.
- Existing rows already written by PR #1754's `stage=applied`/`applied_at
  IS NULL` shape are healed by a one-time backfill where the evidence
  supports it.

**Non-Goals:**

- No change to how a manually-dragged, undated "Applied" card behaves — that
  ambiguity predates this change and is out of scope.
- No new UI to let a candidate manually set `stage = "preparing"` themselves
  beyond what the generic stage-vocabulary API already allows (it becomes a
  normal member of `Stages`, so the existing `PATCH /me/tracking/:slug`
  already accepts it like any other stage — no new endpoint).
- No change to `internal/db/queries/jobs.sql` schema — `applications.stage`
  stays untyped `text`, no CHECK constraint exists today and none is added.

## Decisions

### 1. New stage `preparing`, own Kanban group, rank 0

**Choice:**

- `internal/userjob/stages.go`: add `"preparing"` to `Stages`, first in
  pipeline order.
- `internal/userjob/groups.go`: add
  `{ID: "preparing", Label: "Preparing", Stages: []string{"preparing"}}` as
  the first entry of `Groups` (ahead of `applied`); add
  `"preparing": "Preparing"` to `stageLabels`.
- `internal/userjob/pipeline.go`: add `"preparing": 0` to `activeRank`
  (below `applied`'s `1`).

**Why:** `TestEveryStageIsRankedOrTerminal` exists specifically because a
stage present in `Stages` but absent from both `activeRank` and
`terminalStages` silently defaults to rank 0 via Go's map zero value and
becomes mail-classification-unreachable in a way nothing catches — the
pipeline.go doc comment names this as a real historical bug. `preparing`
must therefore get an *explicit* rank rather than relying on the same
zero-value default, even though the numeric value is the same either way.

**Alternatives considered:** Badge derived from `applied_at` nullness (killed
by the drag-to-Applied finding above — it isn't a clean signal). A stage
nested inside the existing Applied group with only a card-level label (no new
column) — rejected: the whole point is that "in prep" and "applied" are
different funnel/pipeline states, not just different card decorations.

### 2. `silence.go` needs no change

**Choice:** Do not add `preparing` to `silenceThresholds`.

**Why:** `SilenceThresholdDays` looks up the map and returns `(0, false)` for
any key it doesn't hold — the same path terminal stages take — so
`preparing` never accrues silence purely by omission. Belt-and-suspenders:
`jobtracking.TrackedJob.Silence()` also short-circuits to `nil` whenever
`AppliedAt == nil`, which is guaranteed for every `preparing` row (tailoring
never sets `applied_at`). Two independent reasons the timer cannot start;
no code required for either.

### 3. Auto-promotion `preparing → applied` lives in `MarkJobApplied`'s SQL

**Choice:** In `internal/db/queries/user_jobs.sql`, change `MarkJobApplied`'s
upsert from:

```sql
stage = COALESCE(applications.stage, 'applied')
```

to:

```sql
stage = CASE
    WHEN applications.stage IS NULL OR applications.stage = 'preparing' THEN 'applied'
    ELSE applications.stage
END
```

**Why:** `MarkJobApplied` is the one statement behind `MarkApplied` /
`MarkAppliedAt` / `MarkAppliedOn` — every path that records a real, dated (or
now-dated) application, including the mail-reconstruction path. Promoting
`preparing → applied` here, and nowhere else, means the promotion happens
automatically at the exact moment the candidate's action or mail evidence
says the application became real, and any stage at or beyond `applied`
(`screening`, `interview`, …) is left untouched by the `ELSE` branch — no
regression risk.

**Alternative considered:** Have `EnsureOnBoard` or a new service method
poll/promote. Rejected: `MarkJobApplied` already runs inside the locked
transaction (`LockJobForApply`) that owns every other applied-transition
side effect (`applied_count` bump, the `applied` ledger event) — adding a
second write path for the same transition is exactly the kind of drift the
query's own doc comment (`internal/db/queries/user_jobs.sql`) already warns
against ("two records of one transition, decided separately, would
eventually disagree").

### 4. `EnsureOnBoard` sets `preparing`, and its event source becomes `SourceSystem`

**Choice:** In `internal/handler/cv.go`, `trackingBoarder.EnsureOnBoard`:

```go
func (s trackingBoarder) EnsureOnBoard(ctx context.Context, userID, jobID int64) error {
	row, err := s.repo.SaveJob(ctx, userID, jobID)
	if err != nil {
		return err
	}
	if row.Stage != nil && *row.Stage != "" {
		return nil
	}
	stage := "preparing"
	_, err = s.repo.TrackJob(ctx, userID, jobID, &stage, nil, appevent.SourceSystem)
	return err
}
```

Only the stage literal and the event source change; the "never overwrite an
existing stage" guard is untouched.

**Why (source):** `appevent.go` documents `SourceSystem` as "a stage change
the platform made on the candidate's behalf, not one anybody typed" — opening
a tailoring workspace is exactly that, and `internal/nudge` already uses
`SourceSystem` for its own automatic stage change (auto-expiring a closed
listing). `SourceUser` is documented as "the two ways a candidate records
something themselves," which this is not.

### 5. Backfill for rows PR #1754 already wrote

**Choice:** A one-shot DML migration:

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

**Why:** "Applied with no date" is ambiguous between a manual undated drag
and a tailoring auto-placement, but "Applied with no date, AND a tailored CV
exists for exactly this (user, job)" is the strongest available evidence for
the latter. It is not provably exact — a candidate could have both dragged a
card *and* independently tailored a CV for the same role — but it is a
one-time correction, not standing logic, and the failure mode (a genuinely
applied-but-undated row gets relabeled Preparing) is self-healing: any later
`MarkApplied` call promotes it straight back per Decision 3.

**Alternative considered:** Backfill from `application_events.source`, since
the tailoring-originated events are (post-fix) tagged `SourceSystem`.
Rejected as the sole signal: rows written before this fix ships carry the
old, wrong `SourceUser` tag, so the ledger alone cannot distinguish them
retroactively — the `cvs` join is required regardless, and once required, is
sufficient on its own.

## Risks / Trade-offs

- **[Funnel gains a 5th band]** → `PipelineFunnel.svelte`'s doc comment
  ("one Applications source fanning into the four groups") becomes stale
  text, fixed as part of implementation. The SVG geometry already iterates
  `PIPELINE_BANDS` rather than hard-coding a count, but must be eyeballed in
  a browser once a 5th band exists — the ribbon math has not been visually
  proven at 5.
- **[Ambiguous backfill]** → accepted per Decision 5; self-healing on the
  next real apply signal.
- **[`preparing` is a generic vocabulary member]** → any caller of the
  existing generic `PATCH /me/tracking/:slug` (stage-only, no new endpoint)
  can now set `stage = "preparing"` on any job themselves, not only via
  tailoring. Accepted: harmless (same blast radius as setting any other
  existing stage by hand), and consistent with how every other stage in the
  vocabulary is already user-settable.

## Migration Plan

1. Ship the `internal/userjob` vocabulary changes, the `MarkJobApplied` SQL
   change (regenerate sqlc), and the `EnsureOnBoard` change together — they
   are only consistent as one deploy (a "preparing" row written before
   `MarkJobApplied` knows to promote it would strand undated, though it
   self-heals on the next real apply signal regardless of deploy order).
2. Regenerate frontend contracts (`cmd/gen-contracts`) so `STAGE_GROUPS` /
   `PIPELINE_BANDS` picks up the new group without hand-editing generated
   files.
3. Ship the backfill migration in the same release, in either order relative
   to the code deploy: its `WHERE` clause only matches the shape the *old*
   `EnsureOnBoard` wrote (`stage='applied'` with no `applied_at`, alongside a
   tailored CV), never anything the new code produces, so running it before
   or after the deploy is equally safe.
4. Verify visually: a fresh tailor bootstrap lands the vacancy in Preparing;
   `MarkApplied`/a reconstructed-mail apply on that same vacancy moves it to
   Applied; an already-`interview`-staged vacancy is untouched by a tailor
   reopen.
5. Rollback: revert the SQL/Go changes; leftover `stage = 'preparing'` rows
   are harmless orphans (present in `Stages` but produced by nothing) until
   the next deploy reintroduces the promotion path — no data loss, no
   further migration needed to roll back.

## Testing

- `internal/userjob`: existing `TestEveryStageBelongsToExactlyOneGroup` and
  `TestEveryStageIsRankedOrTerminal` must keep passing with `preparing`
  added — they are the guardrails this design relies on, not new tests.
- `internal/handler` integration (extends `cv_tailor_integration_test.go`):
  first tailor bootstrap leaves `stage = "preparing"`, `saved_at` set,
  `applied_at` unset; a subsequent `MarkApplied`-equivalent call (or direct
  repository call in the test) promotes it to `applied` without a second
  tailor call being involved; an existing `interview` stage survives a
  tailor reopen unchanged (already covered, re-verify it still holds).
- `internal/jobtracking` or `internal/db` integration: `MarkJobApplied`
  promotes `preparing → applied` and leaves any stage ranked at or above
  `applied` untouched.
- New assertion on the ledger: the `stage_set` event written by
  `EnsureOnBoard` carries `source = "system"`.
- Manual/one-off verification of the backfill query against a snapshot
  before it ships to prod (per this repo's own migration conventions).
