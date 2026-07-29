## MODIFIED Requirements

### Requirement: Jobs unseen beyond a grace window are closed after a run

After an ingest run, the system SHALL run the unseen-job sweep **per provider, scoped to the companies that run wrote**: for each provider that ingested at least one job during the run, it SHALL stamp `closed_at` on every open job of that provider whose `last_seen_at` is older than that provider's grace window **and whose `company_slug` the run wrote a job for**. The grace window SHALL default to 48 hours and MAY be widened by the provider's adapter, which declares a longer window when its crawl deliberately reaches only a slice of its catalogue — a posting that merely drifted past the crawl's page depth must not be closed and then reopened on a later run. A provider that ingested nothing SHALL NOT have its jobs swept, and a company the run did not write (its board was not in this run, returned no postings, or was removed from the board file) SHALL NOT be swept — so a partial or targeted run, or a full crawl of a large provider that times out before completing, cannot mass-close the boards it never reached. The sweep of one provider never touches another provider's jobs. The trade-off is deliberate: a board that empties out or is removed leaks its open jobs (they reopen or close on a later crawl) rather than risk over-closing live jobs the run simply did not reach.

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

#### Scenario: A partial-coverage provider closes on its own wider window

- **WHEN** a sweep runs for a provider whose adapter declares a 14-day grace window and an open
  job of that provider was last seen 49 hours ago
- **THEN** that job remains open, because 49 hours is inside the declared window

#### Scenario: A provider declaring no window keeps the default

- **WHEN** a sweep runs for a provider whose adapter declares no grace window
- **THEN** its jobs are closed on the 48-hour default, unchanged
