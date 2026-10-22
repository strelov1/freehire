## ADDED Requirements

### Requirement: Per-company posting views rollup
The system SHALL maintain `insights_company_views`, one row per company with the sum of
`job_daily_views.page_uniques` over its postings for the last 30 days (`views`) and for
the 30 days before that (`views_prev`). It SHALL read `page_uniques` and never `uniques`,
which fuses unfiltered API reads with page opens. Every posting carrying the company's
slug SHALL count, duplicates included — a view of a repost is still a view of the
company. Companies with zero views in both windows SHALL have no row. The table SHALL be
rebuilt by `cmd/rollup-company` in the same transaction as the other per-company
rollups.

#### Scenario: Two windows
- **WHEN** a company's postings have 10 page views 3 days ago and 4 page views 40 days ago
- **THEN** its row has `views = 10` and `views_prev = 4`

#### Scenario: Unfiltered reads are not views
- **WHEN** a posting's day carries `uniques = 500` and `page_uniques = 7`
- **THEN** that day contributes 7

#### Scenario: Nothing older than 60 days
- **WHEN** a company's only views are 70 days old
- **THEN** the company has no row
