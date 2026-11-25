## Purpose

Defines the completeness proof a source adapter's `Fetch` must structurally establish before its
boards may be trusted by the post-run sweep's board-scoped close, and names the adapters that meet
it.

## ADDED Requirements

### Requirement: An adapter earns the full-board-listing marker only by structural proof

An adapter's `Fetch` SHALL be eligible for the `fullBoardListing` marker only when it structurally
proves, for every board it crawls, that it retrieved the board's complete posting list. Structural
proof is one of: (a) a fetched count verified against the source's own reported total, or (b)
pagination to the source's natural end — a genuinely empty page, `hasNext=false`, or a declared
page count reached — with no artificial page or offset cap in the adapter's own code cutting the
walk short. An adapter that cannot establish this proof for a run SHALL fail that `Fetch` rather
than return a partial result.

#### Scenario: A later listing page failing without proof of completeness fails the Fetch

- **WHEN** a marked adapter's listing walk has gathered some postings and a subsequent page fails
  to fetch or decode, before the walk has reached a genuinely empty page or the source's own
  declared total/page count
- **THEN** `Fetch` returns an error rather than the postings gathered so far

#### Scenario: Exhausting the page-cap safety ceiling without proof fails the Fetch

- **WHEN** a marked adapter's listing walk reaches its own page-cap safety constant without ever
  observing a genuinely empty page or reaching the source's own declared total/page count
- **THEN** `Fetch` returns an error naming the safety ceiling, rather than the postings gathered up
  to that point

#### Scenario: A genuinely empty page proves completeness

- **WHEN** a marked adapter's listing walk reaches a page that carries no postings at all, read
  before any cross-page deduplication
- **THEN** the walk ends and `Fetch` returns the postings gathered as a success, with no error

#### Scenario: A non-empty page of only already-seen postings does not prove completeness

- **WHEN** a marked adapter's listing walk reaches a page that carries postings, but every one of
  them was already gathered from an earlier page, and neither a genuinely empty page nor the
  source's own declared total has been reached yet
- **THEN** the walk does NOT end — it continues to the next page, so a genuinely new posting listed
  beyond that page is still reached

#### Scenario: Reaching the source's own declared total proves completeness

- **WHEN** a marked adapter's listing walk has gathered as many postings as the source's own
  reported total states, before exhausting the page-cap safety constant
- **THEN** the walk ends and `Fetch` returns the postings gathered as a success, with no error

#### Scenario: A per-posting detail failure that is not the source's own "gone" signal does not silently drop the posting

- **WHEN** a marked adapter has no hydrating fallback (every crawl re-fetches every listed
  posting's own detail page) and a posting's detail request fails with anything other than the
  source's own 404/410 "this posting is gone" answer
- **THEN** `Fetch` still reports that posting rather than silently omitting it, so a transient
  detail failure cannot read as the posting having disappeared from the board

### Requirement: The marker registry names every adapter that meets the bar

`FullBoardListingProviders` SHALL report, for a given provider registry, exactly the set of
adapters that implement the `fullBoardListing` marker interface. `neogov`, `edjoin`, `workstream`,
`peopleforce` and `gusto` SHALL be included, alongside the providers marked by prior changes. `hh`
SHALL NOT be included: it cannot structurally prove completeness for a board whose result count
exceeds the source's own search-depth limit, independent of any cap the adapter itself sets.

#### Scenario: A newly-qualified hand-rolled adapter is reported

- **WHEN** `FullBoardListingProviders` is called over the full provider registry
- **THEN** its result includes `neogov`, `edjoin`, `workstream`, `peopleforce` and `gusto`

#### Scenario: An adapter bounded by the source's own depth limit is not reported

- **WHEN** `FullBoardListingProviders` is called over the full provider registry
- **THEN** its result does not include `hh`
