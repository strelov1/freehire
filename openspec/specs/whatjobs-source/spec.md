# whatjobs-source Specification

## Purpose
The WhatJobs FeedAPI adapter: freehire publishes for this CPC network, and its feed is read as
a keyword-sliced aggregator of US inventory. The capability covers the request shape and the
vendor's documented-but-broken parameters, keyword pagination and its depth ceiling, posting
identity derived from the tracked click-through URL, and the normalization that discards the
feed's placeholder fields.

## Requirements
### Requirement: The WhatJobs feed is crawled as a keyword-sliced aggregator

The system SHALL crawl the WhatJobs FeedAPI as a multi-company aggregator whose board is a
search keyword, registered under the provider key `whatjobs`. Each board entry's `company` is
a display label only — the employer of record comes from each posting's own `company` field.
The adapter SHALL be marked an aggregator so the cross-source dedup pass may suppress its copy
in favour of a first-party ATS posting of the same role. It SHALL NOT be marked boardless (the
keyword is mandatory) and SHALL NOT be marked full-catalogue (a crawl reaches only a slice).

#### Scenario: Keyword board is crawled

- **WHEN** a board entry for provider `whatjobs` carries the keyword `backend engineer`
- **THEN** the adapter requests that keyword's postings and returns them as jobs whose company
  is each posting's own employer, not the entry's display label

#### Scenario: Aggregator copy loses to a first-party posting

- **WHEN** the feed returns a posting for a role the catalogue already holds from that company's
  own ATS board
- **THEN** the cross-source dedup pass treats the `whatjobs` row as the aggregator copy and the
  ATS posting wins

#### Scenario: A board entry without a keyword fails validation

- **WHEN** a `sources/whatjobs.yml` entry omits `board`
- **THEN** config validation fails fast and the run does not start

### Requirement: The publisher id is read from the environment only

The system SHALL read the WhatJobs publisher id from the `WHATJOBS_PUBLISHER_ID` environment
variable and MUST NOT accept it from a board file. The provider SHALL be registered in the
adapter registry only when that variable is set to a non-empty value, so an environment without
the credential leaves `whatjobs` absent from the registry rather than registering a provider
that cannot crawl.

#### Scenario: Configured environment registers the provider

- **WHEN** `WHATJOBS_PUBLISHER_ID` is set and the registry is assembled
- **THEN** the registry contains a `whatjobs` adapter carrying that publisher id

#### Scenario: Unconfigured environment omits the provider

- **WHEN** `WHATJOBS_PUBLISHER_ID` is unset or empty and the registry is assembled
- **THEN** the registry has no `whatjobs` entry, and a board file naming that provider fails
  validation fast

### Requirement: Posting identity is derived from the tracked click-through URL

The system SHALL derive each posting's native id from the tracked URL's
`pub_api__cpl__<id>__<publisher>` path segment, and SHALL store the tracked URL as the job's
posting URL. The tracked URL is not bound to the requesting IP, so a stored copy stays valid
for any later visitor and the publisher attribution survives. A posting whose URL does not
carry that segment SHALL be skipped rather than stored under a guessed id.

#### Scenario: Native id is extracted

- **WHEN** a posting's url is `https://www.whatjobs.com/pub_api__cpl__2737655843__7065?utm_source=7065`
- **THEN** the job's native id is `2737655843` and its URL is the tracked url as returned

#### Scenario: Unrecognized URL shape is skipped

- **WHEN** a posting's url does not contain a `pub_api__cpl__<digits>__<digits>` segment
- **THEN** that posting is not returned as a job

#### Scenario: A page where nothing carries an id fails the board

- **WHEN** a page returns postings and not one of them carries a recognizable native id
- **THEN** the board's crawl fails with an error naming the keyword, rather than reporting an empty
  result — a provider credited with zero ingested jobs is skipped by the unseen sweep, so a silent
  empty would leave every existing row open indefinitely with nothing refreshing it

### Requirement: A blank keyword is refused rather than crawled

The system MUST NOT request the feed when the board's keyword is empty or whitespace-only, and SHALL
fail that board with an error naming the company. The feed answers a blank keyword with its entire
unfiltered inventory — tens of thousands of postings, most not technical — so one padded board entry
would otherwise pour that into the catalogue. Config validation rejects only a strictly empty board,
which is why the adapter checks after trimming. A keyword padded with surrounding whitespace is a
real keyword and SHALL be sent trimmed.

#### Scenario: A whitespace-only keyword issues no request

- **WHEN** a board entry's keyword is `"   "`
- **THEN** the crawl fails for that entry and no feed request is made

#### Scenario: A padded keyword is trimmed and used

- **WHEN** a board entry's keyword is `"  golang  "`
- **THEN** the feed is queried for `golang`

### Requirement: The feed's junk fields are discarded rather than stored

The system SHALL ignore the feed's `salary`, `job_type` and `logo` fields, which carry no
information: `salary` is always the literal `0.000000 - 0.000000`, `job_type` is always empty
and `logo` is always null. The system SHALL strip the reseller signature (a trailing
`#J-<digits>-Ljbffr` marker, present on 96% of descriptions) from the description text before
the job is persisted.

#### Scenario: Placeholder salary is not stored as a salary

- **WHEN** a posting reports `salary` as `0.000000 - 0.000000`
- **THEN** the job carries no salary

#### Scenario: Reseller signature is stripped

- **WHEN** a posting's description ends with `#J-18808-Ljbffr`
- **THEN** the persisted description does not contain that marker

### Requirement: The account's country is stated alongside the posting's city

The system SHALL compose each job's location from the posting's city and the country the publisher
account serves, because the feed names no country and its cities collide with better-known foreign
ones (its London is in Ohio, its Vienna in Virginia). The country is a property of the credential —
the vendor issues one publisher id per country — and not a guess about an individual posting, so it
does not breach the dict-only rule. A posting with no city SHALL carry the country alone, with no
dangling separator for the geography tokenizer to read as an empty token.

#### Scenario: City is qualified by the account country

- **WHEN** a posting's location is `New York`
- **THEN** the job's location names both `New York` and the account's country

#### Scenario: A posting without a city carries the country alone

- **WHEN** a posting's location is empty
- **THEN** the job's location is exactly the account's country

### Requirement: The feed's age field never becomes a posting date

The system MUST NOT map the feed's `age` or `age_days` onto the job's posted date. Those fields
report how long the record has been in the reseller's index rather than when the employer
published the role — postings from unrelated companies routinely share one `age_days` value —
so treating them as a posting date would misorder every freshness-sorted surface.

#### Scenario: Age is not persisted as a posted date

- **WHEN** a posting reports `age_days` as 109
- **THEN** the job's posted date is left unset rather than computed from that value

### Requirement: Pagination stops at the feed's ceiling and its short pages

The system SHALL page through a keyword's results until a page returns no postings, the
configured page budget is exhausted, or the feed's depth ceiling is reached, and SHALL treat a
page returning fewer postings than requested as normal rather than as the last page. The feed
clamps any requested page size above 50, returns fewer rows than asked for once a keyword is
given (it post-filters duplicates), and serves nothing beyond roughly two thousand pages even
when it reports a far larger total.

#### Scenario: A short page does not end pagination

- **WHEN** a page requested with a size of 50 returns 44 postings and the next page returns more
- **THEN** the crawl continues to the next page

#### Scenario: An empty page ends pagination

- **WHEN** a page returns zero postings
- **THEN** the crawl of that keyword stops and the postings gathered so far are returned

#### Scenario: The page budget bounds a huge keyword

- **WHEN** a keyword reports a total far beyond the crawl's page budget
- **THEN** the crawl stops at the budget and the run succeeds with the slice it read

#### Scenario: The bounded crawl says so

- **WHEN** the page budget rather than an empty page is what ended a keyword's crawl
- **THEN** the run logs that the slice is bounded, so a partial read is never mistaken for full
  coverage of that keyword

### Requirement: A keyword's repeated postings are collapsed before they are returned

The system SHALL return each posting at most once per keyword crawl, keyed on the native posting id.
The feed post-filters duplicates only within a page, so the same posting can surface on several
pages of one keyword; returning it each time would have the pipeline upsert one row repeatedly
across a deep crawl for no gain.

#### Scenario: The same posting on two pages is returned once

- **WHEN** two pages of one keyword both carry a posting with the same native id
- **THEN** the crawl returns that posting once

### Requirement: The request avoids the feed's documented-but-broken parameters

The system MUST NOT send a `user_agent` query parameter containing a forward slash, and MUST NOT
rely on the `unique_id` parameter for deduplication. A slash in `user_agent` causes the edge to
redirect the request with the value corrupted, which is why the vendor's own documented examples
fail; `unique_id` does not suppress previously returned postings despite the documentation
claiming it does.

#### Scenario: User agent carries no slash

- **WHEN** the adapter builds a feed request
- **THEN** any `user_agent` value it sends contains no `/` character

#### Scenario: An invalid publisher id surfaces as a board failure

- **WHEN** the feed rejects the configured publisher id (it answers `410` with an
  `Invalid Publisher` body)
- **THEN** the board's crawl fails with an error, is counted as a failure, and does not close
  any existing job

### Requirement: A posting must corroborate the keyword it was found under

The system SHALL keep a posting only when every corroborating term of its board's keyword appears in
the posting's title or description, and SHALL drop it otherwise. Corroborating terms are the keyword's
words with generic role words removed (`developer`, `engineer`, `programmer`, `dev`, `specialist`,
`architect`), because those appear in almost every technical posting and so distinguish nothing:
`rust developer` corroborates on `rust`, `kubernetes engineer` on `kubernetes`. A keyword made
entirely of generic words has no corroborating term, and its postings SHALL pass unfiltered rather
than all be dropped.

The feed's `keyword` ranks by relevance instead of filtering, and pads a thin result set with
unrelated inventory — one board returned 193 postings of which 144 were a hospital group's radiology
listings. The non-technical dictionary does not catch that padding, since titles like
"CT Technologist" read as technical.

#### Scenario: A posting naming the keyword's term is kept

- **WHEN** a posting found under keyword `rust developer` mentions `Rust` in its title
- **THEN** the posting is returned

#### Scenario: Padding that never names the term is dropped

- **WHEN** a posting found under keyword `rust developer` is titled `Senior CT Technologist - PRN`
  and its description never mentions rust
- **THEN** the posting is not returned

#### Scenario: The generic half of a keyword does not have to appear

- **WHEN** a posting found under keyword `rust developer` is titled `Senior Rust Engineer` — naming
  rust but never "developer"
- **THEN** the posting is returned

#### Scenario: A wholly generic keyword filters nothing

- **WHEN** a board's keyword is `developer`, leaving no corroborating term
- **THEN** its postings are returned without corroboration filtering

### Requirement: Pagination ends when a page stops corroborating

The system SHALL stop crawling a keyword when a page's share of corroborated postings falls below a
minimum, treating the keyword as exhausted, and SHALL log that it stopped for this reason. Relevance
does not taper — it collapses, from 100% on the first page to 15% on the second — so a corroborated
share below the minimum means the feed has begun padding and every further page is mostly waste. This
also bounds the request volume that trips the feed's rate limit.

#### Scenario: A collapsed page ends the crawl

- **WHEN** a keyword's first page corroborates fully and its second page corroborates 15%
- **THEN** the crawl keeps the corroborated postings from both pages and requests no third page

#### Scenario: A fully corroborating keyword crawls on

- **WHEN** every page of a keyword corroborates above the minimum until the feed runs dry
- **THEN** the crawl continues to the empty page as before

### Requirement: Feed requests run under a shared in-flight cap

The system SHALL bound how many WhatJobs requests are in flight at once across all of the provider's
boards in a run. The pipeline crawls boards in parallel, and unbounded that parallelism rate-limits
the feed — 8 of 10 boards failed with HTTP 429 on the first production run, while the same requests
issued sequentially succeed.

#### Scenario: Parallel boards share one cap

- **WHEN** several whatjobs boards crawl concurrently in one run
- **THEN** the number of simultaneous feed requests never exceeds the configured cap
