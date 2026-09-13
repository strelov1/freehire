## Purpose

Tells a candidate, before ever tailoring a CV or reviewing a form, whether a
posting's ATS is one `cmd/auto-apply` can currently attempt to fill and submit
at all — a best-effort, provider-level signal, never a guarantee of success.

## ADDED Requirements

### Requirement: A fixed provider allow-list decides availability

The system SHALL mark a posting `auto_apply_available` when its `source` is one
of `greenhouse`, `lever`, `ashby`, or `workable` — the ATS platforms the
auto-apply browser driver can currently attempt, whether through its direct
browser automation or its cloud-agent fallback. Every other source, including
`recruitee`, SHALL NOT be marked.

The signal SHALL communicate provider eligibility only, never a submission
guarantee: a marked posting can still park on a live obstacle (a captcha
challenge, an unrecognized form layout, missing candidate answers) discovered
only when an actual attempt runs.

#### Scenario: A Greenhouse posting is marked available

- **WHEN** a posting's source is `greenhouse`
- **THEN** it is marked `auto_apply_available`

#### Scenario: A Lever posting is marked available

- **WHEN** a posting's source is `lever`
- **THEN** it is marked `auto_apply_available`

#### Scenario: An Ashby posting is marked available

- **WHEN** a posting's source is `ashby`
- **THEN** it is marked `auto_apply_available`

#### Scenario: A Workable posting is marked available

- **WHEN** a posting's source is `workable`
- **THEN** it is marked `auto_apply_available`

#### Scenario: A Recruitee posting is not marked

- **WHEN** a posting's source is `recruitee`
- **THEN** it is NOT marked `auto_apply_available`

#### Scenario: Every other source is not marked

- **WHEN** a posting's source is any value other than the four listed providers
- **THEN** it is NOT marked `auto_apply_available`

### Requirement: The signal is a true-or-absent facet, never stored false

The system SHALL express the signal as true-or-absent, matching every other
derived boolean facet in the catalogue (`is_tech`, `ai_interview`,
`requires_clearance`): a non-eligible posting SHALL carry no value for the
facet, never an explicit `false`.

#### Scenario: A non-eligible posting carries no facet value

- **WHEN** a posting's source is not one of the four eligible providers
- **THEN** the served job object carries no `auto_apply_available` key, and the
  search index document carries no value for the attribute

### Requirement: The facet is computed at index time, not stored in Postgres

The system SHALL derive the signal purely from the posting's already-stored
`source` value, computed when the search index document is built, rather than
persisting it as its own column. Because it is a pure function of `source`,
the value SHALL be correct on every future incremental index update without
any separate backfill process.

This differs from a dictionary-derived facet (e.g. `requires_clearance`),
which needs a one-time backfill pass over Postgres because its source text
can't be cheaply reprocessed at index time; this signal has no such gap
because `source` never needs reprocessing.

#### Scenario: A newly ingested posting from an eligible provider is marked on first index

- **WHEN** a new posting from an eligible provider is ingested and indexed for
  the first time
- **THEN** it is marked `auto_apply_available` with no extra backfill step

#### Scenario: An eligible posting's tag survives an ordinary content update

- **WHEN** an already-indexed eligible posting is re-pushed to the index after
  an unrelated content change
- **THEN** it is still marked `auto_apply_available`

### Requirement: The facet is served and filterable

The public read model SHALL serve the signal as `auto_apply_available`,
omitted when the posting is not eligible.

`GET /api/v1/jobs` SHALL accept an optional `auto_apply_available` query
parameter. `auto_apply_available=true` SHALL return only postings marked
eligible. Omitting the parameter SHALL behave exactly as today.

The Meilisearch index SHALL declare a matching filterable attribute, and the
facet SHALL be included in the `/jobs/facets` distribution.

#### Scenario: Requesting only eligible postings

- **WHEN** a caller requests `GET /api/v1/jobs?auto_apply_available=true`
- **THEN** every result is marked `auto_apply_available`

#### Scenario: Omitting the filter changes nothing

- **WHEN** a caller requests `GET /api/v1/jobs` with no
  `auto_apply_available` parameter
- **THEN** the results include both eligible and non-eligible postings

#### Scenario: The facet appears in the distribution

- **WHEN** a caller requests the job facet distribution
- **THEN** it includes a count for `auto_apply_available`

### Requirement: The filterable attribute reaches the live index before the binary requesting it

The Meilisearch settings patch declaring `auto_apply_available` filterable
SHALL be applied to the live index BEFORE the binary that requests the facet
is deployed, the same rollout order every filterable attribute in this
codebase already follows.

#### Scenario: Settings precede the binary

- **WHEN** the change is rolled out
- **THEN** the index settings declare the attribute before the new binary
  serves traffic, and `/api/v1/jobs/facets` never 500s during the rollout

### Requirement: A full reindex reaches the pre-existing catalogue

Because the incremental index push only re-sends documents whose
`content_hash` moved, and a pre-existing posting's `source` triggers no such
change, a full Meilisearch rebuild SHALL follow the settings-and-binary
rollout so pre-existing open postings from eligible providers carry the facet
too — the same trap `is_tech`/`requires_clearance` already document, without
needing a Postgres backfill process since nothing is stored there.

#### Scenario: Pre-existing eligible postings reach the index after rollout

- **WHEN** the settings and binary have rolled out and a full reindex has run
- **THEN** filtering `auto_apply_available=true` returns pre-existing eligible
  postings, not only ones ingested since the deploy
