## MODIFIED Requirements

### Requirement: A Job is constructed only through the aggregate factory

The system SHALL expose a single construction path for a `Job` domain value — a
factory (`job.New`) taking a source-agnostic draft of the raw posting fields — and
SHALL NOT allow callers to assemble a `Job` from a raw composite literal outside the
`job` package. The factory SHALL run the deterministic derivation (`jobderive`,
wrapping `location`/`skilltag`/`classify`/`roletag` and the public-identity slugs)
internally, so a constructed `Job` always carries facets consistent with its source
fields. The draft MAY carry explicit values for the `regions`, `cities`, `work_mode`,
and `skills` facets; when present, an explicit value SHALL win over the dictionary
derivation for that facet, and when absent, derivation SHALL fill it. Every write path
that persists a posting — automated ingest, moderator authoring, and Telegram
extraction — SHALL obtain its `Job` from this factory.

#### Scenario: Every write path derives facets identically

- **WHEN** the ingest pipeline, the moderator authoring path, and Telegram
  extraction each construct a job from the same title, description, and location
- **THEN** all three produce the same dictionary facets (`countries`, `regions`,
  `work_mode`, `skills`, `seniority`, `category`) and the same public/company slugs,
  because each obtains its `Job` through `job.New`

#### Scenario: Facets cannot be omitted

- **WHEN** a caller constructs a `Job` through the factory and never touches the
  facet fields
- **THEN** the resulting `Job` still carries the derived facets, because derivation
  happens inside `job.New` rather than in caller code

#### Scenario: Explicit facet values win over derivation

- **WHEN** a caller constructs a `Job` whose draft supplies explicit `regions`,
  `cities`, `work_mode`, or `skills`
- **THEN** the resulting `Job` carries those explicit values for those facets instead
  of the values the dictionaries would derive, while any unsupplied facet is still
  derived

## ADDED Requirements

### Requirement: A manual salary override takes precedence over enriched salary

The system SHALL let a job carry an authoritative manual salary (min, max, currency,
period) independent of the LLM-enriched salary. When a job has a manual salary, the
effective salary the system exposes (search facets, filters, insights, the public wire
shape) SHALL be the manual salary, and an enrichment pass SHALL NOT displace it: applying
enrichment to a job that has a manual salary MUST preserve that manual salary as the
effective salary, even if the enrichment payload proposes a different figure.

#### Scenario: Enrichment does not displace a manual salary

- **WHEN** a job with a manual salary is processed by an enrichment pass whose payload proposes a different salary
- **THEN** the job's effective salary remains the manual salary after the pass

#### Scenario: Jobs without a manual salary are unaffected

- **WHEN** a job that has no manual salary is processed by an enrichment pass
- **THEN** the job's effective salary is the enriched salary, exactly as before
