# full-board-listing-marker Specification

## Purpose
TBD - created by archiving change fullboardlisting-hand-rolled-batch. Update Purpose after archive.
## Requirements
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

### Requirement: A board-scoped close is available only to a source that proves listing completeness

The system SHALL restrict the per-run board-scoped unseen-job close to sources whose
adapter structurally proves, for every board it crawls, that its listing reached the
board's true end — either by verifying the number of postings retrieved against a count
the source itself declares, or by paginating until the source returns a genuinely empty
page. A page or offset safety ceiling in the adapter is permitted only when reaching it
is itself treated as a proof failure (see the next requirement) rather than as an
alternate, silent way to end the walk successfully — a ceiling that could be reached and
still return its partial result as success does not satisfy this requirement.

An adapter's self-declaration or code comment describing its own completeness SHALL NOT be
sufficient on its own: the proof SHALL be enforced by the adapter's own control flow, not
merely asserted about it.

#### Scenario: A source without an enforced completeness proof is excluded

- **WHEN** a source's adapter has not been shown to structurally prove listing completeness
- **THEN** its boards are not eligible for the board-scoped unseen-job close

### Requirement: A crawl that cannot prove completeness fails rather than reporting a partial result

When an adapter eligible for the board-scoped close encounters anything that prevents it
from proving a board's listing is complete — a page request failing after the first page,
or the walk exhausting its own safety ceiling without ever finding the board's natural end
— the crawl for that board SHALL fail as a whole. It SHALL NOT return the postings gathered
up to that point as an ordinary successful result.

#### Scenario: A later page failing fails the whole crawl

- **WHEN** an eligible source's listing walk succeeds on at least one page and then fails to
  fetch a later page of the same board
- **THEN** the crawl for that board reports a failure, not the postings already gathered

#### Scenario: Exhausting the pagination ceiling without a natural end fails the crawl

- **WHEN** an eligible source's listing walk reaches its own configured page ceiling without
  ever reaching a genuinely empty page
- **THEN** the crawl for that board reports a failure, not the postings already gathered

#### Scenario: A non-empty page of only already-seen postings does not prove completeness

- **WHEN** an eligible source's listing walk reaches a page that carries postings, but every
  one of them was already gathered from an earlier page
- **THEN** the walk does NOT treat that page as the board's natural end — a "genuinely empty
  page" means the RAW page carries no postings at all, not that none of its postings were new

### Requirement: teamtailor proves listing completeness

The `teamtailor` source adapter SHALL be eligible for the board-scoped unseen-job close: its
listing walk SHALL paginate until a page carrying no postings at all is reached (checked
before any cross-page deduplication), with a page ceiling set as a safety backstop far above
any board size observed in production, and SHALL fail the crawl rather than report a partial
result when a later page fails to fetch, or the ceiling is reached without finding a
genuinely empty page.

#### Scenario: A teamtailor board's postings are fully enumerated

- **WHEN** a `teamtailor` board's listing is crawled and its true end is reached before the
  adapter's page ceiling
- **THEN** every posting on the board is retrieved

