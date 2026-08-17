## Why

The company catalogue offers two filters for one idea. **Domain** names ~20 coarse
verticals derived from the company's job enrichment; **Industry** names 74 curated
values written by importers. A user reading the modal cannot tell which to reach
for, and each answers only part of the catalogue: Industry is set on 27% of
companies with open jobs, and Domain's own `other` bucket is its largest value.

The two are not complementary — they are the same question asked of two sources.
Measured on production: of the 116,647 companies that have open jobs but no curated
industry, 45,967 carry a domain that names an industry the curated vocabulary
already has. That population is unreachable by the filter that is supposed to find
it, while being reachable by a second filter that says the same thing in different
words.

## What Changes

- The **Industry** facet on `GET /api/v1/companies` matches a company through
  either source: its curated `companies.industries`, **or** its job-derived
  `companies.domains` translated through a curated domain→industry mapping.
  Coverage of companies with open jobs rises from 27% to ~66%.
- A new curated mapping in `internal/industrytag` translates between the two
  vocabularies. It is dict-only like every other dictionary in the codebase: 18 of
  the 20 domain values map to a canonical industry, and `media` and `mobility` map
  to nothing, because no curated value names them honestly. `other` maps to
  nothing by the same rule.
- **BREAKING (UI only):** the **Domain** facet is removed from the companies filter
  modal. The `domains` query parameter on `GET /api/v1/companies` keeps working —
  this removes a duplicate control, not a contract.
- Both query backends change together. `/companies` is served by Meilisearch or by
  Postgres depending on the request (a rating sort forces Postgres), so a fix to one
  path alone would make the rendered list disagree with its own `meta.total`.
- No migration, no backfill, no reindex: both `industries` and `domains` are already
  stored and already filterable on both paths.

## Capabilities

### New Capabilities

None. This changes how an existing facet resolves, not what the catalogue offers.

### Modified Capabilities

- `companies`: the `industries` facet gains a second matching source. The current
  requirement text does not list `industries` among the facet parameters at all —
  the facet shipped after the spec was written — so the delta both records the
  facet and states its two-source behaviour.

## Impact

- `internal/industrytag` — new curated domain↔industry mapping and its invariant tests.
- `internal/search/company.go` — `CompanyFilterFromValues` widens the `industries`
  OR-group with fragments over the `domains` attribute.
- `internal/db/queries/companies.sql` — `ListCompanies` / `CountCompanies` widen the
  `industries` predicate to an array-overlap on either column; `make sqlc` regenerates.
- `internal/handler` — the companies handler translates requested industries into the
  domain values both backends need.
- `web/src/lib/facets.ts`, `web/src/lib/filterSections.ts` — the `domains` company
  facet and its rail pane are removed. The rail-coverage test added in #2071 keeps
  the two in step.
- No schema change, no worker change, no Meilisearch settings change.

**Known limitation:** `companies.domains` holds a 21st value, `saas`, on 5,989
companies — a legacy of an earlier enrichment vocabulary, absent from
`vocab.DomainValues`. It maps to nothing and yields no industry. Cleaning that
column is separate work.
