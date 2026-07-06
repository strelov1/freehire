## ADDED Requirements

### Requirement: A deterministic dictionary derives a job's roles

The system SHALL provide `internal/roletag`, a curated deterministic dictionary
(mirroring `internal/classify` and `internal/skilltag`) that derives a job's
`roles` — a list of canonical role slugs — from its resolved seniority, resolved
category, and title. It SHALL emit:

- the **bare category role** `{category}` (e.g. `backend`, `data_science`)
  whenever the category resolves (to a category with a natural role noun — all of
  `enrich.CategoryValues` except `other`), regardless of seniority — this is the
  dominant real-world case, since most titles carry no grade;
- the composite role `{seniority}_{category}` (e.g. `senior_backend`) **in
  addition** when the seniority also resolves — the graded role on top of the
  bare one;
- one canonical slug per **named role** whose alias occurs as a whole word in the
  title, for roles that do not decompose into the seniority×category grid
  (e.g. `software engineer` → `software_engineer`, `founding engineer` →
  `founding_engineer`, `fractional cto` → `fractional_cto`). When several named
  aliases match, the longest (most specific) wins and at most one named role is
  emitted.

It SHALL never guess: an input it cannot resolve contributes no slug, and a job
the dictionary resolves nothing for (no category and no named-alias match) yields
an empty `roles`. The three sources occupy distinct slug namespaces, so the
result carries no duplicates, and every emitted slug SHALL exist in the role
catalog.

#### Scenario: Bare category role without a grade

- **WHEN** `roletag` derives roles for a job with an empty seniority, category
  `data_science`, and title "Data Scientist"
- **THEN** the derived `roles` include `data_science` and no composite role

#### Scenario: A grade adds the composite on top of the bare role

- **WHEN** `roletag` derives roles for a job with seniority `senior`, category
  `backend`, and title "Senior Backend Engineer"
- **THEN** the derived `roles` include both `backend` and `senior_backend`

#### Scenario: Named role from the title regardless of the grid

- **WHEN** `roletag` derives roles for a job titled "Founding Engineer" whose
  category is empty
- **THEN** the derived `roles` include `founding_engineer`

#### Scenario: Nothing resolvable yields empty roles

- **WHEN** `roletag` derives roles for a job whose category is empty (or `other`)
  and whose title matches no named-role alias
- **THEN** the derived `roles` are empty

### Requirement: Roles are derived at index time, not stored or backfilled

The `roles` facet SHALL be computed at index time by `search.FromJob` from the
job's already-derived seniority and category columns and its title. There SHALL
be no `jobs.roles` column, no schema migration, and no `backfill-derive` support
for roles; a reindex SHALL populate `roles` on existing documents (the same
index-only pattern as the derived `posted_ts` field). `roles` is an index/search
concern and SHALL NOT be added to the public job wire shape returned by the job
read endpoints.

#### Scenario: A reindex populates roles without a schema change

- **WHEN** the jobs index is rebuilt for existing jobs
- **THEN** each document carries a `roles` array derived from its seniority,
  category, and title, and no Postgres column or backfill was required

#### Scenario: Roles do not appear in the public job read shape

- **WHEN** a job is read through a public job read endpoint (list, detail,
  company, or search result)
- **THEN** the returned job wire shape does not include a `roles` field

### Requirement: The role catalog is the source of truth for picker labels

`roletag` SHALL expose a curated role catalog mapping each canonical role slug to
a human label (e.g. `senior_backend` → "Senior Backend Engineer",
`founding_engineer` → "Founding Engineer"). `cmd/gen-contracts` SHALL emit this
catalog (slug → label) into the web contracts, so the frontend renders role
labels from the generated catalog rather than a hand-maintained list or the raw
slug. Every slug `roletag` can derive SHALL be present in the catalog.

#### Scenario: The catalog is emitted to the web contracts

- **WHEN** `cmd/gen-contracts` runs
- **THEN** the generated web contracts include the role catalog mapping each
  role's slug to its label

#### Scenario: Every derivable role has a catalog entry

- **WHEN** `roletag` can derive a role slug (composite or named)
- **THEN** that slug has a corresponding label in the catalog

### Requirement: Roles are served with live facet counts

The `roles` facet SHALL be exposed in the `GET /api/v1/jobs/facets` distribution,
keyed by the public param name `role`, so the picker can present roles ordered by
their live open-vacancy counts under the current filter scope. The distribution
SHALL respect the same query params and scoping as the other facets.

#### Scenario: Role counts are returned by the facet endpoint

- **WHEN** a client requests `GET /api/v1/jobs/facets`
- **THEN** the `facets` map includes a `role` entry mapping each present role
  slug to its open-vacancy count under the applied filter scope
