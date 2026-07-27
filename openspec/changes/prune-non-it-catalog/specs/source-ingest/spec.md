## MODIFIED Requirements

### Requirement: Ingest writes jobs through a normalized, namespaced write path

The pipeline SHALL persist each fetched posting that passes the catalogue-fit check via the existing job write path, and SHALL reject — without persisting — any posting whose title the non-tech dictionary flags. It
SHALL set `source` to the board's provider, derive `company_slug` from the company
name using the existing slug normalization, and set `external_id` to the namespaced
form `"<board>:<native-posting-id>"`. Namespacing SHALL guarantee that two companies
on the same platform sharing a native posting id do not collide on the dedup key
`UNIQUE (source, external_id)`. Rejections MUST be counted separately from postings skipped by a construct or save error, so a catalogue-fit rejection never reads as a board malfunction, and a board with any rejections SHALL be logged once per run with its rejected share — a board rejecting everything is the signature of a dictionary term that is too broad, and must be visible within one crawl cycle. The rejection applies only to the crawled board pipeline; write paths that are never re-crawled are unaffected, because a rejection there could not be undone by a later crawl.

#### Scenario: External id is namespaced by board

- **WHEN** a posting with native id `42` is ingested for board `cohere` on provider
  `greenhouse`
- **THEN** the stored job has `source = "greenhouse"` and
  `external_id = "cohere:42"`

#### Scenario: Same native id on different boards does not collide

- **WHEN** two boards on the same provider each return a posting with native id `42`
- **THEN** both jobs are stored as distinct rows, differing in `external_id`

#### Scenario: Re-ingest of the same posting updates in place

- **WHEN** a posting already in the catalogue is ingested again with an edited title
- **THEN** the existing row is updated rather than duplicated, keyed on
  `(source, external_id)`

#### Scenario: A non-technical posting is rejected before the write

- **WHEN** a fetched posting's title is flagged by the non-tech dictionary
- **THEN** the posting is not persisted and is counted as rejected, not as skipped

#### Scenario: Rejections do not mask save failures

- **WHEN** a board yields both a rejected posting and a posting that fails to save
- **THEN** the run reports one rejection and one skip separately

#### Scenario: A board rejecting everything is visible

- **WHEN** a board's postings are all rejected in a run
- **THEN** the run logs that board once with its rejected count and share

#### Scenario: Non-crawled write paths are unaffected

- **WHEN** a job is created through Telegram extraction, a user submission, or a link-source import and its title would be flagged
- **THEN** the job is still persisted
