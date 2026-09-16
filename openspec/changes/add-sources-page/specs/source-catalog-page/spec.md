## ADDED Requirements

### Requirement: Public source catalogue endpoint

The system SHALL serve `GET /api/v1/sources` without authentication, returning every
source the catalogue is built from: every registered adapter — including the ones
currently carrying no postings — AND every source the catalogue holds postings under even
where no adapter is registered for it. Each entry carries its kind, its health status, its
job counts, when it was last read, how much that read took, and the instant the figures
were measured.

The second half of that union is load-bearing: a source can carry postings without being a
crawl adapter (an extraction pipeline, a manual intake), and a page claiming to list every
source cannot be built on a list that omits one silently.

The response SHALL be sanitized by construction: it carries no board identifier and no
crawl error text, the same rule the existing status endpoint holds itself to.

#### Scenario: Every registered source appears

- **WHEN** a client requests the endpoint
- **THEN** the response lists one entry per registered source adapter, whether or not that
  source has open postings

#### Scenario: A source with postings but no adapter appears

- **WHEN** the catalogue holds open postings under a source with no registered adapter
- **THEN** that source appears too, with its counts and a kind of `other`

#### Scenario: No internal detail leaks

- **WHEN** a source is failing to crawl
- **THEN** its entry reports a status and nothing about which board failed or why

#### Scenario: Unauthenticated read

- **WHEN** the endpoint is requested with no credential
- **THEN** it answers 200

### Requirement: Source kind classification

Each source SHALL be classified as one of: an ATS platform, an aggregator, or a company
career page. The classification SHALL come from the adapter registry's own markers — the
same taxonomy the status page reads — never from a list maintained beside it.

#### Scenario: Classification matches the registry

- **WHEN** an adapter is registered as an aggregator
- **THEN** the endpoint reports its kind as aggregator

#### Scenario: A new adapter needs no second edit

- **WHEN** a new adapter is added to the registry
- **THEN** it appears on the endpoint with its kind, with no edit to this feature

### Requirement: ATS-overlap figure for aggregators

For a source classified as an aggregator, the response SHALL carry how many of its open
postings the dedup pass matched to a first-party ATS posting, and how many it did not.

This figure SHALL NOT be presented as a count of exclusive postings, in the wire shape or
on the page. The dedup pass failing to find a pair is not evidence that no pair exists —
it is the absence of evidence — and a name like `exclusive_jobs` would launder that
absence into a claim within one reading.

#### Scenario: An aggregator carries the pair

- **WHEN** the entry for an aggregator is read
- **THEN** it carries both the matched count and the unmatched count

#### Scenario: A non-aggregator carries neither

- **WHEN** the entry for an ATS platform or a company career page is read
- **THEN** it carries no overlap figures at all, rather than zeros — the dedup pass never
  marks these rows, so a zero here would read as "fully exclusive"

### Requirement: Degraded figures are reported, not zeroed

When the snapshot has not been measured, or a figure within it could not be measured, the
endpoint SHALL report that figure as absent. It SHALL NOT substitute zero.

#### Scenario: The snapshot has never run

- **WHEN** `source_stats` holds no rows
- **THEN** the endpoint answers 200 with every source listed, its job counts absent, and
  the health figures it can still read from the fleet health rollup

### Requirement: The public source page

The system SHALL serve a public page at `/sources` listing every source, grouped by kind,
ordered within each group by job count descending.

Each entry SHALL state the source's display name, its logo where one resolves, its
de-duplicated job count as a link to that source's own filtered search, when it was last
successfully read, and how many postings that read returned.

A logo SHALL be resolved from the source's DISPLAY NAME, never from a posting host — a
host serves the employer's mark under the platform's name — and SHALL fall back to a
monogram when it does not resolve, including when the miss happened before hydration.

#### Scenario: The count matches what the link opens

- **WHEN** a visitor reads a source's job count and follows its link
- **THEN** the search that opens is filtered to that source, and its total is the same
  figure — the de-duplicated one, not the raw one

#### Scenario: Grouped and ordered

- **WHEN** the page renders
- **THEN** sources appear under their kind's heading, the largest first within each group

### Requirement: The page states what the overlap figure means

Where the page shows an aggregator's ATS overlap, it SHALL describe the unmatched postings
as postings not matched to a first-party ATS posting, never as exclusive postings, and
SHALL say so in the visitor's own words rather than only in a tooltip.

#### Scenario: Wording

- **WHEN** an aggregator's overlap is rendered
- **THEN** the visible text describes the figure as unmatched, and the word "exclusive"
  does not appear

### Requirement: Client-side search over the loaded list

The page SHALL offer a search box that filters the already-loaded list in the browser.
Typing in it SHALL issue no request.

Every source is already in the payload; a server round-trip per keystroke would buy
nothing and would put a few-hundred-row public page on the request path of every visitor who types.

#### Scenario: Filtering issues no request

- **WHEN** a visitor types in the search box
- **THEN** the list narrows and no network request is made

#### Scenario: No match

- **WHEN** the query matches no source
- **THEN** the page states that plainly rather than rendering empty group headings

### Requirement: The page is cheap to serve and cheap to load

The page's assembled payload SHALL be memoized server-side for a short window, so
concurrent visitors share one build rather than each re-fanning out to the API.

Source logos SHALL be loaded lazily. A few hundred eagerly-fetched logo requests would cost more
than everything else on the page put together.

#### Scenario: Concurrent loads share a build

- **WHEN** the page is requested repeatedly within the memoization window
- **THEN** the upstream API is called once

#### Scenario: Logos load lazily

- **WHEN** the page renders
- **THEN** logo images below the fold are not fetched until they approach the viewport

#### Scenario: A logo that does not resolve

- **WHEN** the logo service answers 404 for a source, whether before or after hydration
- **THEN** the card shows a monogram, never a broken-image icon

### Requirement: The page is reachable

The page SHALL be linked from the site's own navigation and listed in the pages sitemap.

A transparency page nothing links to is not transparency.

#### Scenario: Listed in the sitemap

- **WHEN** the pages sitemap is fetched
- **THEN** it contains the `/sources` URL
