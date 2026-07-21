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
determinable from the input SHALL be omitted, not guessed. The provider SHALL instruct
the LLM that salary amounts are whole units of the currency: a fractional rate written
with cents (e.g. an hourly `$26.08`) MUST be rounded to the nearest whole unit (`26`),
and the decimal point MUST NEVER be stripped (`26.08` MUST NOT become `2608`).

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
