# similar-jobs Specification

## Purpose
TBD - created by archiving change add-similar-jobs. Update Purpose after archive.
## Requirements
### Requirement: Similar-documents query over the semantic index

The system SHALL provide a way to retrieve the jobs whose embeddings are nearest
to a given job, by querying the existing `jobs_semantic` index with
Meilisearch's similar-documents operation keyed by the job's internal `id` and
the configured embedder. It SHALL NOT change the embedder, the document
template, the index settings, or any reindex path.

The source job SHALL never appear in its own similar list. The number of results
SHALL be bounded by a caller-supplied limit. Because the semantic index contains
documents only for open jobs, similar results SHALL be open jobs without any
additional filter.

#### Scenario: Nearest neighbours are returned for a job

- **WHEN** the similar-documents query is run for a job that has indexed
  neighbours in the semantic index
- **THEN** other jobs close to it in embedding space are returned, up to the
  requested limit

#### Scenario: The source job is excluded from its own results

- **WHEN** the similar-documents query is run for a job
- **THEN** that same job is not present in the returned list

#### Scenario: A job with no neighbours yields an empty list

- **WHEN** the similar-documents query is run and no other documents are close
  enough (or the index holds only the source job)
- **THEN** an empty list is returned rather than an error

### Requirement: Public similar-jobs endpoint

The system SHALL expose `GET /api/v1/jobs/:slug/similar` as a public
(unauthenticated) endpoint. It SHALL resolve `:slug` to the job's internal `id`,
run the similar-documents query, and respond with the standard list envelope
`{"data": [...]}` whose `data` is the neighbouring jobs in the public job wire
shape. Each result SHALL identify its job by `public_slug` and SHALL NOT include
the internal numeric `id`, consistent with the other public job reads. An
optional `limit` query parameter SHALL bound the number of results, clamped to a
sane maximum, with a default when absent.

Requesting similar jobs for an unknown slug SHALL return 404. The existing public
job reads SHALL be unchanged.

#### Scenario: Similar jobs for a known slug

- **WHEN** a client requests `GET /api/v1/jobs/<known-slug>/similar`
- **THEN** the response is `{"data": [...]}` listing neighbouring open jobs, each
  carrying its `public_slug` and omitting the internal numeric `id`

#### Scenario: Limit bounds the result count

- **WHEN** a client requests `GET /api/v1/jobs/<known-slug>/similar?limit=3`
- **THEN** at most 3 jobs are returned

#### Scenario: Unknown slug is a 404

- **WHEN** a client requests `GET /api/v1/jobs/<unknown-slug>/similar`
- **THEN** the response status is 404

### Requirement: Similar-jobs section on the job detail page

The SPA job detail page SHALL display a "Similar jobs" section populated from the
`/similar` endpoint, linking each neighbour to its own detail page. The section
SHALL degrade silently when there are no neighbours or the request fails: it
SHALL render nothing and SHALL NOT break or block the rest of the page.

#### Scenario: Section shows neighbours

- **WHEN** a user opens a job detail page that has similar jobs
- **THEN** a "Similar jobs" section lists those jobs, each linking to its detail
  page

#### Scenario: Section is absent when there are none

- **WHEN** a user opens a job detail page with no similar jobs (or the similar
  request fails)
- **THEN** no "Similar jobs" section is shown and the rest of the page renders
  normally

