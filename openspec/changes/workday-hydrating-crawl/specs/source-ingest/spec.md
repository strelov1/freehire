## MODIFIED Requirements

### Requirement: Adapters may hydrate only postings the catalogue lacks

The pipeline SHALL support an optional adapter capability that fetches per-posting detail
only for postings the catalogue does not already have, so an adapter whose detail requests
are expensive (a large aggregator, or a board with tens of thousands of postings) issues them
only for new postings rather than on every crawl. The capability SHALL be opt-in through a
distinct adapter interface (not a change to `Source.Fetch`), so every other adapter and the
base fetch signature are unchanged — the same optional-port shape the pipeline already uses
for streaming and self-closing sources.

When a board's adapter implements this capability, the pipeline SHALL supply a `seen(externalID)`
predicate backed by the set of `external_id`s already stored for **that board**, obtained with a
single query per crawled board (not one query per posting). The seen-set SHALL be scoped by the
board's `external_id` namespace prefix rather than by provider alone, because the lookup runs once
per board: a provider-wide set makes the capability unusable for a multi-board provider, whose
per-board cost would be the whole provider's catalogue. A boardless adapter, whose postings carry
no board prefix, SHALL keep the provider-wide set. An adapter that does not implement the
capability SHALL be crawled through the normal fetch path, unaffected.

A hydrating adapter re-lists postings it already ingested but fetches no fresh content for them,
so the pipeline MUST refresh such a posting's liveness (last-seen state, and reopen if closed)
WITHOUT rewriting its content columns. A content-less re-upsert would re-derive the deterministic
facets from an empty description and overwrite the row's hydrated description/skills/etc. The
adapter SHALL mark an already-ingested posting for liveness refresh; the pipeline routes a marked
posting to the refresh path instead of the content upsert. (This mirrors the removed-posting
close routing.) A refresh keeps the posting out of the post-run unseen sweep just as a full
upsert would.

A liveness refresh SHALL be subject to the same catalogue-fit check as a content upsert: a
posting the non-tech dictionary flags is neither written nor refreshed, so it stops being seen
and the unseen sweep closes it. Without this, hydration would keep a rejected posting alive
indefinitely and defeat the pruning path that depends on a rejected posting ageing out. A
refusal to refresh SHALL be counted as a catalogue rejection, not as a skip.

The check on a refresh SHALL read the tech evidence STORED with the row rather than what the
re-listed posting alone implies, so it reaches the same verdict the write path would. The
dictionary is consulted only after the tech check, and a hydrating crawl carries no description
to derive that check from: judged on the listing alone, 1.7% of the titles the catalogue holds
as technical are flagged — engineering roles whose evidence lives in the description they were
hydrated from. The seen-set SHALL therefore carry each stored posting's tech evidence alongside
its id, since it is read from the same rows.

#### Scenario: Already-ingested posting refreshes liveness without a content rewrite

- **WHEN** a hydrating adapter yields a posting marked for liveness refresh (already ingested)
- **THEN** the pipeline refreshes its last-seen/open state and does NOT overwrite its stored
  description or facets

#### Scenario: Hydrating adapter is driven by the seen-set

- **WHEN** a board's adapter implements the hydrating capability
- **THEN** the pipeline loads the set of already-ingested external ids for that board once
  and passes a membership predicate to the adapter, which hydrates only postings not in the set

#### Scenario: The seen-set of one board excludes a sibling board on the same host

- **WHEN** two boards of one provider share a host prefix (e.g. `…/dollartreeus` and
  `…/dollartreeca`) and each has stored postings
- **THEN** a board's seen-set contains only its own postings, so a posting of the sibling board
  is treated as new rather than as already ingested

#### Scenario: A boardless adapter keeps the provider-wide seen-set

- **WHEN** a hydrating adapter is boardless, so its stored `external_id`s carry an empty board
  prefix
- **THEN** the pipeline supplies the set of every `external_id` stored for the provider

#### Scenario: Non-hydrating adapter is unaffected

- **WHEN** a board's adapter does not implement the hydrating capability
- **THEN** the pipeline crawls it through the normal fetch path and issues no seen-set query

#### Scenario: Seen-set lookup failure does not abort the crawl

- **WHEN** the seen-set query fails for a hydrating board
- **THEN** the pipeline logs the failure and crawls the board with an empty seen-set (every
  offer treated as new) rather than skipping the board

#### Scenario: A rejected posting is not kept alive by a liveness refresh

- **WHEN** a hydrating adapter marks an already-ingested posting for liveness refresh, the
  non-tech dictionary flags its title, and its stored row carries no tech evidence
- **THEN** the pipeline does NOT refresh it, counts it as a catalogue rejection, and the posting
  ages out to the unseen sweep

#### Scenario: A flagged title whose stored row proves it technical is still refreshed

- **WHEN** a posting marked for liveness refresh has a title the dictionary flags but its stored
  row carries tech evidence (the description it was hydrated from established it)
- **THEN** the pipeline refreshes it and counts no rejection — the same verdict the write path
  reaches with the description in hand

## ADDED Requirements

### Requirement: The workday adapter hydrates only postings the catalogue lacks

The `workday` adapter SHALL implement the hydrating capability, because a Workday board carries
one posting per detail request and a large board (over twenty thousand postings) is rate-limited
into failure when every crawl re-fetches all of them. It SHALL list a board exactly as the
non-hydrating path does — the listing page size is fixed by the platform and a larger requested
limit yields no postings — and SHALL issue a detail request only for a posting the seen-set does
not contain, marking an already-ingested posting for liveness refresh instead. When the pipeline
cannot supply a seen-set, the adapter SHALL fall back to fetching every posting's detail, as
before.

#### Scenario: Detail is fetched only for postings not already ingested

- **WHEN** the pipeline supplies a seen-set and a listed posting's namespaced id is already
  ingested
- **THEN** the adapter marks that posting for a liveness refresh and issues NO detail request
- **AND WHEN** a listed posting is not in the seen-set
- **THEN** the adapter fetches that posting's detail and emits the hydrated posting

#### Scenario: A liveness refresh carries the posting's identity

- **WHEN** the adapter marks a listed posting for liveness refresh
- **THEN** the emitted posting carries the same external id, title and URL the full path would
  have produced, so the pipeline can resolve it to the stored row

#### Scenario: Falls back to hydrating every posting without a seen-set

- **WHEN** the pipeline cannot supply a seen-set (e.g. a non-DB caller)
- **THEN** the adapter fetches every listed posting's detail, as before
