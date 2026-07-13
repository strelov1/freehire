# ai-enrichment

## MODIFIED Requirements

### Requirement: Enrichment is extracted from a job's description by an LLM provider

The system SHALL define a `Provider` abstraction in `internal/enrich` that, given a
job's source fields (at minimum `title`, `company`, `location`, `remote`,
`description`), returns a populated `Enrichment` value. The provider SHALL instruct
the LLM with the controlled vocabularies from the phase-1 contract so that the enum
fields it is asked for are constrained to their allowed values. The provider SHALL
NOT ask the LLM for the dictionary-covered facets that the read layer serves from the
deterministic dictionaries (see "Unserved discovery facets are captured raw, not
validated"); those are derived by `internal/jobderive`, not the LLM. Fields not
determinable from the input SHALL be omitted, not guessed.

#### Scenario: Description fields are mapped into the contract

- **WHEN** the provider is given a job whose description states "Senior Go engineer,
  fully remote, €70k–90k/year"
- **THEN** it returns an `Enrichment` with `salary_min=70000`, `salary_max=90000`,
  `salary_currency=EUR`, and `salary_period=year`
- **AND** it does not populate `seniority`, `work_mode`, or `skills` from the LLM —
  those are derived by the deterministic dictionaries, not requested in the prompt

#### Scenario: Unstated fields are omitted

- **WHEN** a job description says nothing about visa sponsorship or company size
- **THEN** the returned `Enrichment` leaves `visa_sponsorship`, `company_size`, and
  every other unstated field absent rather than filled with a guess

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
