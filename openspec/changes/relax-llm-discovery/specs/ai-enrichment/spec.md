## ADDED Requirements

### Requirement: Unserved discovery facets are captured raw, not validated

Because the six dictionary-covered facets (`countries`, `regions`, `work_mode`, `skills`, `seniority`, `category`) are served from the deterministic dictionaries only (dict-only) and the enrichment copies of them are not served, the enrichment worker SHALL capture the LLM's values for those facets **raw**. `Sanitize` SHALL NOT blank or filter an out-of-vocabulary `work_mode`, `seniority`, `category`, or `regions`, and `Validate` SHALL NOT reject the payload for an out-of-vocabulary value in those facets. The served enum fields (`employment_type`, `relocation`, `salary_period`, `english_level`, `education_level`, `company_type`, `company_size`, `domains`) SHALL still be sanitized and validated, and salary clamping is unchanged. The prompt SHALL permit a concise lowercase label of the model's own for the discovery facets when no allowed value fits, while keeping the strict instruction for the served fields. This applies going forward only — `enrich.Version` MUST NOT be bumped and existing payloads MUST NOT be re-enriched.

#### Scenario: An out-of-vocabulary discovery value is persisted, not dropped

- **WHEN** the LLM returns `category="ml_platform"` (not a defined value) for a job
- **THEN** `Sanitize` keeps it, `Validate` passes, and `ml_platform` is written to the job's `enrichment` JSONB

#### Scenario: An out-of-vocabulary served value is still rejected

- **WHEN** the LLM returns `employment_type="seasonal"` (not a defined value)
- **THEN** `Sanitize` blanks it, so no out-of-vocabulary value reaches the served `employment_type`

#### Scenario: The discovery values do not reach the served object

- **WHEN** a job's `enrichment` carries a raw `seniority="staff_plus"` discovery value
- **THEN** the served job object's `seniority` is the dictionary value (or empty), never the raw LLM discovery value
