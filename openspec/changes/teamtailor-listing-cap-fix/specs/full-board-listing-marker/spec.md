## Purpose

The observable contract behind `sources.fullBoardListing`: what an ATS source adapter must
prove about a board's crawl before its boards are trusted for the per-run board-scoped
unseen-job close, and why an adapter that cannot prove it must fail loudly rather than
report a partial crawl as an ordinary success.

## ADDED Requirements

### Requirement: A board-scoped close is available only to a source that proves listing completeness

The system SHALL restrict the per-run board-scoped unseen-job close to sources whose
adapter structurally proves, for every board it crawls, that its listing reached the
board's true end — either by verifying the number of postings retrieved against a count
the source itself declares, or by paginating until the source returns a genuinely empty
page, with no page or offset limit in the adapter that could be reached before the
source's own natural end.

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

- **WHEN** an eligible source's listing walk reaches its own configured page ceiling while
  the source is still returning new postings, never having reached a genuinely empty page
- **THEN** the crawl for that board reports a failure, not the postings already gathered

### Requirement: teamtailor proves listing completeness

The `teamtailor` source adapter SHALL be eligible for the board-scoped unseen-job close: its
listing walk SHALL paginate until a genuinely empty page is reached, with a page ceiling set
as a safety backstop far above any board size observed in production, and SHALL fail the
crawl rather than report a partial result when a later page fails to fetch or the ceiling is
reached without finding the board's natural end.

#### Scenario: A teamtailor board's postings are fully enumerated

- **WHEN** a `teamtailor` board's listing is crawled and its true end is reached before the
  adapter's page ceiling
- **THEN** every posting on the board is retrieved
