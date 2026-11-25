## ADDED Requirements

### Requirement: Sampled liveness audit

A weekly audit SHALL probe a random sample of the open, non-private postings of every source
whose snapshot `open_jobs` is at least 500, and SHALL count, per source, how many it sampled,
how many the source reports gone, and how many it reports live. It SHALL read only the HTTP
status of the posting's stored URL: 404 or 410 is gone; a 2xx whose final URL is the requested
one is live; anything else is unmeasured.

A gone or live verdict SHALL count only when the posting's host has answered 404 or 410 for a
posting id that cannot exist. The audit SHALL NOT close, reopen, or otherwise write to any
posting.

#### Scenario: A host that answers 200 for any id

- **WHEN** a host answers the impossible-id request with a 2xx
- **THEN** none of that host's sampled postings is counted live or gone
- **AND** they are not fetched

#### Scenario: A source whose closed postings answer 200

- **WHEN** the source is one known to keep a closed posting at 200 (hh, djinni, torre)
- **THEN** its sample is reported with zero live and zero gone

#### Scenario: Redirected away

- **WHEN** a posting URL answers 2xx after redirecting to a different path
- **THEN** it is unmeasured, not live

#### Scenario: Politeness

- **WHEN** a host's robots.txt disallows the posting path for the `freehire` agent or `*`
- **THEN** the posting is unmeasured and not fetched
- **AND** requests to one host are sequential and spaced by at least its Crawl-delay
- **AND** a 403 or 429 ends that host for the run
- **AND** no run makes more requests than its budget

### Requirement: Sampled remote-tag audit

A weekly audit SHALL ask a language model, on the service credential under the
`quality-audit` feature tag, whether a sampled posting tagged `work_mode = 'remote'` is remote,
hybrid, onsite, or unclear from its title, location and description, and whether it is open
worldwide. The model SHALL NOT be shown the posting's stored tags. An answer other than
`unclear` SHALL carry a quote, and an answer whose quote does not appear in the posting text
SHALL be recorded as `unclear`. Each judgement SHALL be persisted with the posting, its source,
and whether the posting was tagged `global`.

#### Scenario: Bounded spend

- **WHEN** the audit runs
- **THEN** it judges at most `QUALITY_AUDIT_REMOTE_MAX` postings (default 200)

#### Scenario: Ungrounded answer

- **WHEN** the model answers `remote` with a quote the posting does not contain
- **THEN** the judgement is stored as `unclear`

### Requirement: Audit gauges

The audit SHALL publish, through the textfile collector and only when `PROM_TEXTFILE_DIR` is
set, `freehire_audit_liveness_{sampled,live,dead}{source}` for the latest run,
`freehire_audit_remote_{sampled,confirmed,contradicted}{source}` and
`freehire_audit_global_{sampled,confirmed,contradicted}{source}` over the judgements of the
last 56 days, and `freehire_audit_measured_timestamp_seconds{check}`. Every value SHALL be a
raw count.

#### Scenario: Unconfigured

- **WHEN** `PROM_TEXTFILE_DIR` is unset and the run is not a dry run
- **THEN** the audit does nothing and opens no database connection

#### Scenario: No judgement in the window

- **WHEN** a source has no remote judgement in the last 56 days
- **THEN** no remote or global series is published for it
