## MODIFIED Requirements

### Requirement: Jobs unseen beyond a grace window are closed after a run

After an ingest run, the system SHALL run the unseen-job sweep **per provider**, closing an open
job of that provider whose `last_seen_at` is older than the provider's grace window when EITHER
scope covers it:

- **the company scope** — the run wrote a job for that job's `company_slug`; or
- **the board scope** — the job's board is one this run PROVED it covered.

A provider that ingested nothing SHALL NOT have its jobs swept, and the sweep of one provider
SHALL never touch another provider's jobs. The grace window SHALL default to 48 hours and MAY be
widened by the provider's adapter.

A run PROVES it covered a board when all of the following hold for that board in that run:

1. its crawl did not fail;
2. it yielded at least one posting, counting postings the catalogue filter rejected and the
   aggregator coverage gate skipped — the crawl reached them either way;
3. the board entry names a board (a boardless entry has no board scope, since its postings are
   namespaced with an empty board and a board-scoped match would select the whole provider);
4. the provider does not declare a wider sweep grace, is not self-closing, and is not a
   full-catalogue source.

Condition 2 carries the safety and is not an optimisation: a board that returns ZERO postings is
indistinguishable from a board whose crawl broke, and closing within it would repeat the
truncated-crawl false-close that once retired a live Workday board's tail.

The board scope is what retires a company whose LAST posting left a board we still crawl. The
company scope cannot: it is derived from postings written, so such a company never enters it and
its rows stay open forever. The company scope is kept beside it because it reaches what the board
scope cannot — boardless entries, and boards that yielded nothing.

Each board-scoped close SHALL be logged with its board and the number of jobs it closed, so a
fleet cycle is readable per board rather than as one provider-level number.

#### Scenario: Stale job is closed

- **WHEN** a sweep runs after a provider ingested at least one job and an open job of that
  provider — belonging to a company the run wrote — was last seen 49 hours ago
- **THEN** that job's `closed_at` is set and the job stops appearing in list surfaces

#### Scenario: A company whose last posting left a covered board IS closed

- **WHEN** a board yields postings for company A but none for company B, B has an open job of
  that provider last seen 49 hours ago, and B's job belongs to that same board
- **THEN** B's job is closed — the board listed its content and did not list that job. This is
  the case the company scope alone leaks, because B never enters the crawled-slug set

#### Scenario: A board that yielded nothing closes nothing

- **WHEN** a board's crawl succeeds but returns zero postings, and that board has an open job
  last seen 49 hours ago
- **THEN** the job remains open. A zero-yield crawl cannot be distinguished from a broken one

#### Scenario: A board whose crawl failed closes nothing

- **WHEN** a board's crawl fails and that board has an open job last seen 49 hours ago
- **THEN** the job remains open

#### Scenario: A board the run did not reach is not swept

- **WHEN** a run crawls a subset of a provider's boards — a targeted run, a shard, or a full
  crawl that timed out — and an open job on an unreached board was last seen 49 hours ago
- **THEN** that job remains open, through both scopes

#### Scenario: A partial-coverage provider is excluded from the board scope

- **WHEN** a provider whose adapter declares a wider sweep grace crawls a board that yields
  postings, and that board has an open job past the default window
- **THEN** no board-scoped close runs for that provider — its crawl reaches only a slice of the
  catalogue, so a posting that merely drifted past the crawl's depth reads as unseen

#### Scenario: A boardless entry is not swept by board

- **WHEN** an entry names no board and its crawl yields postings
- **THEN** no board-scoped close runs for it; only the company scope applies

#### Scenario: One board's close leaves another board of the same provider alone

- **WHEN** two boards of one provider both hold stale open jobs and only the first is crawled
- **THEN** only the first board's stale jobs are closed

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

#### Scenario: A board-scoped close queues its rows for search deletion

- **WHEN** a board-scoped close retires open jobs
- **THEN** every retired job is enqueued in `search_delete_outbox` in the same statement, so a
  rolled-back sweep queues nothing and only rows that actually closed are queued — the same
  guarantee the company-scoped close already gives
