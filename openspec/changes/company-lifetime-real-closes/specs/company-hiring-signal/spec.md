## MODIFIED Requirements

### Requirement: Per-company posting lifetime rollup
The system SHALL maintain `insights_company_lifetime`, one row per company with the
number of its postings closed within the last 180 days by a closure that says the
posting is over, and the median days from `created_at` to `closed_at` over those
postings. Only canonical, attributable, public postings (`company_slug <> ''`,
`duplicate_of IS NULL` and `NOT is_private`) SHALL count, and only closures whose `closed_reason` is one of
`unseen`, `feed_removed`, `probe_expired` or `employer_closed` — the same set every
insights rollup that measures a real close uses. `expired` (the age rule for sources
with no close signal) SHALL NOT count. The table SHALL be rebuilt by `cmd/rollup-company`
inside the same transaction as the other per-company rollups.

#### Scenario: Real closures are measured
- **WHEN** a company has postings open 10, 20 and 30 days, closed 5 days ago with `closed_reason` `unseen`, `employer_closed` and `probe_expired`
- **THEN** its row has `closed = 3` and `median_days = 20`

#### Scenario: Closures caused by our own access, rules or decisions are excluded
- **WHEN** a company's only closed postings carry `closed_reason` `board_unreachable`, `feed_empty`, `source_misattributed`, `moderated`, `expired`, `board_retired` or `''`
- **THEN** the company has no row

#### Scenario: Old closures, duplicates and private postings are excluded
- **WHEN** a posting closed 200 days ago, a posting with `duplicate_of` set closed yesterday, or a member's private import closed yesterday as `probe_expired`
- **THEN** none of them contributes to the company's row

#### Scenario: The rebuild is atomic with its siblings
- **WHEN** `cmd/rollup-company` runs
- **THEN** `insights_company_lifetime` is cleared and rebuilt in the same transaction as `insights_company_stats`, `insights_company_growth`, `insights_company_response` and `insights_company_views`
