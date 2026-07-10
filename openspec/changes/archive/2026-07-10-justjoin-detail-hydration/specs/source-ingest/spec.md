## ADDED Requirements

### Requirement: Adapters may hydrate only postings the catalogue lacks

The pipeline SHALL support an optional adapter capability that fetches per-posting detail
only for postings the catalogue does not already have, so an adapter whose detail requests
are expensive (a large aggregator) issues them only for new postings rather than on every
crawl. The capability SHALL be opt-in through a distinct adapter interface (not a change to
`Source.Fetch`), so every other adapter and the base fetch signature are unchanged — the
same optional-port shape the pipeline already uses for streaming and self-closing sources.

When a board's adapter implements this capability, the pipeline SHALL supply a
provider-scoped `seen(externalID)` predicate backed by the set of `external_id`s already
stored for that provider, obtained with a single query per crawl (not one query per
posting). An adapter that does not implement the capability SHALL be crawled through the
normal fetch path, unaffected.

A hydrating adapter re-lists postings it already ingested but fetches no fresh content for them,
so the pipeline MUST refresh such a posting's liveness (last-seen state, and reopen if closed)
WITHOUT rewriting its content columns. A content-less re-upsert would re-derive the deterministic
facets from an empty description and overwrite the row's hydrated description/skills/etc. The
adapter SHALL mark an already-ingested posting for liveness refresh; the pipeline routes a marked
posting to the refresh path instead of the content upsert. (This mirrors the removed-posting
close routing.) A refresh keeps the posting out of the post-run unseen sweep just as a full
upsert would.

#### Scenario: Already-ingested posting refreshes liveness without a content rewrite

- **WHEN** a hydrating adapter yields a posting marked for liveness refresh (already ingested)
- **THEN** the pipeline refreshes its last-seen/open state and does NOT overwrite its stored
  description or facets

#### Scenario: Hydrating adapter is driven by the seen-set

- **WHEN** a board's adapter implements the hydrating capability
- **THEN** the pipeline loads the set of already-ingested external ids for that provider once
  and passes a membership predicate to the adapter, which hydrates only postings not in the set

#### Scenario: Non-hydrating adapter is unaffected

- **WHEN** a board's adapter does not implement the hydrating capability
- **THEN** the pipeline crawls it through the normal fetch path and issues no seen-set query

#### Scenario: Seen-set lookup failure does not abort the crawl

- **WHEN** the seen-set query fails for a hydrating board
- **THEN** the pipeline logs the failure and crawls the board with an empty seen-set (every
  offer treated as new) rather than skipping the board
