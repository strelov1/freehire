## ADDED Requirements

### Requirement: A board addresses one country and one category

The system SHALL address the Adzuna Job Search API as `(country, category)` pairs: the
country is the API path segment (`/jobs/{country}/search/{page}`) and SHALL be carried in the
board's region, the category is a hard `category` filter and SHALL be carried in the board's
board id. Adzuna publishes no "list everything" call, so a boardless crawl is not available.

A board with a blank category SHALL fail rather than fall back to an unfiltered search, since
an unfiltered crawl of a general job board would spend the whole request budget on postings
outside the catalogue's scope.

The category is a filter, not a relevance query: every result on a deep page still carries
the requested category. The adapter therefore SHALL NOT carry the corroboration and
relevance-collapse logic a keyword-search source needs.

#### Scenario: A board's country and category select the request

- **WHEN** a board names region `gb` and board `it-jobs`
- **THEN** the crawl requests `/jobs/gb/search/{page}` with `category=it-jobs`

#### Scenario: A blank category fails the board

- **WHEN** a board carries an empty category
- **THEN** the board fails with an error naming the company, and no request is made

### Requirement: A crawl reads the newest postings first, within a freshness window

The system SHALL request Adzuna's results in date order, newest first, and SHALL bound each
request to a freshness window measured in days.

Without an explicit order Adzuna answers in relevance order, which is stable between runs.
Measured on `gb/it-jobs` on 2026-09-06: the default ordering's first result was published
2026-02-10, seven months earlier, while the same request in date order returned a posting from
that same day. An hourly crawl of a stable relevance ordering re-reads one fixed slice of the
catalogue and reaches almost none of what has been published since.

The freshness window is not redundant with the date order. The page loop ends when a page
returns no results, and against a catalogue tens of thousands of postings deep that never
happens inside the page budget — so a board whose real inflow is smaller than its budget
would spend every remaining request on postings the pipeline's seen-set discards. The window
is what lets the feed run out and the existing exit fire.

#### Scenario: Results arrive newest first

- **WHEN** a board's first page is requested
- **THEN** the request carries a date ordering, and the newest postings the window admits
  are the ones returned

#### Scenario: A quiet board stops early instead of spending its budget

- **WHEN** a board's freshness window holds fewer postings than the page budget could carry
- **THEN** a page returns no results and the crawl for that board ends, leaving the remaining
  requests unmade

#### Scenario: A malformed freshness window fails the request

- **WHEN** a request carries a freshness window the platform cannot parse
- **THEN** the platform answers with a server error and the board fails, rather than silently
  returning an unfiltered result set

### Requirement: The crawl budget stays inside the platform's stated request limits

Adzuna's terms state a ceiling of 250 requests per day and 2,500 per month. The system SHALL
size the Adzuna crawl — page budget per board, boards, and run frequency together — to stay
under the daily ceiling, and SHALL treat that ceiling as a property of the whole schedule
rather than of any one run.

The free API pays nothing and grants nothing: continued access rests on the platform's
tolerance, and the catalogue's largest single source depends on it. A budget that exceeds the
stated ceiling is therefore a standing risk to that whole slice of the catalogue, not a
technical detail.

A page SHALL carry at most 50 results; the platform rejects a larger request outright.

#### Scenario: The schedule stays under the daily ceiling

- **WHEN** the configured boards, page budget and run frequency are multiplied out
- **THEN** the resulting requests per day are below the platform's stated daily ceiling

#### Scenario: A page failure after the first page keeps what was gathered

- **WHEN** a request for a page after the first fails
- **THEN** the crawl for that board stops and returns the postings already gathered, rather
  than discarding them or continuing to spend the budget

### Requirement: An Adzuna posting's liveness cannot be probed by its stored URL

The stored URL is Adzuna's own tracking redirect, which carries the attribution parameters
that credit a click to this publisher. It is not the employer's posting page, and it answers
the same whether or not the posting behind it is live.

The system SHALL NOT treat a response to that URL as evidence of a posting's death.
Measured over 40 sampled stored URLs on 2026-09-06: roughly three quarters answered `403
Access Denied` — the platform's bot protection, not a verdict — with the remainder split
between genuine `404`/`410` and live `200`.

Adzuna postings SHALL therefore be closed by the ingest sweep where it can still see them,
and by the lifecycle's age rule for the tail a bounded, date-ordered crawl never re-reads.

#### Scenario: A blocked probe is not a death verdict

- **WHEN** a stored Adzuna URL answers `403`
- **THEN** the posting is not closed on that basis

#### Scenario: The ageing tail is closed by age

- **WHEN** an open Adzuna posting's effective posting date passes the lifecycle's age window
- **THEN** it is closed with reason `expired`
