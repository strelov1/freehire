# ingest-write-path Specification

## Purpose
How ingest writes the jobs adapters yield into the catalogue: the normalized, namespaced write path, enrichment kept across re-ingest, persisted geography and work mode, the new and changed writes reported to the search index, and the repost-clustering role fingerprint. Split out of source-ingest, which holds the rules every adapter shares.

## Requirements

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

### Requirement: Re-ingest preserves existing enrichment

The ingest write path SHALL NOT write or overwrite a job's enrichment payload or
provenance. Re-ingesting a job that has already been enriched SHALL leave its
`enrichment`, `enriched_at`, and `enrichment_version` unchanged, so that source
re-ingestion never wipes enrichment produced by the enrichment worker.

#### Scenario: Enrichment survives a re-ingest

- **WHEN** an already-enriched job is re-ingested with updated source fields
- **THEN** the job's source fields update but its `enrichment`, `enriched_at`, and
  `enrichment_version` are unchanged

### Requirement: Ingest persists job geography and work mode

The ingest write path SHALL parse each posting's `location` string into
`countries`/`regions`/`work_mode` (via the job-geography parser) and persist them
on the job row. These columns SHALL be written on insert and refreshed on
re-ingest, like the other raw source fields and unlike the enrichment payload
(which ingest never writes). A posting whose location yields no geography SHALL
store empty arrays.

For `work_mode`, when the adapter exposes a STRUCTURED work mode (a workplace-type
enum or an explicit remote flag from the ATS API) it SHALL take precedence over
the parser's free-text heuristic; the parser fills `work_mode` only when the
adapter has no structured signal.

When an ATS exposes BOTH a work-mode field and a boolean flag for the same posting, the
adapter SHALL resolve the mode from the field, and MAY fall back to the flag only for
postings that omit the field — for a posting that carries the field, a flag that merely
separates onsite from every other arrangement SHALL NOT be mapped to `remote`, because
such a flag is set on hybrid postings too. Where an ATS splits the signal across separate
`remote` and `hybrid` booleans, both false SHALL yield no work mode rather than a guessed
`onsite`, since the API cannot distinguish "marked as office" from "not marked at all";
both true SHALL yield `remote`, the broader arrangement.

#### Scenario: A new posting stores its parsed geography

- **WHEN** a posting with location `Remote - Germany` is ingested
- **THEN** the stored job has `countries=[de]` and `regions` including `eu`

#### Scenario: Re-ingest refreshes geography from the updated location

- **WHEN** an already-ingested posting is re-ingested with its location changed
  from `Remote - UK` to `Remote - USA`
- **THEN** the job's `countries` updates to `[us]` and its `regions` update
  accordingly

#### Scenario: A location with no geography stores empty arrays

- **WHEN** a posting with location `Remote` is ingested
- **THEN** the stored job has empty `countries` and empty `regions`

#### Scenario: A structured adapter work mode is persisted over the parsed one

- **WHEN** an adapter reports a structured `work_mode` (e.g. Lever's
  `workplaceType=hybrid`) for a posting whose location parses as `remote`
- **THEN** the stored `jobs.work_mode` is the structured value `hybrid`

#### Scenario: An Ashby hybrid posting is not remote

- **WHEN** an Ashby posting carries `workplaceType=Hybrid` together with `isRemote=true`
- **THEN** the yielded job's `work_mode` is `hybrid` and the job is not marked remote

#### Scenario: An Ashby posting without a workplace type falls back to the flag

- **WHEN** an Ashby posting omits `workplaceType` and carries `isRemote=true`
- **THEN** the yielded job's `work_mode` is `remote`

#### Scenario: A Recruitee hybrid offer keeps its work mode

- **WHEN** a Recruitee offer carries `remote=false` and `hybrid=true`
- **THEN** the yielded job's `work_mode` is `hybrid` and the job is not marked remote

#### Scenario: A SmartRecruiters hybrid posting keeps its work mode

- **WHEN** a SmartRecruiters posting's location carries `remote=false` and `hybrid=true`
- **THEN** the yielded job's `work_mode` is `hybrid` and the job is not marked remote

#### Scenario: Neither remote nor hybrid leaves the work mode unknown

- **WHEN** an offer carries `remote=false` and `hybrid=false`
- **THEN** the yielded job carries no structured `work_mode`, leaving the decision to the
  pipeline's location parser

#### Scenario: An offer flagged both remote and hybrid resolves to remote

- **WHEN** an offer carries `remote=true` and `hybrid=true`
- **THEN** the yielded job's `work_mode` is `remote`

#### Scenario: A BambooHR posting takes its work mode from locationType

- **WHEN** a BambooHR careers-list posting carries `locationType` `0`, `1`, or `2` with a
  null `isRemote`
- **THEN** the yielded job's `work_mode` is `onsite`, `remote`, or `hybrid` respectively,
  and only the `remote` one is marked remote

### Requirement: The write path reports new and changed writes for indexing

The job write path SHALL maintain a deterministic content hash over a job's
indexed fields and SHALL report, for each write, whether it **inserted** a new
row or **changed** the indexed content of an existing one. An upsert that updates
only bookkeeping (such as the last-seen timestamp) without changing any indexed
field SHALL report neither inserted nor changed. The hash SHALL be derived from
the same persisted values that form the search document, so the signal tracks
exactly what the index would need re-pushed.

#### Scenario: A first-time write reports inserted

- **WHEN** a posting not previously in the catalogue is persisted
- **THEN** the write reports it as inserted

#### Scenario: An edited write reports changed

- **WHEN** an existing posting is persisted with an edited indexed field (e.g. its
  title)
- **THEN** the write reports it as changed

#### Scenario: A no-op refresh reports neither

- **WHEN** an existing posting is persisted with identical indexed fields and only
  its last-seen timestamp advances
- **THEN** the write reports neither inserted nor changed

### Requirement: Ingest feeds new and changed jobs to the search index

When the search engine is configured for the ingest worker, the worker SHALL push
the open jobs reported as inserted or changed to the live facet search index,
batched rather than one document per posting, after they are persisted. The push
SHALL be best-effort: a search-index error SHALL be logged and SHALL NOT fail the
ingest run or change its exit code. When the search engine is not configured for
the worker, ingest SHALL run unchanged and skip indexing.

#### Scenario: Configured worker indexes new and changed jobs

- **WHEN** a crawl persists new and edited jobs and the search engine is configured
- **THEN** those jobs' documents are pushed to the live facet index in batches

#### Scenario: Unconfigured worker skips indexing

- **WHEN** ingest runs without a configured search engine
- **THEN** ingest persists and sweeps as before and pushes nothing to the index

#### Scenario: Index error does not fail the run

- **WHEN** pushing documents to the search engine fails
- **THEN** the error is logged and the ingest run's success is unaffected

### Requirement: The write path persists a repost-clustering role fingerprint
The `UpsertJob` write path SHALL compute and persist a narrow `role_fingerprint` for every job — derived from company, normalized title, and description, and deliberately excluding volatile fields (`posted_at`, url, public slug) — so reposts of the same role under new `external_id`s cluster to one fingerprint.

#### Scenario: The fingerprint ignores a bumped posted date
- **WHEN** a role is reposted under a new `external_id` with an identical title and description but a refreshed `posted_at`
- **THEN** both postings resolve to the same `role_fingerprint`

#### Scenario: content_hash is not reused as the fingerprint
- **WHEN** the write path computes the fingerprint
- **THEN** it uses a dedicated narrow fingerprint (not `content_hash`, which includes `posted_at` and so changes on repost)

#### Scenario: A distinct role gets a distinct fingerprint
- **WHEN** two postings differ in normalized title or description
- **THEN** they resolve to different `role_fingerprint` values
