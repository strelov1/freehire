## ADDED Requirements

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
