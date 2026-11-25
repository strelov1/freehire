## ADDED Requirements

### Requirement: Greenhouse and Ashby boards are discovered from the Common Crawl CDX index

The harvest tool SHALL support discovering Greenhouse and Ashby boards from the Common Crawl
CDX index. Discovery SHALL read the current snapshot list from Common Crawl's collection index
and query the 3 most recent snapshots for URLs under the provider's board host
(`boards.greenhouse.io` for Greenhouse, `jobs.ashbyhq.com` for Ashby), paging each snapshot's
result set. For each matched URL, the first non-empty path segment SHALL be extracted, lower
cased, and collected as a candidate board id; the same candidate found under multiple snapshots
or multiple URLs SHALL be collected once. A snapshot whose query fails SHALL be skipped with the
run continuing on the remaining snapshots; discovery SHALL fail only when every queried snapshot
fails.

#### Scenario: Candidates are collected across the most recent snapshots

- **WHEN** Common Crawl discovery runs for Greenhouse or Ashby
- **THEN** it queries the 3 most recent snapshots listed by Common Crawl's collection index and
  returns the deduplicated set of candidate board ids found across all of them

#### Scenario: A candidate is extracted from the URL's first path segment

- **WHEN** a matched CDX record's URL is `https://boards.greenhouse.io/acme/jobs/12345`
- **THEN** the candidate board id collected is `acme`

#### Scenario: A candidate seen more than once is collected only once

- **WHEN** the same board id appears in more than one matched URL, or in more than one queried
  snapshot
- **THEN** it appears exactly once in the discovered candidate list

#### Scenario: One failing snapshot does not stop discovery

- **WHEN** one of the queried snapshots' CDX requests fails
- **THEN** discovery logs the failure, continues querying the remaining snapshots, and returns
  the candidates collected from the snapshots that succeeded

#### Scenario: Every snapshot failing is a discovery error

- **WHEN** every queried snapshot's CDX request fails
- **THEN** discovery returns an error and the run reports it rather than silently producing an
  empty candidate list

#### Scenario: Discovered candidates are validated exactly like any other candidate

- **WHEN** a candidate board id discovered from Common Crawl is probed
- **THEN** it is live-validated, de-duplicated against `sources/<provider>.yml`, and appended (or
  skipped) through the same pipeline a seeded or platform-discovered candidate would use
