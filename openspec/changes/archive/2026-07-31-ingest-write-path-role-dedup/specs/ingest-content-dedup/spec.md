## ADDED Requirements

### Requirement: A repost is marked when it is first written

The ingest write path SHALL determine, in the same transaction that persists a newly inserted
posting, whether the posting's role cluster already holds an open canonical row older than it,
and SHALL mark the new row `duplicate_of` that canon when one exists. The canon consulted
SHALL be the one the periodic recompute would choose (the `min(id)` among the cluster's open
rows), so the two never disagree; a candidate canon that is NEWER than the row being written
SHALL be ignored rather than marked onto. A row so marked SHALL be excluded from the live
search index and from the enrichment enqueue by the same rules that already govern a row
marked by the recompute.

The determination SHALL be limited to newly inserted rows. A posting that becomes a duplicate
only later — because an edit made its title and description match a sibling's — and the
release of a row whose canon has closed both remain the periodic recompute's responsibility. A
failed lookup SHALL leave the row unmarked rather than fail the write, since deduplication is
an improvement on the write and never a condition of keeping the vacancy.

#### Scenario: A per-city fan-out collapses as it is ingested

- **WHEN** one crawl writes several postings of the same role that differ only by location, so
  they share a `role_fingerprint`
- **THEN** the oldest is canonical, every later copy carries `duplicate_of` pointing at it, and
  only the canonical row is pushed to the live search index

#### Scenario: A fresh repost is invisible before the next recompute

- **WHEN** a subscription digest or the jobs list is served after such a crawl but before the
  next batch recompute runs
- **THEN** the role appears once, not once per copy

#### Scenario: A newly written repost is not enriched

- **WHEN** a posting is marked `duplicate_of` as it is written
- **THEN** it is not enqueued for enrichment, so an invisible row costs no LLM call

#### Scenario: A re-crawled posting is not re-examined

- **WHEN** an existing posting is written again by a later crawl, whether unchanged or edited
- **THEN** no canon lookup is made for it and its existing marker stands, leaving the periodic
  recompute to revise it
