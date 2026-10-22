# company-hiring-signal Specification

## Purpose
TBD - created by archiving change add-company-hiring-signal-rollup. Update Purpose after archive.
## Requirements
### Requirement: Per-company daily hiring-velocity rollup

The system SHALL maintain a precomputed rollup table `insights_company_stats` keyed by `(company_slug, day)` that records, for each company and each UTC day the company had activity, the number of jobs `added` (jobs whose `created_at` falls on that day), `removed` (jobs whose current `closed_at` falls on that day), and `open` (jobs open as of the end of that day).

Openness for a day `D` SHALL be computed as `created_at <= D AND (closed_at IS NULL OR closed_at > D)`, matching the convention documented in `migrations/0022_insights_rollups.sql`.

Only canonical, attributable job rows SHALL be counted: rows with a non-empty `company_slug` and `duplicate_of IS NULL` (repost copies are excluded so counts match `companies.job_count` semantics).

#### Scenario: A company's posting is reflected as added and open

- **WHEN** a company has one job with `created_at = 2026-01-10` and `closed_at IS NULL`, and the rollup is rebuilt
- **THEN** `insights_company_stats` has a row for that `company_slug` on day `2026-01-10` with `added = 1`
- **AND** `open` is `1` for that company on `2026-01-10` and every subsequent day up to the rollup's latest day

#### Scenario: A closed posting is reflected as removed

- **WHEN** a company's job has `created_at = 2026-01-10` and `closed_at = 2026-01-20`
- **THEN** the rollup records `added = 1` on `2026-01-10` and `removed = 1` on `2026-01-20`
- **AND** `open` for that company is `0` from `2026-01-20` onward (the close day itself is no longer open)

#### Scenario: Repost copies are not double-counted

- **WHEN** a job row has `duplicate_of` pointing at a canonical row
- **THEN** that duplicate row contributes nothing to `added`, `removed`, or `open`

#### Scenario: Jobs without a company are excluded

- **WHEN** a job row has an empty `company_slug`
- **THEN** it contributes to no `insights_company_stats` row

### Requirement: Retroactive full-history rebuild

The rollup SHALL be reconstructable in full from the current `jobs` table alone, back to the earliest `created_at`, because closed jobs are retained rather than deleted. A rebuild SHALL NOT depend on any prior rollup state or external snapshot.

#### Scenario: First rebuild reconstructs prior days

- **WHEN** the rollup is run for the first time against a catalogue containing jobs created and closed across past days
- **THEN** `insights_company_stats` contains rows for those past days derived solely from `created_at`/`closed_at`

### Requirement: Hiring growth derivable from the open time series

Hiring growth (ramping vs. freezing) over any window SHALL be derivable from the stored per-day `open` series alone, without scanning `jobs`: a company's open count as of a date is the value of `open` on its latest rollup row at or before that date (carry-forward across days with no activity). The rollup therefore SHALL NOT need a separate stored "previous window" column.

#### Scenario: 30-day growth reads from the open series

- **WHEN** a company's rollup shows `open = 4` on its latest row at or before 30 days ago and `open = 10` on its most recent row
- **THEN** its 30-day growth is `10 - 4 = 6`, computed from `insights_company_stats` alone

### Requirement: Atomic rebuild worker

A run-once-and-exit worker `cmd/rollup-company` SHALL rebuild `insights_company_stats` inside a single database transaction using a `DELETE`-then-`INSERT` recompute, so that readers never observe a partially rebuilt table. The worker SHALL require only `DATABASE_URL` and SHALL exit non-zero on failure without leaving the table partially written.

#### Scenario: Rebuild is all-or-nothing

- **WHEN** the rebuild transaction fails partway
- **THEN** the previously committed contents of `insights_company_stats` remain intact (no partial state is committed)

#### Scenario: Successful run replaces contents atomically

- **WHEN** the worker completes successfully
- **THEN** `insights_company_stats` reflects exactly the recomputed rows for the current `jobs` state

### Requirement: Precomputed per-company open/growth scalar

The rollup SHALL maintain a per-company scalar table
`insights_company_growth(company_slug, open_count, open_count_prev)` to back a
ranked leaderboard without per-request aggregation over the full catalogue, where
`open_count` is the company's current count of open canonical jobs and
`open_count_prev` is that count as of 30 days earlier. Only canonical, attributable
rows SHALL be counted (`company_slug <> '' AND duplicate_of IS NULL`), consistent
with `insights_company_stats` and `companies.job_count`. The 30-day window SHALL be
the same constant the existing `insights_*` rollups use.

The table SHALL be rebuilt by `cmd/rollup-company` in the **same transaction** as
`insights_company_stats`, as an atomic delete-and-reinsert, so a reader never sees
one rebuilt without the other.

#### Scenario: Scalar reflects current and prior open counts

- **WHEN** a company has 10 open canonical jobs now and had 4 open 30 days ago
- **THEN** its `insights_company_growth` row has `open_count = 10` and
  `open_count_prev = 4`

#### Scenario: Only canonical rows counted

- **WHEN** a company has open jobs that are repost copies (`duplicate_of` set) or
  have an empty `company_slug`
- **THEN** those jobs do not contribute to `open_count` or `open_count_prev`

#### Scenario: Rebuilt atomically with the per-day rollup

- **WHEN** `cmd/rollup-company` runs
- **THEN** both `insights_company_stats` and `insights_company_growth` are replaced
  within one transaction, or neither is (a failure leaves both prior tables intact)

### Requirement: Per-company application response rate, gated by sample size

The company rollup SHALL maintain, alongside the existing hiring-velocity scalars, the number of
applications tracked against a company and how many of them received a reply, and SHALL serve a
response rate derived from the pair **only when the company has at least ten tracked
applications**. Below that sample size the field SHALL be absent from the response, not zero and
not an estimate.

Both sides of the ratio SHALL be counted from `application_events`, not from live `emails` rows.
An application counts as answered when a non-retracted `employer_reply` event exists for it.
Reading the mail table made the served rate a function of the candidate's inbox hygiene: the
rebuild counted messages `WHERE deleted_at IS NULL`, so a candidate clearing old mail made an
employer that had answered them look silent, on a public page, about a named company.

Only applications whose owner has a connected mailbox SHALL count, on both sides of the ratio, for
the same reason the job-level signal requires it: where no mail can be linked, an unanswered
application is a gap in our data rather than an observed silence.

This is the measure a previous investigation identified as the only unconfounded company-level
signal, having ruled out posting age and never-closing as artifacts of company type and of our own
ingest history. It is expected to be absent for nearly every company until the sample matures;
that absence is the correct answer, not an unfinished implementation.

#### Scenario: A company below the sample gate serves no rate

- **WHEN** a company has four tracked applications from users with connected mailboxes
- **THEN** the served company payload carries no response rate

#### Scenario: A company above the gate serves the rate

- **WHEN** a company has twelve tracked applications, three of which received a reply
- **THEN** the served company payload reports a response rate over that denominator

#### Scenario: Applications without a connected mailbox are excluded from both sides

- **WHEN** a company has twenty tracked applications, of which six belong to users with connected mailboxes
- **THEN** the sample size is six, the gate is not met, and no rate is served

#### Scenario: Deleted mail does not move the rate

- **WHEN** a candidate deletes the reply an employer sent them and the rollup runs again
- **THEN** that application still counts as answered and the company's rate is unchanged

#### Scenario: A corrected link moves the answer to the right company

- **WHEN** an email counted against company A is re-linked to company B and the rollup runs
- **THEN** A's answered count drops by one and B's rises by one

### Requirement: Global application response rate

The company rollup SHALL also maintain one all-companies response rate, summed from the same
per-company observable and answered counts the per-company figure is built from, and SHALL serve
it under the same ten-application sample gate the per-company figure uses. Below the floor the
global figure SHALL be absent, not zero and not an estimate.

The sum SHALL include every company's observable/answered counts regardless of whether that
company individually clears its own ten-application gate — the per-company gate governs what is
safe to publish about one named employer, not what may contribute to an aggregate across all of
them.

#### Scenario: Global rate above the gate

- **WHEN** the sum of every company's observable applications is at least ten
- **THEN** a global response rate is served

#### Scenario: Global rate below the gate

- **WHEN** the sum of every company's observable applications is under ten
- **THEN** no global response rate is served

#### Scenario: Small companies still contribute to the global sum

- **WHEN** every individual company has fewer than ten observable applications, but their combined
  total is fifteen
- **THEN** the global response rate is served, computed over all fifteen

#### Scenario: Read from the same post-rebuild snapshot as the per-company figure

- **WHEN** `cmd/rollup-company` runs
- **THEN** the global response rate, read as a live sum over the per-company rows, reflects
  exactly the snapshot those rows were replaced into within one transaction — never a mix of the
  old and new rebuild

### Requirement: Per-company time to first reply

The company rollup SHALL record the median number of days from application to the first
non-retracted `employer_reply` event, and SHALL serve it under the same ten-application sample
gate as the response rate it accompanies.

Applications that never received a reply SHALL be excluded from the median and reported as a
count beside it. A median over answered applications only is right-censored, and presenting it
without saying how many are still waiting states a reassuring number that the unanswered
majority contradicts.

The median SHALL be computed over `occurred_at`, so importing a year of historical mail does not
compress every past reply into the day the mailbox was connected.

#### Scenario: A company with enough answered applications

- **WHEN** a company has fourteen observable applications, nine of them answered
- **THEN** the payload reports the median days to first reply over those nine, and reports that
  five received no reply

#### Scenario: Above the gate but never answered

- **WHEN** a company has eleven observable applications and none received a reply
- **THEN** the payload reports a response rate of zero and no median, rather than a median of
  zero days

#### Scenario: The median ignores retracted events

- **WHEN** an application's only `employer_reply` event has been retracted by a link correction
- **THEN** that application counts as unanswered in both the rate and the median

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

