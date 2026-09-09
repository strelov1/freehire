## MODIFIED Requirements

### Requirement: The marker registry names every adapter that meets the bar

`FullBoardListingProviders` SHALL report, for a given provider registry, exactly the set of
adapters that implement the `fullBoardListing` marker interface. `neogov`, `edjoin`, `workstream`,
`peopleforce`, `gusto`, `teamtailor` and `bayt` SHALL be included, alongside the providers marked
by prior changes. `hh` SHALL NOT be included: it cannot structurally prove completeness for a board
whose result count exceeds the source's own search-depth limit, independent of any cap the adapter
itself sets.

#### Scenario: A newly-qualified hand-rolled adapter is reported

- **WHEN** `FullBoardListingProviders` is called over the full provider registry
- **THEN** its result includes `neogov`, `edjoin`, `workstream`, `peopleforce`, `gusto`,
  `teamtailor` and `bayt`

#### Scenario: An adapter bounded by the source's own depth limit is not reported

- **WHEN** `FullBoardListingProviders` is called over the full provider registry
- **THEN** its result does not include `hh`

#### Scenario: An adapter with a documented concurrency-triggered throttling risk is still eligible when its listing walk is sequential

- **WHEN** a marked adapter's own code documents a live-observed throttling risk tied to a
  CONCURRENT request pattern (e.g. a bounded detail-fetch fan-out), and its listing walk is a
  separate, single-threaded, sequential pagination loop
- **THEN** that documented risk does not by itself disqualify the adapter from the marker — the
  bar is evaluated against the listing walk's own structural proof, not against an unrelated
  concurrent code path's risk profile

#### Scenario: A per-posting detail failure on a throttling-prone platform is still reported, not silently dropped

- **WHEN** a marked adapter with no hydrating fallback fetches a posting's detail and the
  request fails with the platform's own documented throttling signal (e.g. a 403 from a
  concurrent burst), rather than its "this posting is gone" signal (404/410)
- **THEN** `Fetch` still reports that posting via the `unreadableDetail` marker rather than
  silently omitting it — a throttled request is not evidence the posting is gone
