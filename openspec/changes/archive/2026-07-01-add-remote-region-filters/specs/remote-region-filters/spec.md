## ADDED Requirements

### Requirement: The remote_unspecified facet is derived deterministically

The system SHALL derive a boolean facet `remote_unspecified` for each job from
its already-derived geography facets, with no LLM involvement. The facet SHALL be
`true` when, and only when, the derived `work_mode` is `remote` AND the derived
`countries` set is empty AND the derived `regions` set is empty; otherwise it
SHALL be `false`. The facet SHALL be computed by `jobderive.Derive` from the same
inputs that produce the geography facets, so the ingest pipeline and the moderator
write path produce identical results, and a re-derive is idempotent.

#### Scenario: A bare remote job is flagged

- **WHEN** a job's geography derives to `work_mode=remote`, empty countries, and
  empty regions (e.g. location `Remote`)
- **THEN** `remote_unspecified` is `true`

#### Scenario: A remote job with a resolved region is not flagged

- **WHEN** a job derives to `work_mode=remote` with a non-empty region (e.g.
  location `Remote - Europe` → `[eu]`, or `Remote - Anywhere` → `[global]`)
- **THEN** `remote_unspecified` is `false`

#### Scenario: A non-remote job with no geography is not flagged

- **WHEN** a job derives to an empty `work_mode` (or `hybrid`/`onsite`) with empty
  countries and regions
- **THEN** `remote_unspecified` is `false`

### Requirement: The remote_unspecified facet is stored, served, and indexed

The system SHALL persist `remote_unspecified` as a `jobs` table column written on
every upsert and re-derive, alongside the other deterministic facets and NOT
inside the `enrichment` JSONB. The public read model (`jobview`) SHALL serve the
column value. The search index SHALL register `remote_unspecified` as a
filterable attribute, sourced from the served read model, so existing rows reflect
the facet only after a re-derive and reindex.

#### Scenario: The facet is served from the jobs column

- **WHEN** a job with `remote_unspecified=true` is read through the public model
- **THEN** the served object reports the facet as `true`

#### Scenario: The facet is a filterable search attribute

- **WHEN** the search index is configured
- **THEN** `remote_unspecified` is among the index's filterable attributes

### Requirement: Jobs can be filtered by remote_unspecified

The search API SHALL accept a `remote_unspecified` boolean filter param that,
when set to `true`, restricts results to jobs whose `remote_unspecified` facet is
`true`. The param SHALL be built by the same pure filter builder shared by the
HTTP search handler and the saved-search/notification matcher, so both produce an
identical filter. An unset or empty param SHALL emit no filter fragment.

#### Scenario: Filtering by remote_unspecified narrows results

- **WHEN** a search request sets `remote_unspecified=true`
- **THEN** only jobs whose `remote_unspecified` facet is `true` are returned

#### Scenario: An unset param emits no filter

- **WHEN** a search request omits `remote_unspecified` (or passes it empty)
- **THEN** no `remote_unspecified` fragment is added to the search filter

### Requirement: The SPA exposes remote_unspecified as a sidebar toggle

The web frontend SHALL present `remote_unspecified` as a boolean toggle control in
the jobs filter sidebar (mirroring the existing boolean filters such as visa
sponsorship), labelled to convey location-flexible remote work (no specific country
or region) and kept distinct from the `Global` region option. Enabling it SHALL
drive the `remote_unspecified` search param; the filter model SHALL serialize the
enabled state to `remote_unspecified=true`, parse it back from the URL, and count it
as one active filter.

#### Scenario: The toggle is shown and drives the param

- **WHEN** the user opens the jobs filter sidebar
- **THEN** a location-flexible remote toggle appears, distinct from the `Global`
  region option, and enabling it sets the `remote_unspecified` search param to
  `true`
