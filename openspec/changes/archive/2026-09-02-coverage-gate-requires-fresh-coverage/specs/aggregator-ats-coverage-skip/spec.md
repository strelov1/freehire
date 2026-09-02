## MODIFIED Requirements

### Requirement: An aggregator posting is not saved when its company already has non-aggregator coverage

The system SHALL, when ingesting a posting from a provider `sources.ProviderKind` classifies as
`KindAggregator`, skip saving that posting when the company already has at least one posting for
which ALL of the following hold:

- the posting is OPEN (`closed_at IS NULL`);
- its source is NOT in `sources.AggregatorProviders(sources.Taxonomy())`;
- its `last_seen_at` is within the coverage freshness window;
- it is NOT private;
- its `company_slug_folded` equals the asked slug with its hyphens removed.

Matching on the folded slug alone is deliberate and is not a widening of today's behaviour
beyond the fold itself: `company_slug_folded` is `replace(company_slug,'-','')` written by the
same upsert that writes `company_slug`, so an exact-slug match always implies a folded match.
The exact clause the search-backed lookup carried existed only because that engine could not
compute the fold at query time; against the stored column it is redundant.

The company SHALL be asked about using the alias-resolved slug taken from the board run's single
shared resolution map, so the value the gate asks about is the value the upsert would write.
A skipped posting SHALL be counted in a dedicated `Stats.ATSCovered` counter, distinct from
`Stats.Rejected`.

The freshness window SHALL be a single compile-time constant of 14 days, not configurable per
run. Coverage is a claim about the present — that the catalogue still crawls this employer — and
a posting nothing has seen for longer than the window is not evidence for it.

Sharing one map is the requirement, not merely sharing one function. The gate and the upsert
previously agreed only because each independently applied the same pure derivation; once the
slug depends on registry state, an independently derived value can silently disagree, and a gate
that silently stops matching looks exactly like a gate that found nothing to skip.

#### Scenario: Aggregator posting for a recently-seen covered company is skipped

- **WHEN** an aggregator-provider board yields a posting for a company that already has an open
  posting from a non-aggregator source whose `last_seen_at` is inside the freshness window
- **THEN** the posting is not saved, and `Stats.ATSCovered` is incremented for that board

#### Scenario: Aggregator posting for an uncovered company is saved normally

- **WHEN** an aggregator-provider board yields a posting for a company with no open posting
  from any non-aggregator source
- **THEN** the posting is saved exactly as it is today, and `Stats.Ingested` is incremented

#### Scenario: Coverage older than the freshness window does not suppress the posting

- **WHEN** every open non-aggregator posting for a company was last seen longer ago than the
  freshness window — for example the board it came from has left `sources/` and is no longer
  crawled at all
- **THEN** the company reads as UNCOVERED, the aggregator posting is saved, and
  `Stats.ATSCovered` is NOT incremented. A stale row is not evidence that the catalogue still
  carries this employer, and treating it as evidence discards a live posting permanently

#### Scenario: A private posting is never coverage

- **WHEN** the only open, recently-seen, non-aggregator posting for a company is private (the
  jd-tailor-intake path: a job description one user pasted in, visible only to them)
- **THEN** the company reads as UNCOVERED and the aggregator posting is saved. A private
  posting is crawled from nowhere, so it cannot be evidence that the catalogue still crawls
  the employer — and if it were, one user's pasted description for a common company name would
  silently discard every aggregator posting for every other employer of that name

#### Scenario: A streaming aggregator source is gated the same as a buffered one

- **WHEN** an aggregator provider that fetches via a streaming source (postings emitted one at
  a time rather than as one fetched batch) emits a posting for a covered company
- **THEN** the posting is not saved, and `Stats.ATSCovered` is incremented — the gate applies
  regardless of whether the provider's adapter buffers or streams

#### Scenario: Coverage matches the hyphen-folded slug as well as the exact one

- **WHEN** an aggregator posting's `company_slug` and a fresh open non-aggregator posting's
  `company_slug` differ only by hyphenation (e.g. `cfoinsights` vs `cfo-insights`)
- **THEN** the company IS treated as covered and the aggregator posting is skipped. The lookup
  reads Postgres, where the fold is the stored `jobs.company_slug_folded` column with its own
  partial index, so both spellings are ONE indexed predicate rather than the two OR'd clauses
  the search engine's filter language forced, and rather than a query-time computation it
  cannot express at all

### Requirement: A missing coverage lookup disables the gate without failing ingest

The system SHALL behave exactly as it does without this change (write every posting that passes
the existing catalogue filter) when no coverage lookup is configured for the run, and SHALL
treat a coverage lookup that returns an error as reporting that nothing is covered.

Both are the same rule, and the rule follows from the asymmetry of the two failures: a posting
wrongly suppressed is never written and so cannot be found or repaired, while a posting wrongly
saved is a duplicate the periodic `aggregator-ats-dedup` pass already marks on its own schedule.

#### Scenario: Coverage lookup is not configured

- **WHEN** the ingest `Runner` has no coverage lookup wired (e.g. no database handle, or a test
  fake that does not implement it)
- **THEN** aggregator postings are saved exactly as they were before this change, and no error
  is raised

#### Scenario: Coverage lookup fails

- **WHEN** the configured coverage lookup returns an error for a batch
- **THEN** every company in that batch reads as uncovered, the run continues, and the failure is
  logged rather than failing the board
