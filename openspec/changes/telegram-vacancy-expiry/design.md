## Context

A Telegram vacancy never closes. The lifecycle writes `closed_at` through five paths and
not one of them reaches a `telegram` row:

| path | why it misses Telegram |
|---|---|
| `CloseUnseenJobs` (ingest sweep, 48h unseen) | `telegram` is not a board provider; `cmd/tg-extract` writes through `UpsertJob` directly and `cmd/ingest` never sweeps it |
| `CloseUnseenJobsBySource` (full-catalogue sweep) | same |
| `CloseJobBySourceExternalID` (stream self-close) | there is no change feed |
| `CloseJobByID` (moderator) | manual, one row at a time |
| `MarkLivenessExpired` (URL probe) | excluded by name: `cmd/liveness/main.go:55`, `unprobableSources = []string{"telegram"}` |

The exclusion is correct and documented at `cmd/liveness/main.go:48-53`: a Telegram job's
stored URL is the post itself, which stays live after the vacancy is filled, so a probe can
never reach a death verdict. The same comment states the consequence plainly — "These jobs
have no lifecycle close signal at all."

Measured on prod, 2026-08-02:

| open `telegram` jobs | 10,395 |
|---|---|
| older than 30 days | 4,914 (47%) |
| older than 45 days | 1,905 (18%) |
| older than 60 days | 374 (3.6%) |
| oldest | 2024-11-09 |

A second, smaller problem sits underneath. `closed_at` is one word for five different facts,
and adding a sixth — "we presume this is stale" — would make it unreadable. The project has
already refused this overload once: when catalogue pruning needed to express "not our
profile", it got its own table rather than a second meaning for `closed_at`, because
"overloading `closed_at` would corrupt a signal three mechanisms already write"
(`docs/agents/job-lifecycle.md:8`).

## Goals / Non-Goals

**Goals:**

Close Telegram vacancies once they are old enough to be presumed filled, and make every
close say which mechanism closed it and why.

**Non-Goals:**

- **A general age rule for the catalogue.** Board jobs have a real signal — they vanish from
  the feed. Age is a guess, and a guess is only justified where no evidence exists.
- **Reopening an expired job.** A Telegram job never re-crawls, so nothing would reopen it.
  This matches the bias `cmd/liveness` already takes: close on evidence, never reopen.
- **Backfilling a reason onto already-closed rows.** They keep the empty string, which
  honestly reads as "unknown", rather than a label invented after the fact.
- **Exposing the reason in the public API.** It is an operational and audit signal. Add a
  surface when something actually needs to read it.

## Decisions

### 1. Age expiry writes `closed_at`, and every close path records a reason

New column, migration `0071` (verified free against both the repo and prod, which has `0070`
applied):

```sql
ALTER TABLE jobs ADD COLUMN closed_reason text NOT NULL DEFAULT '';
```

Constrained by a `CHECK` over the known values, following the `job_reports_reason_check`
precedent already in the schema. **The empty string must be one of the permitted values** —
it is the default, and every row that exists when the migration runs carries it, so a
constraint that omitted it would reject the whole table:

| reason | written by |
|---|---|
| `unseen` | `CloseUnseenJobs`, `CloseUnseenJobsBySource` |
| `feed_removed` | `CloseJobBySourceExternalID` |
| `moderated` | `CloseJobByID` |
| `probe_expired` | `MarkLivenessExpired` |
| `expired` | the new age rule |
| `''` | unknown — rows closed before this change |

The three reopen paths — `UpsertJob`, `UpsertManualJob`, `TouchJob` — clear the reason
wherever they already clear `closed_at`. A row that reopens must not keep the label of the
mechanism that closed it.

The precedent for recording it per row is `migrations/0041_pruned_jobs.sql:17-19`, whose
comment gives the reason directly: a rule that turns out to be too broad can then be audited
alone. That is exactly what an age rule needs, because it is the one close that rests on a
guess.

### 2. The rule lives in `cmd/liveness`, not a new worker

`cmd/liveness` already owns the question "what closes the rows no crawl reaches", already
holds its own Postgres advisory lock, and already computes the exact source set this rule
targets — it computes it *because* those sources cannot be probed. Putting the age rule
there keeps both halves of one problem in one place: what can be reached is probed, what
cannot is judged by age.

*Alternative — a separate `cmd/expire-jobs`.* Rejected: it would duplicate the lock, the
cron entry, and the source list, and would split a single decision ("how do we close an
unreachable row?") across two binaries.

### 3. The window is 45 days on `COALESCE(posted_at, created_at)`

`COALESCE(posted_at, created_at)` is the codebase's existing idiom for a posting's effective
date and is already indexed (`jobs_open_enrich_freshness_idx`).

45 rather than 30: the close rests on a guess, not evidence, so the bias should match the
liveness probe's — prefer under-closing. 30 days would close 4,914 rows, nearly half the
segment, taking live vacancies with the dead. 60 would close 374 and barely address the
problem. At 45 the first run closes 1,905 and the steady state is a trickle as rows age.

### 4. Closed rows leave the index on the next reindex, not immediately

`internal/search/client.go:502-503` states it: `DeleteJobs` is "used by reindex to drop
closed jobs". `cmd/liveness` does not import the search client at all — it writes `closed_at`
and lets the scheduled reindex sweep the documents. The age rule inherits that, which means
no extra rollout step and a lag of up to one reindex interval before an expired vacancy
disappears from search. This is the behaviour every other close already has.

## Migration Plan

Deploy, then let the existing `cmd/liveness` cron run. The first run closes ~1,905 rows; the
scheduled reindex removes them from the index within its normal interval. The migration is
additive with a default, so it applies without a rewrite and needs no manual step.

Rollback is reverting the code; `UPDATE jobs SET closed_at = NULL, closed_reason = ''
WHERE closed_reason = 'expired'` restores the rows, which is precisely the audit the reason
column exists to enable.

## Testing

- Integration test (`internal/db`, build tag `integration`): a `telegram` row older than the
  window closes with `closed_reason = 'expired'`; one inside the window does not; a row from
  a probeable source is untouched whatever its age.
- Boundary test on the window itself — a row exactly at the cutoff.
- A test asserting each of the five existing close queries writes its own reason, and that
  each reopen path clears it. Without this the reason silently rots as paths are edited.

## Open Questions

None. The index behaviour, the migration number, the close-path inventory and the window
distribution were all checked against the code and against prod before this was written.
