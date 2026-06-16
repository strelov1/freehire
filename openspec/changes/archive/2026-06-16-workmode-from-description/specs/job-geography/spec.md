## MODIFIED Requirements

### Requirement: Work mode is resolved by precedence across sources

`work_mode` is a scalar, so it SHALL be resolved by precedence, not union. It is
derived at ingest into `jobs.work_mode` from three sources, most authoritative
first: (1) the adapter's STRUCTURED work mode (a workplace-type enum or explicit
remote flag from the ATS), (2) a marker in the parsed **location** string, and
(3) a conservative phrase match in the job **description**. A lower source fills
`work_mode` only when every higher source left it empty; the parser never guesses,
so a description with no clear work-arrangement phrase yields nothing. At read time
the served `work_mode` SHALL be the stored `jobs.work_mode` only; the LLM-derived
`enrichment.work_mode` SHALL NOT override it.

#### Scenario: Structured adapter work mode beats the parser

- **WHEN** an adapter reports a structured `work_mode=hybrid` for a posting whose
  location text would parse as `remote`
- **THEN** the stored `jobs.work_mode` is `hybrid`

#### Scenario: The location marker beats the description

- **WHEN** a job has no structured work mode, a location that parses to `remote`,
  and a description that mentions a hybrid arrangement
- **THEN** the derived `work_mode` is `remote` (location wins; description only fills)

#### Scenario: The description fills when location is silent

- **WHEN** a job has no structured work mode, a location with no work-mode marker
  (e.g. a bare city), and a description stating "this is a fully remote position"
- **THEN** the derived `work_mode` is `remote`

#### Scenario: A noisy description token does not trigger a false positive

- **WHEN** a job's description contains incidental tokens like "distributed
  systems" or "hybrid cloud" but no actual work-arrangement phrase, and no
  structured or location signal
- **THEN** the derived `work_mode` is empty

#### Scenario: The ingest value is served regardless of the LLM

- **WHEN** a job has `jobs.work_mode=onsite` from ingest and
  `enrichment.work_mode=remote` from the LLM, and is read
- **THEN** the resolved top-level `work_mode` is `onsite`
