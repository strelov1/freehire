## MODIFIED Requirements

### Requirement: The marker registry names every adapter that meets the bar

`FullBoardListingProviders` SHALL report, for a given provider registry, exactly the set of
adapters that implement the `fullBoardListing` marker interface. `neogov`, `edjoin`,
`workstream`, `peopleforce`, `gusto`, `teamtailor`, `bayt` and `apploi` SHALL be included,
alongside the providers marked by prior changes. `hh` SHALL NOT be included: it cannot
structurally prove completeness for a board whose result count exceeds the source's own
search-depth limit, independent of any cap the adapter itself sets.

#### Scenario: A newly-qualified hand-rolled adapter is reported

- **WHEN** `FullBoardListingProviders` is called over the full provider registry
- **THEN** its result includes `neogov`, `edjoin`, `workstream`, `peopleforce`, `gusto`,
  `teamtailor`, `bayt` and `apploi`

#### Scenario: An adapter bounded by the source's own depth limit is not reported

- **WHEN** `FullBoardListingProviders` is called over the full provider registry
- **THEN** its result does not include `hh`

#### Scenario: A page shorter than the requested page size proves completeness for offset/limit pagination

- **WHEN** a marked adapter's listing walk uses offset/limit pagination and receives a page
  carrying fewer rows than the page size it requested
- **THEN** the walk ends and `Fetch` returns the postings gathered as a success, with no error
  — a full-size page can never be an offset/limit API's genuinely last one, so a short page is
  as unambiguous a natural-end signal as a literally empty page
