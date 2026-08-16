# seek-source Specification

## Purpose
TBD - created by archiving change add-seek-source-adapter. Update Purpose after archive.
## Requirements
### Requirement: SEEK per-market, per-subclassification listing crawl

The system SHALL provide a `seek` source adapter that crawls SEEK's Australian and New Zealand
ICT catalogues into the catalogue. The adapter is a **board-based multi-company aggregator**: each
configured entry names a MARKET in its region field (`au` or `nz`) and an ICT **subclassification
id** in its board field, not a per-company board. The crawl is keyless — SEEK's search API requires
no credential, no cookie, and no browser-shaped User-Agent.

#### Scenario: Slice yields its listed postings

- **WHEN** the adapter crawls a configured (market, subclassification) entry
- **THEN** it returns one `Job` per posting the search listing yields for that slice, each populated
  with the posting's id, title, employer, free-text location, structured country, work mode,
  employment type and listing date

#### Scenario: Market selects host, site key and search scope together

- **WHEN** an entry names market `au`
- **THEN** the adapter requests SEEK's Australian host with that market's site key AND its
  whole-country search scope, because omitting the scope does not mean "everywhere" — it collapses
  the result set to a small unrelated subset

#### Scenario: Unknown market fails the board

- **WHEN** an entry names a market the adapter does not know
- **THEN** the crawl returns a board-level error naming the entry, rather than guessing a market

### Requirement: Listing walk bounded by observation, not by reported total

The adapter MUST advance through listing pages until a page yields no posting it has not already
collected. It MUST NOT use the listing's own `totalCount` to drive or bound pagination: SEEK reports
that field as a function of the requested page size (the same query answers a small nonsense number
at page size 1 and the real figure at page size 20), so it can neither schedule the walk nor detect
truncation.

#### Scenario: Walk stops when a page adds nothing new

- **WHEN** a listing page yields only postings already collected, or no postings at all
- **THEN** the walk ends and the adapter returns what it gathered

#### Scenario: Result window is backstopped

- **WHEN** SEEK keeps serving pages past its result window
- **THEN** the walk still stops at a fixed page ceiling, so a misbehaving edge cannot loop the crawl

### Requirement: Which page failed decides whether the board failed

The adapter SHALL treat a FIRST-page failure as a board-level error and a LATER-page failure as the
end of the walk, returning the postings gathered so far — the repository-wide rule for a paginated
listing walk. The adapter is not a `fullCatalog` source, so a partial crawl is safe.

#### Scenario: First page fails

- **WHEN** the first listing page of a slice cannot be fetched or decoded
- **THEN** the crawl returns an error, so board health records the failure and the sweep does not
  read the slice as emptied

#### Scenario: Later page fails

- **WHEN** a listing page after the first fails
- **THEN** the walk ends and the adapter returns the postings collected from the earlier pages

### Requirement: Descriptions hydrated only for postings the catalogue lacks

The listing carries no description, only a one-line teaser, so the adapter SHALL implement
`HydratingSource` and fetch a posting's body from SEEK's GraphQL `jobDetails` operation ONLY when
the pipeline reports that posting as not yet ingested. A posting the catalogue already holds MUST be
returned marked `SeenRefresh`, so the pipeline refreshes its liveness without re-fetching or
overwriting the body hydrated when it was new.

The detail fetch MUST be rate-paced through a limiter shared by every board in a run, so the run's
aggregate request rate stays under SEEK's burst window independently of the detail pool's
concurrency.

A posting whose description could not be fetched MUST NOT be ingested. It is dropped for that run,
so the next crawl still reports it as new and retries it. This is the opposite of the rule the other
hydrating adapters follow, and it is deliberate: that rule trades a rare missing body for keeping the
posting, but SEEK's endpoint refuses in bursts, so ingesting body-less would strand whole slices with
no description permanently — the `seen` predicate reports only row existence, never whether the row
carries a body, so nothing downstream can repair it.

#### Scenario: New posting is hydrated

- **WHEN** the crawl yields a posting the catalogue does not hold
- **THEN** the adapter fetches its description and returns the job with that body

#### Scenario: Known posting costs no detail request

- **WHEN** the crawl yields a posting the catalogue already holds
- **THEN** the adapter returns it marked `SeenRefresh` and issues no detail request for it

#### Scenario: Failed detail never drops a posting

- **WHEN** a posting's detail request fails or returns no content
- **THEN** the adapter omits that posting from THIS run rather than storing it body-less, so the
  next crawl still reports it as new and retries the fetch — the posting is deferred, not lost.
  This reverses the behaviour this scenario originally described (ingest the posting list-only):
  a body-less row is unrecoverable past the hydration retry window, while a deferred posting is
  retried by every later crawl

#### Scenario: Detail requests are paced across the whole run

- **WHEN** many boards hydrate postings concurrently in one run
- **THEN** every detail request passes through one shared rate limiter, so the run's aggregate rate
  stays under SEEK's window rather than scaling with the number of boards in flight

### Requirement: Employer resolved per posting, placeholder rejected

The adapter MUST read each posting's employer from the posting itself, preferring the profiled
employer name and falling back to the advertiser name SEEK shows when no profile exists. SEEK's
"Private Advertiser" placeholder is NOT an employer; a posting carrying it, or carrying no name at
all, MUST be dropped rather than persisted under a placeholder company.

#### Scenario: Profiled employer wins

- **WHEN** a posting names a profiled employer
- **THEN** that name is the job's company

#### Scenario: Advertiser name is the fallback

- **WHEN** a posting has no profiled employer but names an advertiser
- **THEN** the advertiser name is the job's company

#### Scenario: Placeholder advertiser is dropped

- **WHEN** a posting's only employer name is SEEK's "Private Advertiser" placeholder
- **THEN** the adapter omits that posting

### Requirement: Structured facets carried only where SEEK states them

The adapter SHALL map SEEK's structured fields into freehire's controlled vocabularies — work
arrangement to work mode, work type to employment type, the posting's country code to a canonical
country — and leave a field empty when SEEK states no value, so the pipeline's dictionaries decide.
SEEK's salary is a free-text label rather than a structured amount, so it MUST be folded into the
description instead of the structured salary fields.

#### Scenario: Work arrangement maps to work mode

- **WHEN** a posting states a work arrangement
- **THEN** the job carries the corresponding work mode, preferring the most remote arrangement the
  posting offers

#### Scenario: Unstated arrangement leaves work mode empty

- **WHEN** a posting states no work arrangement
- **THEN** the job's work mode is empty, so the pipeline's location heuristic decides

#### Scenario: Salary label is not a structured amount

- **WHEN** a posting carries a salary label
- **THEN** it is folded into the description and the job's structured salary fields stay unset

### Requirement: Sweep grace covers the unreachable tail

The adapter SHALL declare a post-run sweep grace window wider than the default, because SEEK stops
serving results past roughly the 550th and the busiest slices hold more postings than that. Ordered
newest-first, the reachable window covers most of a SEEK advertisement's run, but a posting that
drifts past it would otherwise be closed and reopened as it drifts back, writing a phantom removal
each cycle. SEEK's own job pages sit behind the same interstitial as its search pages, so liveness
cannot be probed instead.

#### Scenario: Grace window is declared

- **WHEN** the ingest worker computes the unseen-job cutoff for `seek`
- **THEN** it reads the adapter's declared window rather than the default

### Requirement: Aggregator identity in the source taxonomy

The adapter MUST register as an `aggregator` so the cross-source dedup pass prefers a first-party
ATS copy of a posting over SEEK's re-listing. It MUST NOT register as `boardless`: the board selects
the slice to crawl and is required.

#### Scenario: SEEK copy loses to the first-party posting

- **WHEN** the same vacancy exists both on an employer's own ATS board and on SEEK
- **THEN** the dedup pass keeps the ATS copy and suppresses the SEEK one

#### Scenario: Entry without a board fails validation

- **WHEN** a `seek` entry omits its board
- **THEN** board-file validation rejects it before any request goes out

