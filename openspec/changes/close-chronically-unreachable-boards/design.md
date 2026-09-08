## Context

See `proposal.md` - Why. The relevant existing mechanics:

- `board_health` (`internal/platform/db/queries/board_health.sql`) already tracks
  `consecutive_failures`, `cooldown_until`, `last_success_at`, `last_error_at`,
  `last_run_at` per `(provider, board, region)`, created lazily on first crawl
  attempt. It has no "first seen" timestamp today.
- `CloseUnseenJobsForBoard` already closes one board's open jobs by an
  `externalid.BoardPattern(board)` LIKE-prefix on `(source, external_id)`, riding the
  `text_pattern_ops` index — the same scoping mechanism this change reuses, minus the
  "this run proved coverage" gate that makes it unsafe to use for a board that has
  never once proven coverage.
- A boardless provider (e.g. `uber`, `jobdanmark`) is tracked in `board_health` as a
  single record with `board = ''`; `boardQualifies` explicitly refuses to board-scope
  such a record (a `''` LIKE-prefix would match the provider's whole catalogue).
- `jobs.closed_reason` is text but NOT free-form: `migrations/0071_jobs_closed_reason.sql`
  added a `CHECK` constraint enumerating exactly six permitted values (`''`, `unseen`,
  `feed_removed`, `moderated`, `probe_expired`, `expired`). A new value needs its own
  migration, following 0071's own large-table remedy (`DROP` + re-`ADD ... NOT VALID` +
  `VALIDATE CONSTRAINT`, so the widen costs no `ACCESS EXCLUSIVE` scan) — caught by an
  integration test hitting `SQLSTATE 23514` before this design doc was corrected to say
  so; see `migrations/0147_jobs_closed_reason_board_unreachable.sql`.

## Goals / Non-Goals

**Goals:**
- Give a board that has proven unreachable for a long time an exit path out of "open
  forever," without touching the existing per-run sweep's conservative behavior.
- Make chronic boards visible to the existing curation pass before anything auto-closes.

**Non-Goals:**
- Not fixing individual broken adapters (paylocity/jobdanmark/oracle's specific
  boards) — that is ordinary curation work once the boards are visible, using
  whatever the per-board `last_error` says.
- Not changing `shouldSweep`, `boardQualifies`, or `sweepableCompanies` — those guard
  the per-run sweep against transient failures and are out of scope; see proposal.
- Not building a UI. Chronic boards surface through the existing log summary and
  `board_health` table, the same as unhealthy boards do today.

## Decisions

**1. Two windows, not one.** "Chronic" (default 30 days, for visibility) and
"closure" (default 60 days, for the safety net) are separate, both counted from the
same `last_success_at`/`first_seen_at` baseline. A curator who reads the chronic-board
report has a full 30-day gap to retire or fix the board before the safety net would
have closed it anyway. Both are env-configurable (`CHRONIC_BOARD_WINDOW_DAYS`,
`CHRONIC_BOARD_CLOSE_WINDOW_DAYS`), following the existing `BACKFILL_*`/`*_DAYS`
convention for one-off operational knobs. Alternative considered: one window that
both reports and closes — rejected because it gives curators no lead time and
couples a purely informational threshold to a destructive one.

**2. New `board_health.first_seen_at` column, migration-backfilled conservatively.**
A board that has *never* succeeded has no `last_success_at` to measure "chronic
since" from. `first_seen_at TIMESTAMPTZ NOT NULL DEFAULT now()` is set once on the
row's `INSERT` (both `RecordBoardSuccess` and `RecordBoardFailure`'s `ON CONFLICT DO
UPDATE` leave it untouched). For a board that has already succeeded at least once,
`last_success_at` remains the measure — `first_seen_at` is only the fallback.
Existing rows are backfilled to `now()` at migration time (not to `last_error_at` or
any other historical column), which deliberately *resets* the chronic clock for
never-succeeded boards on deploy: we have no reliable historical "first failure"
timestamp for them (`last_error_at` is overwritten every run, not append-only), and
guessing one risks understating how long ago it started. Resetting to `now()` costs
one extra chronic-window's delay before a genuinely long-broken, never-succeeded
board is flagged post-deploy — the safe direction, matching this codebase's stated
bias toward leaving a questionable job open over closing it wrongly (see
`job-lifecycle`'s sweep rationale). A board that has succeeded at least once needs no
such grace: `last_success_at` is already exact history, so `paylocity/3d3c12d8-...`
(`last_success_at` 2026-07-29) is immediately chronic on deploy, no reset.

**3. One parameterized query, two callers.** `ListChronicBoards(window interval)`
returns every `(provider, board, region)` where `last_success_at IS NOT NULL AND
last_success_at < now() - window` OR `(last_success_at IS NULL AND first_seen_at <
now() - window)`. The board-health summary calls it with the 30-day window for
reporting; the safety-net closer calls it with the 60-day window for closing. One
query, no duplicated threshold logic.

**4. Safety-net closer is a new one-off (`cmd/close-chronic-boards`), not folded into
`cmd/ingest`.** It runs independently of any provider's crawl — a chronic board by
definition is not being crawled — on its own daily systemd timer, following the
existing `cmd/backfill-*` shape (dry-run by default per repo convention for anything
touching live catalogue state, `--apply` to write). Folding it into `cmd/ingest`
would tie its cadence to a specific provider's crawl schedule for no reason; it reads
and writes only `board_health` and `jobs`, needing just `DATABASE_URL`.

**5. Board-scoped close for `board != ''`, source-scoped for `board = ''`.** A new
query `CloseChronicBoardJobs(source, board_pattern)` mirrors
`CloseUnseenJobsForBoard`'s LIKE-prefix scoping but drops the run-coverage
parameters entirely (there is no "this run" — the evidence is 60 days of accumulated
failure, not one run's Stats). For a boardless provider's chronic record
(`board = ''`), the closer instead calls the existing source-scoped
`CloseUnseenJobsBySource`-shaped close (by `source` alone, no LIKE pattern) — see
`ingest-board-health`'s existing note that a boardless health record already stands
for the whole provider. Both paths write `closed_reason = 'board_unreachable'`.

**5a. Region-ambiguous board names are refused, not board-scoped.** `board_health`'s
primary key is `(provider, board, region)` specifically because a board id like
Adzuna's `it-jobs` repeats once per country (`internal/ingest/sources/adzuna.go`),
and `jobs.external_id` has no region dimension at all — the same fact
`CloseUnseenJobsForBoard`'s existing caller already accounts for via
`pipeline.ambiguousRegionBoards`/`AmbiguousBoardNames`, which refuses to board-scope
an ambiguous name and falls back to the company scope instead. This worker has no
crawl-run board list to compute that check against (and does not need one):
`CountBoardHealthRegions(provider, board)` asks `board_health` directly — more than
one row for the same `(provider, board)` means the name is ambiguous — before
`closeOrCountOneBoard` is ever called for a `board != ''` row. An ambiguous board is
skipped and counted separately in the report (`boardsSkippedAmbiguous`), never
silently merged into "processed" or "closed", so a curator reading the log can see it
needs a by-hand decision rather than assuming the pass covered it. This was caught in
review, not anticipated in the original design — the LIKE-pattern reuse in Decision 5
above copied the SQL shape from `CloseUnseenJobsForBoard` without also copying the
safety check that makes that shape sound.

**6. Chronic reporting reuses `ListUnhealthyBoards`'s consumer, adds a second
section.** The per-run log summary (`cmd/ingest/main.go`'s unhealthy-boards line) and
the `/status`-page-adjacent operator query both gain a chronic count/list alongside
the existing cooling/failing one, calling `ListChronicBoards(30 days)`. No new
surface — same log line shape, one more labeled group.

## Risks / Trade-offs

- **[Risk] The 60-day closure window is a guess, not measured.** It is far above the
  cooldown ceiling (~24h) and the existing age-close windows (45 days for
  no-close-signal sources), which is the right ordering, but the exact value is a
  judgment call. → Mitigation: env-configurable without a deploy of new code: adjust
  and re-run if 60 days proves too eager or too lax once real chronic boards are
  observed.
- **[Risk] Backfilling `first_seen_at` to `now()` delays flagging never-succeeded
  boards by up to one full chronic window post-deploy.** → Mitigation: deliberate,
  see Decision 2; boards that have succeeded at least once (the majority of the
  measured backlog — paylocity, oracle, jobdanmark's worst offenders all have a real
  `last_success_at`) are unaffected and chronic immediately.
- **[Risk] A board retired from the catalog (`DeleteBoardHealth`) never reaches the
  closure window** — its row is deleted, not aged out. → Not a new risk: this is
  existing behavior (`ingest-board-health`'s "Removing a board from YAML leaves no
  orphaned behavior" — now the `boards` table per `AGENTS.md`) and orthogonal to this
  change; a board actively retired already has its own close path via the ordinary
  ingest-sweep-adjacent retirement flow.
- **[Risk] A region-ambiguous board name that stays permanently split (one region dead,
  another alive) never gets auto-closed by this safety net at all** (Decision 5a) — it
  is reported as skipped on every run, indefinitely, rather than resolved. →
  Mitigation: deliberate, same bias as the ordinary sweep's own refusal to
  board-scope such a name; this is a genuinely rare shape (one board_health-tracked
  provider, Adzuna, as of this writing) and the chronic-report line names it
  specifically so a curator can close the dead region's postings by hand if it
  matters enough to act on before the board name naturally stops being ambiguous
  (the dead region's `board_health` row could also be deleted by hand, which removes
  the ambiguity and lets a later run close it normally).

## Migration Plan

1. Migrations: add `board_health.first_seen_at TIMESTAMPTZ NOT NULL DEFAULT now()`
   (backfills existing rows to `now()` in the same statement — see Decision 2,
   deliberately not history-derived); and widen `jobs.closed_reason`'s `CHECK`
   constraint to admit `board_unreachable` (see Context).
2. `sqlc` regenerate after the new/changed queries land.
3. Deploy `cmd/close-chronic-boards` in dry-run-only for at least one full 60-day
   window before enabling `--apply` on its timer, so the first real closure list is
   reviewed by a human before anything closes — consistent with how this codebase
   treats every other broad, hard-to-reverse catalogue write.
4. No rollback complexity: the closer only sets `closed_at`/`closed_reason` on rows
   that already independently satisfy "no successful crawl in 60+ days" — disabling
   its timer stops all further action, and any job it closed reopens normally on its
   board's next successful crawl (per the existing reopen requirement).
