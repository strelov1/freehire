## ADDED Requirements

### Requirement: JustJoin list-plus-detail crawl

The system SHALL provide a `justjoin` source adapter that crawls the justjoin.it public
API. The adapter is boardless and multi-company (it stays in the source facet and takes
each posting's company from the feed), paging the cursor list endpoint
`https://api.justjoin.it/v2/user-panel/offers/by-cursor` by the `from` cursor. Because the
list endpoint omits the posting body, the adapter SHALL obtain the description from the
per-offer detail endpoint `https://api.justjoin.it/v1/offers/{slug}`, whose `body` field
carries the description HTML.

#### Scenario: List page maps to jobs

- **WHEN** the adapter reads a cursor page from the list endpoint
- **THEN** it yields one `Job` per offer carrying the offer's title, company, city
  location, work mode (from `workplaceType`), posted-at timestamp, and the offer `guid` as
  the `ExternalID`, synthesizing the canonical detail URL from the offer slug

#### Scenario: Offer without slug, guid, or company is dropped

- **WHEN** an offer in the feed has no slug, no guid, or no company name
- **THEN** the adapter drops that offer (it cannot build a URL, a dedup key, or a company)

### Requirement: JustJoin hydrates the description for unseen offers only

The adapter SHALL fetch per-offer detail only for an offer the catalogue does not already
have, so a steady-state crawl issues detail requests only for new offers. The adapter SHALL
receive a `seen(externalID)` predicate; for an offer whose `guid` is not seen it SHALL fetch
`/v1/offers/{slug}`, put the sanitized `body` into the job description, and — from the same
detail payload — set the structured facets the platform states unambiguously: skills from
`requiredSkills[].name` (canonicalized through the skill dictionary) and seniority from
`experienceLevel.value` (mapped into freehire's seniority vocabulary, with justjoin's `mid`
meaning `middle`), each left empty when a value does not map. The adapter SHALL NOT derive a
category from justjoin's `category`: it is a language/stack tag (JavaScript, Java, Python, …)
that does not pin a single freehire role category, so the title dictionary decides category.
For a seen offer the adapter SHALL yield a liveness-refresh job (no detail request): the pipeline
refreshes the posting's last-seen state (and reopens it if closed) WITHOUT rewriting its content,
so the description and facets hydrated when it was new are preserved rather than overwritten by a
content-less re-upsert.

Detail requests SHALL be bounded (a single crawl SHALL NOT issue unbounded concurrent detail
requests), and a single offer's detail failure SHALL be isolated: the adapter logs and skips
that offer's hydration rather than aborting the crawl.

#### Scenario: New offer is hydrated with a description

- **WHEN** the crawl encounters an offer whose guid is not in the seen set
- **THEN** the adapter fetches that offer's detail and yields a `Job` whose description is
  the sanitized detail `body`, with skills and seniority set from the detail when they map to
  freehire's vocabularies

#### Scenario: Already-ingested offer skips the detail request and preserves content

- **WHEN** the crawl encounters an offer whose guid is already in the seen set
- **THEN** the adapter yields a liveness-refresh `Job` (marked, no detail request), and the
  pipeline only refreshes the row's last-seen/open state — its stored description and facets are
  not overwritten

#### Scenario: A single offer's detail failure does not abort the crawl

- **WHEN** the detail request for one unseen offer fails
- **THEN** the adapter logs and skips that offer's hydration and continues crawling the
  remaining offers

### Requirement: One-time backfill of existing empty JustJoin descriptions

The system SHALL provide a run-once `cmd/backfill-justjoin` worker that, for existing
catalogue rows with `source = 'justjoin'`, fetches each posting's detail from
`/v1/offers/{slug}` (slug taken from the stored URL) and updates the row's description, so
the rows ingested before hydration gain a body and re-index (their `content_hash` moves). A
single posting's detail failure SHALL be isolated and counted, never aborting the run.

#### Scenario: Existing empty row gains a description

- **WHEN** the backfill worker processes a `justjoin` row whose detail fetch succeeds
- **THEN** it updates that row's description with the sanitized detail body

#### Scenario: Backfill isolates a failed posting

- **WHEN** the detail fetch for one row fails or the posting is gone
- **THEN** the worker logs and skips that row and continues with the rest
