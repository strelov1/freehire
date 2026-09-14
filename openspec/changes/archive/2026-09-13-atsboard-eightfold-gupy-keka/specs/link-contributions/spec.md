## MODIFIED Requirements

### Requirement: Supported-ATS board recognition

The system SHALL accept a link as a board contribution only when its host belongs to a
supported multi-tenant ATS and the URL yields a board slug, determined without any network
request. A link from an unknown host, a single-tenant source, or a URL with no board segment
SHALL NOT be recorded as a board; instead, when it is a well-formed `http(s)` URL it SHALL be
recorded for manual review (see "Record an unrecognized link for manual review"), and only a
value that is not a well-formed `http(s)` URL SHALL be rejected with the "unsupported ATS"
error. Both a vacancy URL and a bare board-listing URL for the same company SHALL yield the
same board.

#### Scenario: Unsupported host is rejected

- **WHEN** a user submits `https://example.com/careers/123`
- **THEN** it is rejected as a board — no board is derived — and, being a valid URL, recorded for manual review instead

#### Scenario: Single-tenant source is rejected

- **WHEN** a user submits a single-tenant aggregator link (e.g. `https://geekjob.ru/vacancy/6a1e`)
- **THEN** it is rejected as a board — not a per-company board — and, being a valid URL, recorded for manual review instead

#### Scenario: Non-URL garbage is rejected

- **WHEN** a user submits a value that is not a well-formed `http(s)` URL
- **THEN** the system responds 422 with an "unsupported ATS" error and records nothing

#### Scenario: Vacancy URL and board-listing URL yield the same board

- **WHEN** a user submits `https://jobs.ashbyhq.com/blitzy/<uuid>` and another submits `https://jobs.ashbyhq.com/blitzy`
- **THEN** both derive source `ashby`, board `blitzy`, so the second is a duplicate of the first

#### Scenario: Keka links are accepted

- **WHEN** a user submits `https://acme.keka.com/careers`
- **THEN** it derives provider `keka`, board `acme`, and is accepted as a contribution, the
  same as any other recognized multi-tenant ATS
