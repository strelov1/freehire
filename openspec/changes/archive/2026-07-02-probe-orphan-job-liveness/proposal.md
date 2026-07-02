## Why

The `close-stale-jobs` sweep keeps the catalogue fresh, but it only covers jobs an
ingest run re-crawls: it is scoped per ATS provider (`CloseUnseenJobs WHERE source =
$provider`) and called only from `cmd/ingest`. Jobs whose `source` is **not** a
registered ATS board provider — `telegram` (direct TG extraction) and the non-ATS
link sources `habr_career` / `geekjob` — are upserted once and never crawled again.
Their `last_seen_at` freezes at creation and no sweep ever runs for them, so
`closed_at` stays NULL forever. These orphan postings keep surfacing in lists and
search long after the employer page is dead.

There is no board to disappear from, so the only liveness signal available is the
posting URL itself. A lightweight HTTP probe (no headless browser, no LLM) can read
the page and close a job on a definitive death signal, reusing the exact soft-close
model `close-stale-jobs` already established.

## What Changes

- **New `cmd/liveness` worker** (run-once-and-exit, like ingest/enrich): selects open
  jobs whose `source` is not a registered ATS board provider, fetches each posting
  URL over plain HTTP, and classifies the result as `expired` or not.
- **`internal/liveness` classifier**: a pure, table-tested function over
  `(status, finalURL, bodyText)` returning `expired` only on definitive signals —
  HTTP 404/410, a redirect to an error/listing URL, a body matching a curated
  "no longer accepting applications" pattern set (EN/DE/FR), or sub-threshold
  content length. Everything else (5xx, 403, timeout, healthy content, a JS-only
  shell) is not-expired and triggers no action.
- **Two-strike grace**: a new `jobs.liveness_strikes` counter. An `expired` probe
  increments it and closes the job (`closed_at = now()`) once it reaches 2; any
  not-expired probe resets it to 0. Two consecutive expired reads (across runs)
  absorb a transient 404 during an employer-site deploy — the same "grace window"
  philosophy as the 48h ingest sweep, and biased to under-close rather than
  false-close (orphan jobs have no re-ingest to reopen them).
- **Reuse `closed_at`**: liveness becomes a second writer of the existing soft-close
  column. No new user-facing state; all visibility semantics (lists hide, detail
  serves with `closed_at`, reindex deletes the document, SPA renders the closed
  notice) are inherited unchanged from `close-stale-jobs`.

## Capabilities

### Modified Capabilities

- `job-lifecycle`: URL-probe liveness for orphan (non-board) jobs — the probe
  selection rule, the `expired` classification signals, the two-strike close, and
  strike reset on a healthy probe.

## Impact

- **Schema**: `ALTER TABLE jobs ADD COLUMN liveness_strikes int NOT NULL DEFAULT 0`
  (new migration file; prod gets a manual apply — the versioned-migration-runner
  seam from AGENT.md remains open).
- **Code**: new `internal/liveness` (classifier + curated patterns); new
  `cmd/liveness/main.go`; new sqlc queries (select orphan candidates excluding the
  ATS provider set; increment-and-close; reset) in `internal/db/queries`,
  regenerated; the shared `internal/sources` HTTP client and registry key set reused
  for fetching and for the provider exclusion list.
- **Out of scope**: a versioned migration runner; round-robin probe scheduling via a
  `liveness_checked_at` column (probe all orphan candidates per run until volume
  warrants it — noted as a seam); a headless-browser tier for JS-only pages
  (consciously deferred, makes the probe under-close, never false-close); the
  pre-existing collision where link-source `greenhouse`/`ashby` jobs are swept by
  the ATS sweep through a shared `source` string (separate concern, untouched here).
