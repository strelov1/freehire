## Context

See proposal.md - Why. The relevant existing machinery:

- `internal/dict/industrytag/domains.go` holds `domainIndustry` (19 pairs,
  domain→industry) and its inverse `industryDomains`, exposed only as
  `DomainsForIndustries(industries []string) []string` — the direction the current
  request-time filter needs. Nothing today exposes the forward direction
  (domain→industry) as data a caller can pass elsewhere.
- `RefreshCompanyFacets` (`internal/platform/db/queries/companies.sql`) is one
  set-based `UPDATE ... FROM`, run periodically by `cmd/recount-companies`. It already
  has a `dom` CTE producing each company's distinct-union `domains` array, and an
  `IS DISTINCT FROM` guard per column so a re-run rewrites only rows that changed.
- The industry facet's precedence is currently applied by both query paths at
  request time: `internal/api/handler/companies.go` (Postgres) and
  `CompanyFilterFromValues` in `internal/search/search/company.go` (Meilisearch) each
  gate the derived arm on `industries IS EMPTY`.
- The companies Meilisearch index's filterable-attribute rollout hazard is already
  documented (`new-filterable-attr-reindex-window`): adding a filterable attribute
  500s the filter until the index rebuild completes, so the attribute must exist in
  live index settings before any deployed code queries it, and `reindex-companies`
  must never be stacked with `make reindex` (same serial Meilisearch task queue).

## Goals / Non-Goals

**Goals:**

- One source of truth for the domain→industry mapping (`internal/dict/industrytag`),
  used by both the new materialization and the existing request-time
  `DomainsForIndustries` path.
- Keep `RefreshCompanyFacets` a single set-based `UPDATE`, no per-company Go loop.
- Move the derived-arm precedence and the new domain-count threshold out of
  request-time filter logic and into the materialized column, simplifying both
  filter paths rather than adding a second condition to them.

**Non-Goals:**

- Changing the domain→industry mapping table itself (the 19 pairs, `media`,
  `mobility` aliases) — untouched, already correct per #2082's design.md.
- Changing `companies.domains` or any other existing derived facet array.
- A configurable threshold. `2` is fixed, per the coverage measurement in the issue
  (`≤2` keeps ~78% of reach and drops the tail); making it configurable is
  speculative until a second data point ever asks for a different value.

## Decisions

### Materialize `industries_derived` at recompute time, not at request time

The alternative — keep computing precedence and the threshold per request — was
rejected because the threshold needs a per-company domain **count**, which is not an
attribute either backend can filter on today, and adding a synthetic "domain count"
attribute to serve one facet is a worse shape than a column that directly holds the
answer. Materializing also lets both filter paths drop their `IsUnset`/`cardinality`
precedence check entirely (the column is already empty when precedence say it should
be), which is a net simplification, not just a threshold bolted on.

### Pass the domain→industry mapping into SQL as two array parameters, not as literal SQL or as a Go-side loop

Three options, discussed with the user (see proposal's Impact and the issue's own
"suggested shape", which posed the first two):

1. **Mirror the 19 pairs as a SQL `VALUES` literal** inside
   `RefreshCompanyFacets`. Stays set-based, but creates a second copy of the mapping
   that only a human keeps in sync with `internal/dict/industrytag` — the same
   failure mode `normalize.CompanySlug` was consolidated to prevent ("there were
   four, they disagreed, and every resulting miss was silent").
2. **Compute in Go, row by row**, in `cmd/recount-companies`, after the main
   `UPDATE`. One copy of the mapping, but turns the recompute from one set-based
   statement into N round-trips (or a second bulk-upsert path), which the surrounding
   code and its comments treat as a deliberately avoided shape ("this is
   cmd/recount-companies' whole job", "pinned MATERIALIZED... re-scan... per
   aggregate" — the existing query is written to do all of this in one pass).
3. **Chosen: pass the mapping as two parallel `text[]` query parameters**
   (`industrytag.DomainIndustryPairs()` returns `(domains, industries []string)`,
   sorted, same length, `pairs[i]` is one mapping), joined in SQL via
   `unnest($domains::text[], $industries::text[]) AS map(domain, industry)`. One
   source of truth (the Go map), one set-based `UPDATE`, and the SQL only ever sees
   data, never a second copy of the dictionary's *content*.

`DomainIndustryPairs()` sits beside the existing `DomainsForIndustries` in
`domains.go`, built from the same `domainIndustry` map so there is exactly one
literal table in the codebase.

### `industries_derived` bakes in precedence, so filters become a plain OR

Because `industries_derived` is already empty for a curated company or a
high-domain-count one, `?industries=X` on both backends becomes `industries = X OR
industries_derived = X` — no conjunct, no `IS EMPTY`/`cardinality` check at request
time. This removes code rather than adding it: `CompanyFilterFromValues` drops its
`IsUnset("industries")` fragment and the loop that calls `DomainsForIndustries` per
requested value; the Postgres path drops its `cardinality(industries) = 0` guard the
same way. Both now treat `industries_derived` exactly like any other array facet
column.

### Companies index gets a new filterable attribute

`industries_derived` is added to the company document (wherever `domains`/
`industries` are set for the Meilisearch projection) and to the companies index's
filterable-attributes list, mirroring how `industries` itself is already exposed.

## Risks / Trade-offs

- **New filterable attribute during reindex window** → the companies index 500s
  filtered queries while `reindex-companies` rebuilds it with the new attribute
  (documented hazard). Mitigation: tasks.md sequences deploy-with-new-index-settings
  before backfill before `reindex-companies`, matching the pattern this index has
  already hit once.
- **Second-order derivation drifts from `domains` between recomputes** → this is the
  same eventual-consistency window every other derived facet array already has
  (`domains` itself lags `jobs` until the next recompute); `industries_derived`
  lagging `domains` by the same cadence is not a new risk, just the existing one
  applied one layer up.
- **The `≤2` threshold is still a heuristic, not a certainty** → a company with
  exactly 3 unrelated domains and one real focus industry stays unreachable via
  `industries`. Accepted per the issue's own coverage table: the alternative (a
  higher or no threshold) reintroduces the noise this change removes, and `≤2` keeps
  most of the reach.

## Migration Plan

1. Deploy code adding `industries_derived` to the companies index's filterable
   attributes and to the document projection (no schema change yet, no reader
   depends on the column yet — this step only prepares Meilisearch).
2. Run the schema migration (`companies.industries_derived text[] NOT NULL DEFAULT
   '{}'` + GIN index) and deploy the `RefreshCompanyFacets` change plus the
   simplified filter paths.
3. Backfill via `recount-companies` (idempotent, guarded, same as every other
   periodic recompute).
4. Run `reindex-companies` on its own — never stacked with `make reindex`.

Rollback is a code revert plus leaving the unused column in place (dropping a column
is its own migration, not part of this change's rollback path).

## Open Questions

None.
