## MODIFIED Requirements

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
