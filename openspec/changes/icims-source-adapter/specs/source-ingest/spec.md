## ADDED Requirements

### Requirement: An adapter MAY enumerate a board through a sitemap with JSON-LD detail

The system SHALL support adapters that enumerate a board's postings from the
board's XML sitemap rather than a JSON list endpoint, and that obtain each
posting's fields from a server-rendered detail page carrying a schema.org
`JobPosting` JSON-LD block. Such an adapter SHALL filter non-posting sitemap
entries, SHALL derive the platform's native posting id from the posting URL, and
SHALL bound its per-posting detail fetches (as required of any detail-fetching
adapter). A posting whose detail page cannot be fetched or carries no usable
posting id SHALL be skipped without aborting the rest of the board.

#### Scenario: Adapter enumerates postings from a sitemap

- **WHEN** an adapter fetches a board whose postings are listed only in an XML
  sitemap (e.g. an iCIMS career site)
- **THEN** the adapter reads the sitemap, keeps only the job-posting URLs, and
  fetches each posting's detail page to obtain the normalized job shape

#### Scenario: Adapter reads posting fields from JSON-LD detail

- **WHEN** a posting's detail page server-renders a schema.org `JobPosting`
  JSON-LD block
- **THEN** the adapter maps that block to the normalized job shape (at least
  title, url, location, description, and the platform's native posting id)

#### Scenario: A posting whose detail fetch fails is skipped

- **WHEN** one posting's detail page cannot be fetched or yields no posting id
- **THEN** that posting is dropped and the remaining postings on the board are
  still ingested
