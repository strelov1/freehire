## Why

A Telegram vacancy never closes. The lifecycle writes `closed_at` through five paths and not
one reaches a `telegram` row: the ingest sweep does not cover it (`cmd/tg-extract` writes
through `UpsertJob` directly), there is no change feed to self-close from, and the liveness
probe excludes it by name — a Telegram job's stored URL is the post, which stays live after
the vacancy is filled, so a probe can never reach a death verdict. `cmd/liveness/main.go:52`
says it outright: "These jobs have no lifecycle close signal at all."

Measured on prod 2026-08-02: of 10,395 open `telegram` jobs, 4,914 are older than 30 days and
1,905 older than 45. The oldest is from 2024-11-09.

## What Changes

- A row from a source with no close signal — today exactly the set `cmd/liveness` names in
  `unprobableSources`, i.e. `telegram` — closes once its effective posting date
  (`COALESCE(posted_at, created_at)`) is more than **45 days** old. The rule lives in
  `cmd/liveness`, which already owns "what closes the rows no crawl reaches" and already
  computes that source set.
- Every close records **why**. New column `closed_reason` on `jobs`, written by all five
  existing close paths and by the new age rule, and cleared by the three reopen paths.
  Without it, a sixth meaning would make `closed_at` unreadable: it would stand equally for
  "the employer took this down", "it vanished from the board", "a moderator closed it", "a
  probe found it dead" and "we assume it went stale".

No breaking change. The migration is additive with a default.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `job-lifecycle`: gains a fourth closing mechanism — an age rule for sources that carry no
  close signal — and a requirement that every close records which mechanism wrote it. The
  spec also currently requires the opposite of what the code does for `telegram` (it says a
  `source = 'telegram'` job SHALL be probed, while `cmd/liveness` excludes it); this change
  reconciles the two.

## Impact

- `migrations/0071_jobs_closed_reason.sql` — new column plus a `CHECK` over the known values.
  Number verified free against both the repo and prod, which has `0070` applied.
- `internal/db/queries/jobs.sql` — five close queries set their reason, three reopen queries
  clear it, one new query closes by age. `make sqlc` after.
- `cmd/liveness` — the age rule, run alongside the probe under the same advisory lock.
- `openspec/specs/job-lifecycle/spec.md` — delta.
- `docs/agents/job-lifecycle.md` — the mechanism table and the Limitations section.
- First run closes ~1,905 rows. They leave the search index on the next scheduled reindex,
  the same as every other close — `internal/search/client.go:502` states that `DeleteJobs` is
  "used by reindex to drop closed jobs", and `cmd/liveness` does not import the search client.
