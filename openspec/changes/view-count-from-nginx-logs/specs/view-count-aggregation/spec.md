## ADDED Requirements

### Requirement: Offline aggregation of job views from nginx access logs

The system SHALL count job views by aggregating completed nginx access-log
day-files offline in a scheduled worker, with no per-request work on the job read
path. The worker SHALL parse each log line, select the counted signals, and
update the materialized counters in batch. The read path SHALL NOT perform any
counting or write as a side effect of serving a job.

#### Scenario: Worker runs off the read path

- **WHEN** a job detail page or `GET /api/v1/jobs/:slug` is served
- **THEN** no view-counting write occurs during that request
- **AND** the view is counted later by the worker from the nginx log

#### Scenario: Unparseable lines are skipped

- **WHEN** the worker encounters a log line that does not match the expected
  access-log shape
- **THEN** that line is skipped and processing continues with the next line

### Requirement: Counted signals and bot handling

The worker SHALL count exactly two request signals, both requiring a successful
(2xx) response:

- a page open: `GET /jobs/<slug>` (the SSR detail page) — the worker SHALL skip
  lines whose User-Agent matches a known-bot list;
- an API read: `GET /api/v1/jobs/<slug>` — the worker SHALL NOT apply bot
  filtering to this signal.

Requests to other paths, non-GET methods, and non-2xx responses SHALL NOT be
counted.

#### Scenario: Page open by a browser is counted

- **WHEN** the log contains `GET /jobs/acme-engineer-123 HTTP/2.0` with status 200
  and a non-bot User-Agent
- **THEN** the worker counts one page-open view for that job's slug

#### Scenario: Known bot on the page path is skipped

- **WHEN** the log contains a `GET /jobs/<slug>` line whose User-Agent matches the
  known-bot list
- **THEN** the worker does not count that line

#### Scenario: API read is counted without bot filtering

- **WHEN** the log contains `GET /api/v1/jobs/<slug>` with status 200, regardless
  of User-Agent
- **THEN** the worker counts one API view for that job's slug

#### Scenario: Non-view traffic is ignored

- **WHEN** the log contains lines for other paths (e.g. `/companies/...`), non-GET
  methods, or non-2xx statuses on a job path
- **THEN** the worker counts none of them

### Requirement: Unique-daily-visitor deduplication

The worker SHALL take each view's day from the line's timestamp (converted to UTC)
and deduplicate so that a visitor counts at most once per job per day. The visitor
identity SHALL be `hash(client-IP + User-Agent)`, and the dedup key SHALL be
`(visitor, slug, day)`. Deduplication SHALL apply to both counted signals.

#### Scenario: Repeat opens by the same visitor collapse to one

- **WHEN** the same client-IP and User-Agent open the same job three times on the
  same day
- **THEN** the worker counts one unique view for that job that day

#### Scenario: Distinct visitors count separately

- **WHEN** two different client-IP/User-Agent pairs open the same job on the same
  day
- **THEN** the worker counts two unique views for that job that day

#### Scenario: The same visitor on two different days counts twice

- **WHEN** the same client-IP and User-Agent open the same job on two different days
- **THEN** the worker counts one unique view for that job on each day

### Requirement: Additive rollup and materialized counter update

For each processed file, the worker SHALL apply its per-`(day, slug)` uniques with
an additive upsert into `job_daily_views(day, job_id, uniques)`
(`ON CONFLICT (day, job_id) DO UPDATE SET uniques = uniques + EXCLUDED.uniques`) and
add the same per-`(day, job)` delta to `jobs.view_count`, in a single batched
statement. Slugs that do not resolve to a job SHALL be ignored. Additivity ensures
a day whose lines span two files sums correctly across both.

#### Scenario: Counter reflects a processed file

- **WHEN** the worker processes a file in which a job received 4 unique views on a day
- **THEN** `job_daily_views` holds a row `(day, job_id, 4)`
- **AND** the job's `view_count` is increased by 4

#### Scenario: A day split across two files sums additively

- **WHEN** the worker processes two files that each contribute uniques for the same
  `(day, job)`
- **THEN** the `job_daily_views` row for `(day, job)` holds the sum of both files'
  contributions

#### Scenario: Unknown slug is ignored

- **WHEN** a counted line references a slug that matches no job
- **THEN** the worker skips it without error and processes the rest

### Requirement: Processed-file cursor and idempotency

The worker SHALL record each fully processed file in a `processed_view_logs` marker
table keyed by a signature of the file's decompressed content, and SHALL skip any
file whose signature is already marked. The signature SHALL be stable across
logrotate's rename and gzip, so the same content is recognized whether stored
uncompressed or gzipped. It SHALL only process rotated files, never the live
`access.log`. Re-running the worker or the backfill SHALL NOT reprocess an
already-marked file.

#### Scenario: Already-processed file is skipped

- **WHEN** the worker runs and a rotated file's content signature is already in
  `processed_view_logs`
- **THEN** that file is not processed again and counters are unchanged for it

#### Scenario: A gzipped copy of an already-processed file is skipped

- **WHEN** a file's content was applied while uncompressed and the worker later sees
  the same content gzipped (a new inode, same bytes)
- **THEN** the content signature matches and the file is not reprocessed

#### Scenario: A file is marked processed after its update commits

- **WHEN** the worker finishes applying a file's counts
- **THEN** that file's `(device, inode)` is recorded in `processed_view_logs`

#### Scenario: The live log is not processed

- **WHEN** the worker scans the log directory
- **THEN** the currently-written `access.log` is skipped and only rotated files are
  processed

### Requirement: One-shot backfill from historical logs

The worker SHALL provide a backfill mode that processes all retained rotated
(possibly gzipped) access-log files, seeding `job_daily_views` and
`jobs.view_count`. Backfill SHALL share the per-file idempotency so it composes with
ongoing daily runs and can run on the current numeric-suffix logs before any ops
format change.

#### Scenario: Backfill seeds counts from retained history

- **WHEN** backfill runs over retained rotated logs
- **THEN** each unprocessed file is aggregated into `job_daily_views` and added to
  `view_count`, and each is marked in `processed_view_logs`

#### Scenario: Backfill skips files already processed

- **WHEN** backfill encounters a file already in `processed_view_logs`
- **THEN** it does not reprocess that file

### Requirement: No log file is a no-op

When no nginx access log is available (e.g. local or dev environments), the worker
SHALL exit cleanly without error and without modifying counters.

#### Scenario: Missing log directory

- **WHEN** the configured access-log path does not exist
- **THEN** the worker logs that it found nothing to process and exits successfully
