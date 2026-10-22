## ADDED Requirements

### Requirement: Per-company posting lifetime rollup
The system SHALL maintain `insights_company_lifetime`, one row per company with the
number of its postings closed within the last 180 days by an employer-side closure and
the median days from `created_at` to `closed_at` over those postings. Only canonical,
attributable postings (`company_slug <> ''` and `duplicate_of IS NULL`) SHALL count, and
only closures whose `closed_reason` is one of `unseen`, `employer_closed`,
`feed_removed` or `expired`. The table SHALL be rebuilt by `cmd/rollup-company` inside
the same transaction as the other per-company rollups.

#### Scenario: Employer-side closures are measured
- **WHEN** a company has postings open 10, 20 and 30 days, each closed 5 days ago with `closed_reason = 'unseen'`
- **THEN** its row has `closed = 3` and `median_days = 20`

#### Scenario: Closures caused by our own access are excluded
- **WHEN** a company's only closed postings carry `closed_reason` `board_unreachable`, `feed_empty`, `source_misattributed` or `moderated`
- **THEN** the company has no row

#### Scenario: Old closures and duplicates are excluded
- **WHEN** a posting closed 200 days ago, or a posting with `duplicate_of` set closed yesterday
- **THEN** neither contributes to the company's row

#### Scenario: The rebuild is atomic with its siblings
- **WHEN** `cmd/rollup-company` runs
- **THEN** `insights_company_lifetime` is cleared and rebuilt in the same transaction as `insights_company_stats`, `insights_company_growth` and `insights_company_response`
