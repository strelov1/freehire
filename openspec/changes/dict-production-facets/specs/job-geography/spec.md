## MODIFIED Requirements

### Requirement: Work mode is resolved by precedence across sources

`work_mode` is a scalar, so it SHALL be resolved by precedence, not union. At
ingest the adapter's STRUCTURED work mode (a workplace-type enum or explicit
remote flag from the ATS) SHALL take precedence over the parser's free-text
heuristic, and the result SHALL be stored in `jobs.work_mode`. At read time the
served `work_mode` SHALL be the stored `jobs.work_mode` only; the LLM-derived
`enrichment.work_mode` SHALL NOT override it. The net order, most authoritative
first, is adapter-structured, then parsed location.

#### Scenario: Structured adapter work mode beats the parser

- **WHEN** an adapter reports a structured `work_mode=hybrid` for a posting whose
  location text would parse as `remote`
- **THEN** the stored `jobs.work_mode` is `hybrid`

#### Scenario: The ingest value is served regardless of the LLM

- **WHEN** a job has `jobs.work_mode=onsite` from ingest and
  `enrichment.work_mode=remote` from the LLM, and is read
- **THEN** the resolved top-level `work_mode` is `onsite`

#### Scenario: An empty ingest work mode stays empty even when the LLM has a value

- **WHEN** a job has an empty `jobs.work_mode` and `enrichment.work_mode=remote`,
  and is read
- **THEN** the resolved top-level `work_mode` is empty

### Requirement: The public job object exposes geography and work mode as a top-level facet

The public job object SHALL expose geography as top-level `regions` and
`countries` fields carrying the deterministic (jobs-column) values, and
`work_mode` as a top-level field carrying the deterministic value, each reported
exactly once. The `enrichment.regions`, `enrichment.countries`, and
`enrichment.work_mode` fields SHALL NOT additionally appear as independent fields
in the served object. The stored `enrichment` JSONB SHALL be left untouched (the
enrichment worker's data is preserved for future discovery use).

#### Scenario: Geography and work mode appear once, at the top level

- **WHEN** a client reads a job whose enrichment contained `regions` and `work_mode`
- **THEN** the returned object carries top-level `regions`/`countries`/`work_mode`
  from the jobs columns and does not separately repeat those fields under
  `enrichment`

## REMOVED Requirements

### Requirement: Job geography is stored on jobs and unioned with enrichment geography at read time

**Reason**: The read-time union of parsed geography with the LLM-derived
`enrichment.regions`/`enrichment.countries` is replaced by dict-only sourcing.
The "store parsed geography in jobs columns as source facts" fact and the new
read-model behavior are restated in the `deterministic-facets` capability, which
owns how all six dictionary-derived facets are sourced into the wire shape.

**Migration**: Geography is now served from the `jobs.countries`/`jobs.regions`
columns only; the LLM's geography values are no longer merged into the served
facet (they remain in the `enrichment` JSONB). Re-derive existing rows with
`cmd/backfill-derive` and run a `reindex` so the search index reflects the new
sourcing.

### Requirement: Existing jobs are backfilled with parsed geography

**Reason**: The geography-only backfill is superseded by the single unified
`backfill-derive` pass (defined in `deterministic-facets`) that re-derives all six
dictionary facet columns — geography, skills, seniority, and category — in one
pass, replacing the three separate per-facet backfill commands.

**Migration**: Run `cmd/backfill-derive` (replacing `cmd/backfill-geo`), then a
single `reindex`.
