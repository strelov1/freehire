## ADDED Requirements

### Requirement: Digest day selection

The digest SHALL be built for the most recent day for which `job_daily_views`
holds any row, not for a day computed from the current clock. `cmd/rollup-views`
never reads the live access log, so every day present in that table is complete;
which day is the freshest depends on when logrotate runs on the host, which the
digest SHALL NOT assume.

If that day is more than three days older than the current UTC date, the digest
SHALL treat the view-count pipeline as broken: it SHALL publish nothing and
SHALL fail the run.

An explicit day MAY be supplied to replay a past day, in which case that day is
used verbatim and the staleness check SHALL NOT apply.

#### Scenario: Freshest available day is used

- **WHEN** `job_daily_views` holds rows for days up to and including 2026-09-03
  and no day is requested explicitly
- **THEN** the digest is built for 2026-09-03

#### Scenario: Stale view data fails the run

- **WHEN** the freshest day in `job_daily_views` is more than three days before
  the current UTC date and no day is requested explicitly
- **THEN** nothing is published
- **AND** the run fails

#### Scenario: An explicit day bypasses the staleness check

- **WHEN** a specific day is requested
- **THEN** the digest is built for that day regardless of how old it is

#### Scenario: No view data at all

- **WHEN** `job_daily_views` holds no rows
- **THEN** nothing is published
- **AND** the run fails

### Requirement: Ranking on the bot-filtered signal

The digest SHALL rank candidate postings by `job_daily_views.page_uniques` — the
bot-filtered page-open count — and SHALL NOT rank on `uniques`, which fuses that
signal with API reads that carry no bot filtering.

#### Scenario: A posting with heavy API traffic does not outrank one with more page opens

- **WHEN** posting A has 5 page uniques and 900 API uniques on the digest day,
  and posting B has 40 page uniques and 0 API uniques
- **THEN** posting B ranks above posting A

### Requirement: Editorial eligibility rules

The digest SHALL select at most ten postings for the day, applying these rules:

- **Open only.** A posting that is closed, marked as a duplicate by any of the
  duplicate-marker columns, or whose source ATS has stopped listing it SHALL NOT
  be selected. The last of those is the evidence, not the full ghost verdict:
  that verdict is a hedged classification needing evidence this selection has no
  reason to gather, and "the employer's own board no longer shows it" is the
  strongest single piece of it.
- **Tech only.** A posting SHALL be selected only if it is positively classified
  as a tech job. A posting the classifier could not decide about SHALL NOT be
  selected: this catalogue carries non-tech postings, the most-viewed of them are
  exactly the ones that rise, and "we are not sure this is a tech job" is not a
  good enough reason to put it in front of people under our own name.
- **View floor.** A posting with fewer than ten page uniques on the digest day
  SHALL NOT be selected.
- **Company cap.** At most two postings from any one `company_slug` SHALL be
  selected. When more qualify, the highest-ranked two are kept.
- **Quarantine.** A posting published in a digest within the previous seven days
  SHALL NOT be selected again.

The view floor and the quarantine window SHALL be constants of the package, not
environment variables: changing either changes what the public sees, and that
decision belongs in a commit.

#### Scenario: A closed posting is excluded

- **WHEN** the day's highest-viewed posting is closed
- **THEN** it is not selected, and the next eligible posting takes its place

#### Scenario: A non-tech posting is excluded however popular

- **WHEN** the day's highest-viewed posting is classified as not a tech job
- **THEN** it is not selected, and the next eligible posting takes its place

#### Scenario: An unclassified posting is excluded

- **WHEN** a posting's tech classification is unresolved
- **THEN** it is not selected

#### Scenario: A posting below the view floor is excluded

- **WHEN** a posting has nine page uniques on the digest day
- **THEN** it is not selected

#### Scenario: A third posting from the same company is dropped

- **WHEN** the ranked candidates include four postings from `acme`
- **THEN** only the two highest-ranked `acme` postings are selected

#### Scenario: A recently published posting is quarantined

- **WHEN** a posting was published in a digest three days ago and ranks in
  today's top ten
- **THEN** it is not selected, and the next eligible posting takes its place

#### Scenario: A posting published eight days ago may return

- **WHEN** a posting was last published in a digest eight days ago and ranks in
  today's top ten
- **THEN** it is selected

### Requirement: An empty or thin day is not published

When fewer than one posting survives the eligibility rules, the digest SHALL
publish nothing and SHALL exit successfully. A day with nothing worth posting is
a quiet day, not a failure.

#### Scenario: No posting clears the floor

- **WHEN** every posting on the digest day has fewer than ten page uniques
- **THEN** nothing is published
- **AND** the run succeeds

### Requirement: Publish-once ledger

Every published digest SHALL be recorded in a `social_digest_posts` ledger keyed
by `(day, channel)`, listing the postings it contained. A second run for a day
and channel already present in the ledger SHALL publish nothing to that channel.

The ledger is also what the quarantine rule reads.

#### Scenario: Re-running the worker on the same day publishes nothing

- **WHEN** the worker runs twice for the same day and the first run published to
  a channel
- **THEN** the second run does not publish to that channel again

#### Scenario: A channel that has not yet published still publishes

- **WHEN** two channels are configured, a run published to the first but failed
  before publishing to the second, and the worker runs again for the same day
- **THEN** the first channel is skipped and the second is published

### Requirement: Channels are independent and optional

Each publishing channel SHALL be configured independently and SHALL be disabled
without error when its credentials are absent, matching how the rest of the
worker fleet degrades.

A channel that fails SHALL NOT prevent another channel from publishing. When any
configured channel fails, the run SHALL fail after all channels have been
attempted.

#### Scenario: An unconfigured channel is skipped silently

- **WHEN** a channel's credentials are absent from the configuration
- **THEN** the digest is not published to that channel
- **AND** the run does not fail for that reason

#### Scenario: One channel failing does not block the other

- **WHEN** two channels are configured, the first returns an error and the second
  succeeds
- **THEN** the second channel's post is published and recorded in the ledger
- **AND** the first channel's failure is reported and the run fails

#### Scenario: No channel configured at all

- **WHEN** neither channel is configured
- **THEN** the worker publishes nothing and exits successfully

### Requirement: Dry run renders without publishing

The worker SHALL provide a dry-run mode that selects the postings and renders
each configured channel's message, writes them to the log, and publishes
nothing. A dry run SHALL NOT write to the ledger.

#### Scenario: Dry run leaves no trace

- **WHEN** the worker runs in dry-run mode
- **THEN** the rendered message is logged
- **AND** no request is made to any channel
- **AND** `social_digest_posts` is unchanged
