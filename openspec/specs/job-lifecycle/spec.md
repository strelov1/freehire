# job-lifecycle Specification

## Purpose
TBD - created by archiving change close-stale-jobs. Update Purpose after archive.
## Requirements
### Requirement: Every ingested job records when a crawl last saw it

The system SHALL stamp `last_seen_at` on a job every time ingest upserts it, for
both newly inserted and re-ingested postings, within the same atomic write that
persists the job.

#### Scenario: Re-ingest refreshes liveness

- **WHEN** an ingest run upserts a job that already exists
- **THEN** the job's `last_seen_at` is set to the time of that write

### Requirement: Jobs unseen beyond a grace window are closed after a run

After an ingest run, the system SHALL run the unseen-job sweep **per provider, scoped to the companies that run wrote**: for each provider that ingested at least one job during the run, it SHALL stamp `closed_at` on every open job of that provider whose `last_seen_at` is older than a 48-hour grace window **and whose `company_slug` the run wrote a job for**. A provider that ingested nothing SHALL NOT have its jobs swept, and a company the run did not write (its board was not in this run, returned no postings, or was removed from the board file) SHALL NOT be swept — so a partial or targeted run, or a full crawl of a large provider that times out before completing, cannot mass-close the boards it never reached. The sweep of one provider never touches another provider's jobs. The trade-off is deliberate: a board that empties out or is removed leaks its open jobs (they reopen or close on a later crawl) rather than risk over-closing live jobs the run simply did not reach.

#### Scenario: Stale job is closed

- **WHEN** a sweep runs after a provider ingested at least one job and an open job of that
  provider — belonging to a company the run wrote — was last seen 49 hours ago
- **THEN** that job's `closed_at` is set and the job stops appearing in list surfaces

#### Scenario: A company the run did not crawl is not swept

- **WHEN** a run ingests jobs for company A of a provider but does not write any job for
  company B of the same provider (B's board was not in this run, or returned no postings)
  and B has an open job last seen 49 hours ago
- **THEN** the sweep closes A's stale jobs but leaves B's stale job open

#### Scenario: Recently seen job survives the sweep

- **WHEN** a sweep runs and an open job was last seen 6 hours ago
- **THEN** the job remains open

#### Scenario: A provider that ingested nothing closes nothing

- **WHEN** a run ingested jobs for provider A but zero for provider B (B's crawl failed)
- **THEN** the sweep runs for A but not for B, so no B job is closed

#### Scenario: One provider's sweep leaves another provider's jobs alone

- **WHEN** a multi-provider run sweeps provider A's stale jobs
- **THEN** provider B's jobs are never closed by A's sweep

### Requirement: A reappearing posting reopens its job

The system SHALL clear `closed_at` when ingest upserts a job that was previously
closed, restoring it to all open-job surfaces.

#### Scenario: Republished posting reopens

- **WHEN** a closed job's posting appears again in a crawl
- **THEN** the upsert clears `closed_at` and the job is listed again

### Requirement: Closed jobs are hidden from lists but served on detail

The jobs list SHALL return only open jobs. The job detail endpoint SHALL still
return a closed job — its public slug, enrichment, and a `closed_at` timestamp in
the job view shape — so external links and application history never break.

#### Scenario: Closed job leaves the list

- **WHEN** a job has `closed_at` set
- **THEN** `GET /api/v1/jobs` does not include it

#### Scenario: Closed job detail still resolves

- **WHEN** a client requests `GET /api/v1/jobs/:slug` for a closed job
- **THEN** the response is 200 and the job view carries its `closed_at`

### Requirement: Orphan jobs are liveness-probed by URL

The system SHALL probe the posting URL of every open job whose `source` is not a
registered ATS board provider — the sources no ingest run re-crawls (e.g.
`telegram`, `habr_career`, `geekjob`). Board-provider jobs, which the ingest sweep
already covers, SHALL NOT be probed. The probe SHALL use a plain HTTP request (no
headless browser, no LLM) with a per-probe timeout, and SHALL classify the outcome
without any persisted page content.

#### Scenario: Orphan job is a probe candidate

- **WHEN** the liveness worker runs and an open job has `source = 'telegram'`
- **THEN** that job's posting URL is fetched and classified

#### Scenario: Board job is not probed

- **WHEN** the liveness worker runs and an open job has `source = 'greenhouse'`
  (a registered ATS provider)
- **THEN** that job is not selected for probing

#### Scenario: Closed job is not probed

- **WHEN** the liveness worker runs and an orphan job already has `closed_at` set
- **THEN** that job is not selected for probing

### Requirement: A probe is classified into one of three verdicts

The classifier SHALL return one of three verdicts so the worker can act differently on
"alive" than on "could not tell":

- `expired` — a definitive death signal: an HTTP `404` or `410`; a final URL matching
  an error/listing redirect pattern; a body matching a curated hard-expired pattern; or
  body content below a minimum length threshold. Only this verdict advances a job toward
  closing.
- `live` — a healthy `2xx` posting with no death signal. This verdict clears strikes.
- `uncertain` — any non-`2xx` that is not a definitive gone (`5xx`, `403`), or a network
  or timeout error. This verdict triggers no state change at all.

#### Scenario: HTTP gone is expired

- **WHEN** a probe returns HTTP 404 or 410
- **THEN** the probe is classified `expired`

#### Scenario: Closed-posting body is expired

- **WHEN** a probe returns HTTP 200 with a body matching a hard-expired pattern
  (e.g. "no longer accepting applications")
- **THEN** the probe is classified `expired`

#### Scenario: Empty shell is expired

- **WHEN** a probe returns body content shorter than the minimum content threshold
- **THEN** the probe is classified `expired`

#### Scenario: Healthy page is live

- **WHEN** a probe returns HTTP 200 with substantial content and no hard-expired
  signal
- **THEN** the probe is classified `live`

#### Scenario: Transient failure is uncertain

- **WHEN** a probe returns HTTP 503 or 403, or fails with a timeout
- **THEN** the probe is classified `uncertain` and the job's strike count is left
  unchanged

### Requirement: An orphan job is closed only after two consecutive expired probes

The system SHALL track consecutive `expired` probes per job in
`jobs.liveness_strikes`. An `expired` probe SHALL increment the counter, and on
reaching two SHALL set `closed_at` within the same write. A `live` probe SHALL reset
the counter to zero, so only consecutive expired reads close a job. An `uncertain`
probe SHALL leave the counter unchanged — a probe that could not reach or judge the
page is neither evidence of death nor of life. This grace absorbs a transient signal
and biases toward leaving an orphan job open rather than closing it irreversibly.

#### Scenario: First expired probe stamps a strike but does not close

- **WHEN** an open orphan job with `liveness_strikes = 0` is probed `expired`
- **THEN** `liveness_strikes` becomes 1 and `closed_at` remains NULL

#### Scenario: Second consecutive expired probe closes the job

- **WHEN** an open orphan job with `liveness_strikes = 1` is probed `expired`
- **THEN** `liveness_strikes` becomes 2 and `closed_at` is set, and the job stops
  appearing in list and search surfaces

#### Scenario: A live probe resets the strike count

- **WHEN** an open orphan job with `liveness_strikes = 1` is probed `live`
- **THEN** `liveness_strikes` is reset to 0 and the job remains open

#### Scenario: An uncertain probe preserves the strike count

- **WHEN** an open orphan job with `liveness_strikes = 1` is probed `uncertain`
  (a transient failure)
- **THEN** `liveness_strikes` is left at 1 and the job remains open

