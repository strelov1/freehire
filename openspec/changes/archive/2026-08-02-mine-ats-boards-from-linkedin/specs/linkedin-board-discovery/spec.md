## ADDED Requirements

### Requirement: A query worklist drives the public job search

The discovery tool SHALL take its search worklist from a file of queries kept in the
repository, each query naming the keywords and the market to search, and optionally how
recent a posting must be and how many result pages to page through. The tool SHALL page
the platform's public, unauthenticated job-search surface per query, and SHALL treat a
worklist entry that names no market as a usage error rather than searching globally, since
an unbounded market is what turns a bounded harvest into an unbounded crawl.

#### Scenario: Each worklist query is searched across its requested pages

- **WHEN** the worklist names a query with keywords, a market, and a page count
- **THEN** the tool requests that many result pages for that query and collects the
  postings from all of them

#### Scenario: Recency and page count fall back to bounded defaults

- **WHEN** a worklist query omits the recency window or the page count
- **THEN** the tool applies its documented defaults rather than paging without bound

#### Scenario: A query without a market is refused

- **WHEN** a worklist query names no market
- **THEN** the tool reports a usage error and makes no requests

### Requirement: Companies are de-duplicated and filtered before any extra request

The discovery tool SHALL read the employer's name and platform profile from the search
result itself, and SHALL collapse the postings of one employer to a single candidate
before fetching anything further. A candidate whose normalized-name slug is present in a
supplied set of existing company slugs SHALL be dropped at that point, so no request is
spent on a company the catalogue already covers.

#### Scenario: Several postings by one employer become one candidate

- **WHEN** a search returns multiple postings from the same employer
- **THEN** the employer is carried forward once, and only one posting of theirs is
  fetched in detail

#### Scenario: A company already in the catalogue costs no request

- **WHEN** a candidate employer's normalized-name slug is present in the supplied slug set
- **THEN** the candidate is dropped without fetching its posting or its profile page

### Requirement: Each candidate carries its website and its ATS-native posting id

For every candidate that survives filtering, the discovery tool SHALL emit the employer's
name, the employer's own website, and — when the posting publishes one — the identifier
the posting carries in the employer's own applicant tracking system. The website SHALL be
read from the employer's public profile and the posting identifier from the posting's
structured job metadata. A candidate whose posting publishes no such identifier SHALL
still be emitted, without one; a candidate whose website cannot be determined SHALL be
omitted, since there is nothing for the resolve step to follow.

#### Scenario: A candidate with a website and a posting id is emitted in full

- **WHEN** a candidate's profile publishes a website and its posting's metadata carries an
  ATS-native identifier
- **THEN** the candidate is emitted with name, website, and that identifier

#### Scenario: A missing posting id does not disqualify a candidate

- **WHEN** a candidate's posting publishes no ATS-native identifier
- **THEN** the candidate is still emitted, with its name and website and no identifier

#### Scenario: A candidate without a website is omitted

- **WHEN** a candidate's website cannot be determined from its profile
- **THEN** the candidate is not emitted

#### Scenario: One unreachable candidate does not abort the run

- **WHEN** fetching one candidate's posting or profile fails
- **THEN** that candidate is skipped and logged, and the run continues with the others

### Requirement: An empty search result is reported as a failure, not as an absence

A search request that succeeds but yields no postings SHALL be logged as a warning rather
than silently counted as an empty market, and a run in which **every** query yields no
postings SHALL exit non-zero. A markup change or a block returns exactly the shape an
honestly empty market returns, and a harvest that reports success while collecting nothing
is indistinguishable from one that ran correctly.

#### Scenario: A single empty query warns and continues

- **WHEN** one query returns a successful response containing no postings
- **THEN** the tool logs a warning for that query and continues with the remaining queries

#### Scenario: An entirely empty run fails

- **WHEN** every query in the worklist returns no postings
- **THEN** the tool exits non-zero

### Requirement: Requests identify the project and are rate-limited

The discovery tool SHALL issue its requests through the project's shared HTTP client, under
the project's own User-Agent, and SHALL NOT impersonate a browser. The aggregate request
rate SHALL be gated by a rate limiter whose default is conservative and which the operator
can lower further, so the worker pool bounds requests in flight while the limiter bounds
requests per second.

#### Scenario: Requests carry the project's identity

- **WHEN** the tool issues any request to the platform
- **THEN** the request goes out under the project's own User-Agent

#### Scenario: The aggregate rate is bounded independently of concurrency

- **WHEN** the tool runs with its default settings
- **THEN** the outgoing request rate is capped by the rate limiter, and the operator can
  cap it lower
