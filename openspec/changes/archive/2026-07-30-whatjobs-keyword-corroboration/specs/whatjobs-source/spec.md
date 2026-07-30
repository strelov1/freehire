## ADDED Requirements

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
