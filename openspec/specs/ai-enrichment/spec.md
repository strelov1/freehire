# ai-enrichment

## Purpose

Drive the production of job enrichment payloads: track which jobs need enriching
in a durable outbox queue, extract enrichment from each job's description via a
provider-agnostic LLM, and write validated results back with provenance — safely
under concurrency, with retries and dead-lettering — run by a standalone batch
command.
## Requirements
### Requirement: Jobs needing enrichment are tracked in a durable outbox queue

The system SHALL maintain an `enrichment_outbox` table holding one entry per
`(job_id, target_version)` that needs enriching. The entry SHALL reference the job by
id and SHALL NOT duplicate the job's source fields. The system SHALL provide an
idempotent enqueue that adds entries for **open** jobs (`closed_at IS NULL`) whose
`enriched_at IS NULL` or whose `enrichment_version` is below the current schema version
(`enrich.Version`); a closed job SHALL NOT be enqueued, and re-enqueuing an
already-queued `(job_id, target_version)` SHALL NOT create a duplicate.

The ingest write path SHALL additionally enqueue a job into the outbox in the **same
transaction** as the job's upsert, gated on the same condition (`enriched_at IS NULL`
or `enrichment_version` below the current version), so that a newly ingested job is
queued for enrichment atomically with its write while an already-enriched job is not
re-queued.

#### Scenario: Pending jobs are enqueued

- **WHEN** the enqueue runs and an open job has `enriched_at = NULL`
- **THEN** an outbox entry for that job at the current `target_version` exists

#### Scenario: Stale-version jobs are enqueued

- **WHEN** an open job's `enrichment_version` is below the current `enrich.Version`
- **THEN** an outbox entry for that job at the current `target_version` exists

#### Scenario: Closed jobs are not enqueued

- **WHEN** the enqueue runs and a job has `closed_at IS NOT NULL` and `enriched_at = NULL`
- **THEN** no outbox entry is created for that job

#### Scenario: Enqueue is idempotent

- **WHEN** the enqueue runs twice without the job being enriched in between
- **THEN** the job has exactly one outbox entry for that `target_version`

#### Scenario: Ingest enqueues a new job transactionally

- **WHEN** the ingest write path upserts a job whose `enriched_at IS NULL`
- **THEN** an outbox entry for that job at the current `target_version` is created in
  the same transaction as the upsert

#### Scenario: Ingest does not re-queue an already-enriched job

- **WHEN** the ingest write path re-ingests a job already enriched to the current
  `enrich.Version`
- **THEN** no new outbox entry is created for that job

### Requirement: Enrichment is extracted from a job's description by an LLM provider

The system SHALL define a `Provider` abstraction in `internal/enrich` that, given a
job's source fields (at minimum `title`, `company`, `location`, `remote`,
`description`), returns a populated `Enrichment` value. The provider SHALL constrain
the **served** enum fields to their allowed values by carrying the controlled
vocabularies in the **request schema**, so a value outside a vocabulary cannot be
generated rather than merely being discarded after the fact; those vocabularies SHALL
continue to be validated on receipt, because a provider that stops honouring the
schema reports no error. The **discovery** facets `regions` and `countries` SHALL NOT
be constrained by the schema: the prompt permits a label of the model's own when no
allowed value fits, and an enum would foreclose the novel value that mechanism exists
to collect (see "Unserved discovery facets are captured raw, not validated"). The
request schema SHALL describe exactly the fields the prompt asks for and no others,
so the dictionary-covered facets served by `internal/jobderive` are absent from both;
under a strict schema a field left in is a field the model is required to produce.
Fields not determinable from the input SHALL be returned as `null`, not guessed — a
strict request schema admits no absent key — and a `null` SHALL be indistinguishable
downstream from the omitted field it replaces. The provider SHALL instruct the LLM
that salary amounts are whole units of the currency: a fractional rate written with
cents (e.g. an hourly `$26.08`) MUST be rounded to the nearest whole unit (`26`), and
the decimal point MUST NEVER be stripped (`26.08` MUST NOT become `2608`).

#### Scenario: Description fields are mapped into the contract

- **WHEN** the provider is given a job whose description states "Senior Go engineer,
  fully remote, €70k–90k/year"
- **THEN** it returns an `Enrichment` with `salary_min=70000`, `salary_max=90000`,
  `salary_currency=EUR`, and `salary_period=year`
- **AND** it does not populate `seniority`, `work_mode`, or `skills` from the LLM —
  those are derived by the deterministic dictionaries, not requested in the prompt

#### Scenario: A fractional hourly rate is rounded, not decimal-stripped

- **WHEN** the provider is given a job whose description states an hourly base pay
  range of "$26.08—$38.40 USD"
- **THEN** the prompt instructs the model to round each figure to a whole currency
  unit, so the returned `Enrichment` has `salary_min=26`, `salary_max=38`,
  `salary_currency=USD`, and `salary_period=hour`
- **AND** it never returns `salary_min=2608` (the decimal point is not stripped)

#### Scenario: Unstated fields are omitted

- **WHEN** a job description says nothing about visa sponsorship or company size
- **THEN** the returned `Enrichment` leaves `visa_sponsorship`, `company_size`, and
  every other unstated field absent rather than filled with a guess

#### Scenario: An unstated field arrives as an explicit null

- **WHEN** the model, bound by a strict schema that lists every field as required,
  reports `null` for a field the description does not state
- **THEN** the resulting `Enrichment` treats that field exactly as it treated an
  absent key before — unset, not written back, and not stamped as provenance

#### Scenario: A served enum value outside its vocabulary cannot be produced

- **WHEN** a job description describes a company size the vocabulary has no value for
- **THEN** the request schema restricts the field to the vocabulary, so the model
  returns one of the allowed values or `null`, and never invents a new one

#### Scenario: A discovery facet may still carry a label of the model's own

- **WHEN** a posting states a geographic reach no region value fits
- **THEN** the schema leaves `regions` and `countries` unconstrained, so the model's
  own concise label is returned and captured raw, exactly as before this change

#### Scenario: A provider that ignores the schema is still caught

- **WHEN** a response carries an enum value outside its vocabulary despite the schema
- **THEN** the existing validation drops that field as it does today, leaving the
  rest of the enrichment intact

### Requirement: The LLM endpoint is configured provider-agnostically

The system SHALL configure the enrichment LLM from three provider-neutral settings:
`LLM_BASE_URL` (an OpenAI-compatible API endpoint — e.g. a LiteLLM gateway or a
Chinese model provider), `LLM_API_KEY` (the credential), and `LLM_MODEL` (the model
id). No provider name, vendor-specific key, or default model SHALL be hard-coded.
The enrichment command SHALL fail with a clear error when any of the three is unset.

#### Scenario: Endpoint and model come from config

- **WHEN** `LLM_BASE_URL`, `LLM_API_KEY`, and `LLM_MODEL` are set
- **THEN** the provider calls that endpoint with that model, with no provider name
  baked into the code

#### Scenario: Switching provider needs no code change

- **WHEN** `LLM_BASE_URL` and `LLM_MODEL` are changed to a different OpenAI-compatible
  provider
- **THEN** the enrichment run targets the new provider without a code change or
  rebuild

#### Scenario: Missing configuration fails fast

- **WHEN** any of `LLM_BASE_URL`, `LLM_API_KEY`, or `LLM_MODEL` is unset
- **THEN** the enrichment command exits with an error naming the missing setting and
  enriches no jobs

### Requirement: Queue entries are claimed safely under concurrency

The system SHALL claim a bounded batch of outbox entries that are not dead-lettered,
not currently leased, and whose job is **open** (`closed_at IS NULL`), using
`FOR UPDATE SKIP LOCKED`, stamping `claimed_at` on each claimed entry. The claim SHALL
order candidates by job freshness — `COALESCE(posted_at, created_at) DESC, id DESC` —
so the newest open postings are enriched first; a job without a source post date SHALL
rank by its ingest time (`created_at`) rather than always sorting last, so undated jobs
are not starved while dated jobs keep arriving. Concurrent claimers SHALL receive
disjoint entries. An entry whose `claimed_at` is older than the lease duration SHALL
become claimable again, so a crashed or stalled worker's entries are reclaimed without a
separate process. An outbox entry whose job has since been closed SHALL NOT be claimed.

#### Scenario: Concurrent workers get disjoint entries

- **WHEN** two enrichment runs claim a batch at the same time
- **THEN** no outbox entry is handed to both runs

#### Scenario: Fresher open jobs are claimed first

- **WHEN** the outbox holds entries for two open jobs with different `posted_at`
- **THEN** a claim returns the entry for the job with the later `posted_at` before the
  one with the earlier `posted_at`

#### Scenario: Undated jobs rank by ingest time, not last

- **WHEN** the outbox holds an entry for an old dated job and one for a recently
  ingested job with no `posted_at`
- **THEN** a claim returns the undated-but-recent job's entry before the old dated one

#### Scenario: Entries for closed jobs are not claimed

- **WHEN** an outbox entry references a job with `closed_at IS NOT NULL`
- **THEN** it is not returned by a claim

#### Scenario: A stalled claim is reclaimed after the lease

- **WHEN** an entry was claimed but its `claimed_at` is older than the lease duration
- **THEN** a subsequent claim is allowed to pick it up again

#### Scenario: Dead-lettered entries are not claimed

- **WHEN** an entry has been dead-lettered (`failed_at` set)
- **THEN** it is not returned by a claim

### Requirement: Validated write-back stamps provenance and removes the queue entry

When extraction passes `Enrichment.Validate`, the system SHALL, in one transaction,
write the payload to the job's `enrichment` column, set `enriched_at` to the write
time, set `enrichment_version` to the entry's `target_version`, and delete the outbox
entry. The write SHALL NOT modify any raw source field (`title`, `company`,
`location`, `remote`, `description`, `posted_at`, `company_slug`).

#### Scenario: Successful enrichment is written and dequeued

- **WHEN** a claimed job is enriched and the payload validates
- **THEN** the job's `enrichment` is set, `enriched_at` is non-null,
  `enrichment_version` equals the entry's `target_version`, the outbox entry is gone,
  and the job's raw source fields are unchanged

### Requirement: Repeated failures are retried then dead-lettered

An extraction that fails validation SHALL be retried at most once within the same
attempt before the attempt is counted as failed. On a failed attempt the system SHALL
increment the entry's `attempts` and record the error, leaving its lease in place so
the entry is retried on a later run after the lease expires (never reprocessed within
the same run); no invalid payload SHALL ever be written to `jobs`.

Whether a failed entry is dead-lettered SHALL depend on whether the posting caused
the failure:

- A failure **the posting caused** — a corrupted row, a model response that cannot be
  parsed for this input, or a payload that fails validation — SHALL be dead-lettered
  once `attempts` reaches the configured maximum.
- **Every other failure**, including any transport, gateway, authentication or timeout
  error, SHALL NOT be dead-lettered on the attempt counter. It SHALL be dead-lettered
  only once the entry has been queued longer than a configured grace window.

The classification SHALL be defined as the set of failures the system itself raises
for a posting, so an unrecognised failure counts as not the posting's fault. An
outage shorter than the grace window SHALL therefore cost no entry permanently,
however many attempts it consumes.

#### Scenario: A transient failure is retried on a later run

- **WHEN** enriching an entry fails once (validation or LLM error) and it is not yet
  eligible for dead-lettering
- **THEN** the job is left unenriched, the entry's `attempts` is incremented, and the
  entry becomes eligible to be claimed again only after its lease expires

#### Scenario: A posting that cannot be enriched is dead-lettered

- **WHEN** an entry fails with an unparseable model response until its attempts reach
  the configured maximum
- **THEN** its `failed_at` is set, it is no longer claimed, and the job's `enrichment`
  was never written with an invalid value

#### Scenario: A gateway outage does not dead-letter an entry

- **WHEN** an entry fails repeatedly with a gateway error, past the attempt maximum,
  while it has been queued for less than the grace window
- **THEN** its `failed_at` remains unset and it stays claimable, so the entry survives
  the outage and is enriched once the gateway recovers

#### Scenario: An entry failing on our side indefinitely still stops

- **WHEN** an entry has failed with a non-posting error and has been queued longer
  than the grace window
- **THEN** its `failed_at` is set, so a permanently unservable entry cannot consume
  the queue forever

#### Scenario: An unrecognised failure is not blamed on the posting

- **WHEN** an entry fails with an error the classifier does not recognise
- **THEN** it is treated as not the posting's fault and bounded by the grace window
  rather than by the attempt counter

### Requirement: A batch command runs the enrichment process

The system SHALL provide a standalone command (`cmd/enrich`) that connects to the
database, enqueues pending jobs, then repeatedly claims a wave of outbox entries and
drains it, enriching and writing back each entry, until no claimable entry remains. The
command SHALL process each claim wave **concurrently** across a configurable number of
workers (`ENRICH_CONCURRENCY`, default 4), and SHALL size each claim wave to the
configured concurrency so that the time an entry stays leased before processing remains
well under the lease duration. The command SHALL report how many entries were enriched,
failed, and dead-lettered. A failure on one entry SHALL NOT abort the run.

#### Scenario: A run reports its outcome

- **WHEN** `cmd/enrich` processes a wave with some enrichable and some failing entries
- **THEN** it writes the enrichable ones, advances the failing ones' attempts, and
  exits reporting the enriched / failed / dead-lettered counts

#### Scenario: A wave is drained concurrently

- **WHEN** a claim wave of multiple entries is drained with concurrency greater than one
- **THEN** entries in the wave are processed in parallel and the reported counts equal
  the sum of each entry's outcome

#### Scenario: One failing entry does not abort the run

- **WHEN** enriching a single entry returns an error (e.g. an LLM call fails)
- **THEN** that entry is recorded as a failed attempt and the run proceeds to the
  remaining entries

### Requirement: Unserved discovery facets are captured raw, not validated

The enrichment prompt SHALL NOT request the dictionary-covered facets `work_mode`, `seniority`, `category`, or `skills`, nor the non-enum dict-derived `posting_language` and `experience_years_min`, nor the dict-covered `employment_type`, `education_level`, and `english_level` — the read layer serves all of these from the deterministic dictionaries (`internal/jobderive`), so the LLM's copies are never served and paying output tokens for them is waste.

The prompt SHALL continue to request `countries`/`regions` as the sole discovery
facets — the dict-then-LLM hybrid where the LLM fills only the unpinned geographic
bucket via `jobview.geoFacet` — and for those two it MAY permit a concise lowercase
label of the model's own when no allowed value fits.

For any discovery value that is present (from the still-requested `countries`/`regions`
facets, or a pre-existing payload), the worker SHALL capture it raw: `Sanitize` SHALL
NOT blank or filter an out-of-vocabulary `work_mode`, `seniority`, `category`, or
`regions`, and `Validate` SHALL NOT reject the payload for an out-of-vocabulary value
in those facets. The served enum fields (`relocation`, `salary_period`,
`company_type`, `company_size`, `domains`) SHALL still be sanitized and validated, and
salary clamping is unchanged. This applies going forward only — `enrich.Version` MUST
NOT be bumped and existing payloads MUST NOT be re-enriched.

#### Scenario: The prompt does not request dict-covered facets

- **WHEN** the enrichment system prompt is built
- **THEN** it contains no request for `work_mode`, `seniority`, `category`, `skills`,
  `posting_language`, `experience_years_min`, `employment_type`, `education_level`, or
  `english_level`
- **AND** a new enrichment payload for a job whose description states "Senior Go
  engineer" leaves those fields absent

#### Scenario: A still-requested discovery value is persisted raw

- **WHEN** the LLM returns `regions=["antarctica"]` (not a defined value) for a job
- **THEN** `Sanitize` keeps it, `Validate` passes, and it is written to the job's
  `enrichment` JSONB

#### Scenario: An out-of-vocabulary served value is still rejected

- **WHEN** the LLM returns `company_type="conglomerate"` (not a defined value)
- **THEN** `Sanitize` blanks it, so no out-of-vocabulary value reaches the served
  `company_type`

#### Scenario: The discovery values do not reach the served object

- **WHEN** a job's `enrichment` carries a raw `seniority="staff_plus"` discovery value
  (e.g. from an older payload)
- **THEN** the served job object's `seniority` is the dictionary value (or empty),
  never the raw LLM discovery value

