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

- **WHEN** a marked adapter's listing walk reaches a page that yields no new postings
- **THEN** the walk ends and `Fetch` returns the postings gathered as a success, with no error

#### Scenario: Reaching the source's own declared total proves completeness

- **WHEN** a marked adapter's listing walk has gathered as many postings as the source's own
  reported total states, before exhausting the page-cap safety constant
- **THEN** the walk ends and `Fetch` returns the postings gathered as a success, with no error

### Requirement: The marker registry names every adapter that meets the bar

`FullBoardListingProviders` SHALL report, for a given provider registry, exactly the set of
adapters that implement the `fullBoardListing` marker interface. `hh`, `neogov`, `edjoin`,
`workstream`, `peopleforce` and `gusto` SHALL be included, alongside the providers marked by prior
changes.

#### Scenario: A newly-qualified hand-rolled adapter is reported

- **WHEN** `FullBoardListingProviders` is called over the full provider registry
- **THEN** its result includes `hh`, `neogov`, `edjoin`, `workstream`, `peopleforce` and `gusto`
