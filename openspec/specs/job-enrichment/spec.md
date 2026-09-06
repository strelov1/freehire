# job-enrichment

## Purpose

Capture AI-derived, additive metadata for each job — seniority, work mode,
skills, salary, location descriptors, and company descriptors — in a typed,
versioned enrichment payload, so jobs can be filtered and presented richly
without altering the raw source fields ingested by parsers.
## Requirements
### Requirement: Jobs store an additive enrichment payload

The system SHALL store a structured enrichment payload per job in a
`jobs.enrichment` JSONB column that defaults to an empty object. Enrichment SHALL
be additive: writing it SHALL NOT modify any raw source field (`title`,
`company`, `location`, `remote`, `description`, `posted_at`, `company_slug`).

#### Scenario: New job defaults to an empty payload

- **WHEN** a job is upserted without an enrichment payload
- **THEN** its `enrichment` reads back as an empty object (`{}`) and its raw
  fields are stored unchanged

#### Scenario: Enrichment is stored without altering raw fields

- **WHEN** a job is upserted with an enrichment payload
- **THEN** the payload is persisted under `enrichment` and the job's raw fields
  remain exactly as supplied

### Requirement: Enrichment fields follow a typed contract with controlled vocabularies

The system SHALL define the enrichment payload as a single typed Go contract in
`internal/ai/enrich` whose fields and allowed values are the schema's source of
truth. Every field SHALL be optional and omitted when not determined. Enum
fields SHALL accept only their defined vocabulary values; `skills`, `cities`,
`countries`, and `regions` SHALL be arrays; `skills` values SHALL be normalized
lowercase tokens. The contract SHALL provide validation of a payload against the
vocabularies.

The contract SHALL capture a job's geographic area in a single `regions` field —
an enum array of codes drawn from one controlled vocabulary that mixes levels:
`global` (open anywhere), macro-regions (`eu`, `emea`, `eea`, `uk`, `americas`,
`north_america`, `latam`, `apac`, `mena`, `africa`), and select countries treated
as area codes (e.g. `us`, `ru`). `regions` denotes the geographic area of the job
and is meaningful for any `work_mode` (for a remote role its reach, for an onsite
role its office area); the prior restriction to remote roles is removed. There
SHALL be no separate scope discriminator field: an absent/empty `regions` means
*unknown*, and `global` is an explicit value (never inferred from the absence of
other codes), so open-anywhere is distinct from unknown. Validation SHALL check
each `regions` element against the vocabulary. The enrichment-derived `regions`,
`countries`, and `work_mode` are an *additive* source: at read time they fold into
the top-level job geography facet (see the job-geography capability) — geography by
union, `work_mode` by precedence (the LLM value winning over the ingest one) —
rather than being served as independent enrichment fields.

#### Scenario: Payload round-trips through the typed contract

- **WHEN** an `Enrichment` value (e.g. `seniority=senior`, `work_mode=remote`,
  `regions=[eu]`, `skills=[go, postgresql]`) is marshalled to JSON, stored, read
  back, and unmarshalled
- **THEN** the resulting value equals the original

#### Scenario: Undetermined fields are omitted, not zero-filled

- **WHEN** an enrichment payload does not determine salary
- **THEN** the `salary_min`, `salary_max`, `salary_currency`, and
  `salary_period` keys are absent from the stored JSON rather than present with
  zero/empty values

#### Scenario: A value outside a vocabulary is reported invalid

- **WHEN** the contract validates a payload whose `seniority` is `"sr"` (not a
  defined value)
- **THEN** validation reports the payload as invalid, identifying the offending
  field

#### Scenario: An out-of-vocabulary region is reported invalid

- **WHEN** the contract validates a payload whose `regions` contains `"europe"`
  (not a defined value)
- **THEN** validation reports the payload as invalid, identifying the offending
  `regions` field

#### Scenario: Global reach is distinct from unknown reach

- **WHEN** one job's enrichment has `regions=[global]`, and another's has empty
  `regions`
- **THEN** the two payloads are distinguishable: the first denotes open-anywhere,
  the second denotes unknown

### Requirement: Enrichment provenance is tracked per job

The system SHALL track enrichment provenance with two job columns: `enriched_at`
(nullable timestamp) and `enrichment_version` (integer, default 0). A job that
has never been enriched SHALL have `enriched_at` null and `enrichment_version` 0,
so un-enriched rows are identifiable for later processing.

#### Scenario: Un-enriched job is identifiable

- **WHEN** a job has never been enriched
- **THEN** its `enriched_at` is null and its `enrichment_version` is 0

#### Scenario: Provenance reflects a completed enrichment

- **WHEN** a job is written with an enrichment payload produced at schema
  version N
- **THEN** its `enriched_at` is set and its `enrichment_version` equals N

### Requirement: Company descriptors are captured as job enrichment fields

The system SHALL capture company descriptors (`company_type`, `company_size`) as
fields of the job's enrichment payload, not as columns on the `companies` table.
Writing them SHALL NOT alter any `companies` row.

#### Scenario: Company descriptors live in the job payload

- **WHEN** a job is upserted with enrichment including `company_type=product`
- **THEN** the value is stored in that job's `enrichment` and no `companies` row
  is created or modified by it

### Requirement: The jobs read API exposes enrichment and provenance

The system SHALL include `enrichment`, `enriched_at`, and `enrichment_version` in
the job objects returned by the jobs read endpoints (`GET /api/v1/jobs`,
`GET /api/v1/jobs/:id`, and jobs nested under a company). The public job object
SHALL expose geography as top-level `regions` and `countries` fields (the union of
the parsed-location columns and the enrichment-derived values) and `work_mode` as
a top-level field (the LLM value when present, else the ingest-derived one); these
fields SHALL NOT additionally appear as independent fields under `enrichment`. The
public job object SHALL NOT include the raw `remote` boolean: the public notion of
"remote" is expressed solely through the top-level `work_mode` (and the top-level
geography for area), which subsume it. The `jobs.remote` column itself SHALL be
retained as an internal enrichment input and SHALL NOT be removed.

#### Scenario: Job detail includes enrichment and provenance

- **WHEN** a client requests `GET /api/v1/jobs/:id` for an existing job
- **THEN** the returned object under `data` includes `enrichment`,
  `enriched_at`, and `enrichment_version` alongside the existing fields

#### Scenario: Empty enrichment serializes as an object

- **WHEN** a job that has not been enriched is returned by a read endpoint
- **THEN** its `enrichment` is serialized as an empty object (`{}`), not null

#### Scenario: Geography and work mode are served top-level, not duplicated under enrichment

- **WHEN** a client reads a job whose enrichment contained
  `regions`/`countries`/`work_mode`
- **THEN** the returned object carries top-level `regions`/`countries`/`work_mode`
  and its `enrichment` object does not separately repeat those fields

#### Scenario: The raw remote flag is absent from the public job object

- **WHEN** a client requests any jobs read endpoint
- **THEN** the returned job objects do not contain a top-level `remote` field

### Requirement: Enrichment captures the posting's own stated requirements

The enrichment contract SHALL include a `requirements` field: an array of
objects, each with a `text` (the requirement as stated) and a `priority` of
either `required` or `preferred`. This list SHALL be job-only — derived
solely from the posting, with no comparison against any candidate — and is
distinct from any candidate-comparison requirement list produced elsewhere in
the system. The list SHALL be bounded to a maximum number of entries and each
`text` bounded to a maximum length, consistent with how other free-text
enrichment fields (e.g. `cities`) are bounded. A `priority` value outside
`required`/`preferred` SHALL be coerced into that vocabulary rather than
rejected, and an entry whose `text` is empty after bounding SHALL be dropped.

#### Scenario: A posting's requirements round-trip through the typed contract

- **WHEN** an `Enrichment` value with `requirements=[{text: "5+ years Go", priority: required}, {text: "Kubernetes", priority: preferred}]` is marshalled to JSON, stored, read back, and unmarshalled
- **THEN** the resulting value equals the original

#### Scenario: An oversized requirements list is bounded

- **WHEN** an enrichment payload's `requirements` list exceeds the maximum entry count, or an entry's `text` exceeds the maximum length
- **THEN** sanitizing the payload truncates the list to the maximum entry count and clips each `text` to the maximum length

#### Scenario: An unrecognized priority is coerced, not rejected

- **WHEN** a `requirements` entry's `priority` is not exactly `required` (e.g. a different casing, surrounding whitespace, or an unrelated value)
- **THEN** sanitizing the payload coerces that entry's `priority` to `required` when it case/whitespace-insensitively matches `required`, and to `preferred` otherwise

#### Scenario: No stated requirements yields an empty list, not a guess

- **WHEN** a job description states no explicit requirements
- **THEN** the returned `Enrichment` has an empty (or absent) `requirements` list rather than a fabricated one

### Requirement: The jobs read API exposes the posting's stated requirements

The `requirements` field SHALL be served as part of a job's `enrichment`
payload in the jobs read endpoints (`GET /api/v1/jobs`, `GET /api/v1/jobs/:id`,
and jobs nested under a company), the same way every other served enrichment
field is — with no suppression.

#### Scenario: Job detail includes the posting's requirements

- **WHEN** a client requests `GET /api/v1/jobs/:id` for a job whose enrichment includes a non-empty `requirements` list
- **THEN** the returned object's `enrichment.requirements` matches the stored list

### Requirement: The derived requirements fill the served field when the model states none

The served `enrichment.requirements` SHALL be filled from a job's stored,
deterministically derived requirements whenever the enrichment payload being
written states none of its own. The model's reading wins when it has one: it
reads the postings whose requirements are prose with no list markup, which the
derivation cannot reach, so the two sources add coverage rather than compete.

The fold SHALL happen on the READ path, where the projection assembles the served
payload, and the derived list SHALL NOT be copied into the stored enrichment blob.
Two attempts to do it at write time both failed, in ways worth stating as
requirements of their own:

- A write-time overlay runs only when the model runs, so it fills nothing for the
  postings the model has never reached — which are the majority, and the whole
  reason the derivation exists.
- A copy in the blob is a second stored value that nothing revises. A later crawl
  rewrites the column and leaves the copy, so a description edit that removed the
  requirements section would leave a consumer reading a list the posting no longer
  states, out of reach of any backfill.

Reading the column on every projection satisfies both: the served field always
reflects the current column, and a consumer still reads exactly one field.

A later enrichment run therefore SHALL NOT be able to erase a derived list — the
run writes only what the model said, and the projection re-reads the column.

The derivation SHALL NOT change a job's enrichment version or provenance stamp:
it is orthogonal to the model payload, and a job that has never been enriched
stays that way while still serving derived requirements.

#### Scenario: A model payload with requirements is left alone

- **WHEN** a job whose stored enrichment states a non-empty `requirements` list also has stored derived requirements
- **THEN** the job's served `enrichment.requirements` is the payload's list, unchanged

#### Scenario: A model payload without requirements picks up the derivation

- **WHEN** a job whose stored enrichment states no `requirements` has non-empty stored derived requirements
- **THEN** the job's served `enrichment.requirements` is the derived list

#### Scenario: Neither source yields a list

- **WHEN** a job whose stored enrichment states no `requirements` also has empty stored derived requirements
- **THEN** the job's served `enrichment.requirements` is empty or absent

#### Scenario: An unenriched job still serves derived requirements

- **WHEN** a client requests a job that has never been enriched but whose stored derived requirements are non-empty
- **THEN** the returned object's `enrichment.requirements` holds the derived list and the job's enrichment provenance still reports it as unenriched

#### Scenario: A later enrichment run cannot erase a derived list

- **WHEN** a job serving derived requirements is enriched by a run whose payload states no requirements of its own
- **THEN** the job still serves the derived list afterwards

#### Scenario: The derived list is not copied into the stored enrichment

- **WHEN** an enrichment payload stating no `requirements` is written for a job whose stored derived requirements are non-empty
- **THEN** the job's stored enrichment still states no `requirements`, and the derived column is unchanged
