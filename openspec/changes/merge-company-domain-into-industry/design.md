## Context

The companies catalogue carries two facets for one question. `domains` (20 values,
`internal/vocab.DomainValues`) is job-derived: `RefreshCompanyFacets` unions the
`enrichment` domain of a company's open, searchable jobs. `industries` (74 values,
`internal/industrytag`) is importer-owned: `cmd/import-yc` and
`cmd/import-company-industries` write it, and nothing else may.

That ownership split is load-bearing. `docs/agents/company-facets.md` states the two
models "must never bleed into each other", and a test asserts `RefreshCompanyFacets`
never references a curated column. Any design that writes derived values into
`companies.industries` breaks it — the next recompute or the next importer run would
erase the other's work.

Production measurements (2026-08-17), companies with `job_count > 0`:

| | count |
|---|---|
| no curated industry | 116,647 |
| of those, carrying a mappable domain | 45,967 |
| of those, carrying a tagline | 2,771 |
| of those, carrying a description | 966 |

The text columns are effectively empty, so `domains` is the only derived source with
enough reach to matter.

## Goals / Non-Goals

**Goals:**

- One industry filter, resolving through both the curated column and the job-derived
  one, without either owning the other's data.
- Coverage of companies with open jobs from 27% to ~66%.
- No migration, no backfill, no reindex.

**Non-Goals:**

- Changing what `RefreshCompanyFacets` computes, or the `domains` column itself.
- Cleaning the legacy `saas` value out of `companies.domains`.
- A forward mapping (industry → domain label) for display. Company cards read
  `industries[0]` and render nothing when it is empty; nothing needs the reverse yet.
- Touching the job catalogue's own `domains` facet, which is a different surface over
  a different table.

## Decisions

### Translate at query time; store nothing

The alternative was a job-derived `companies.industries_derived` column, computed in
the same `RefreshCompanyFacets` pass and filtered alongside the curated one. It keeps
ownership clean and puts the derived values where a human can see them.

Rejected on cost against benefit. It needs a migration, a backfill over 388k rows, and
a new Meilisearch filterable attribute — and adding a filterable attribute means a full
reindex window during which the filter answers 500 (this has bitten before; see the
`new-filterable-attr-reindex-window` incident). Query-time translation needs none of
that: `industries` and `domains` are both already stored, both already indexed, both
already filterable. Rollback is a code revert.

The cost is that the rule lives in the query rather than in the data. Acceptable here
because the mapping is a pure function of a 20-value vocabulary — there is no history
to preserve and nothing to recompute if it changes.

### The mapping is a dictionary, not a heuristic

It lives in `internal/industrytag` beside the vocabulary it produces into, written the
natural direction (domain → industry, since a domain has at most one industry) and
inverted once at package init for the lookup the filter needs.

17 of the 20 domains map. `other` does not, being the classifier declining to answer.
`media` and `mobility` do not either: the curated vocabulary's nearest values
(`entertainment`, `automotive`) are narrower than the domains they would stand in for,
and stretching them would put companies under an industry that misdescribes them. That
costs ~4,700 companies of reach and is the same trade every dictionary here makes —
emit nothing rather than guess. A value outside `vocab.DomainValues` (`saas`) maps to
nothing by the same rule, with no special case needed.

Invariant tests assert every mapped target is a real canonical industry and every key
a real domain, so a typo cannot produce a value the facet could never offer.

### Both backends translate independently, and a test holds them together

`GET /api/v1/companies` is served by Meilisearch or by Postgres depending on the
request — a rating sort forces Postgres — and within one page the list and its
`meta.total` can come from different paths. So both must resolve the facet identically.

Rather than compute the translation in the handler and thread it into both, each path
calls the same `industrytag` helper where it builds its own filter: Meili inside
`CompanyFilterFromValues`, Postgres in the handler's query params. Two call sites, one
rule. The seam is covered by a test that runs one filter through both paths and
compares the matched sets — the risk is real enough to test directly rather than to
guard by convention.

Meilisearch needs no new capability for this: `CompanyFilterFromValues` already ORs the
values within one facet into a group, so widening the `industries` group with fragments
over the `domains` attribute is an addition to an existing structure, not a new one.

### The Domain facet leaves the UI, not the API

`domains` stays a working query parameter on `GET /api/v1/companies`, exactly as
`subindustries` did when its facet was retired in #2071. Removing the control does not
justify breaking a documented parameter, and the parameter still means something
precise that the widened `industries` facet cannot express (the raw job-derived value,
including `other` and `saas`).

## Risks / Trade-offs

- **The two backends drift apart** → a test runs the same filter through both and
  compares matched sets. This is the failure this change most plausibly produces, so it
  is tested at the seam rather than in each path separately.
- **Result counts jump without explanation** → this is the intended effect (+45,967
  companies reachable), but it will look like a regression to anyone comparing before
  and after. Recorded here and in the proposal so the jump is attributable.
- **The mapping quietly rots as either vocabulary changes** → invariant tests fail the
  build if a mapped industry or domain stops existing. They do not catch a *newly added*
  domain that nobody maps; that is a gap accepted for now, since domains change rarely
  and the effect of missing one is lost reach, not wrong data.
- **`media`/`mobility` reach is lost** → ~4,700 companies stop being filterable by the
  removed Domain control and do not become filterable by Industry. Accepted per the
  dictionary rule above; revisit only if the curated vocabulary gains honest values.

## Migration Plan

An ordinary deploy. No migration to apply, no backfill to run, no reindex to schedule —
the columns and index attributes this reads already exist and are already populated.

Rollback is a code revert; nothing is written that a revert would have to undo.

## Open Questions

None. The two questions this design turned on — whether to merge the facets at all, and
whether to materialise the derived values — were settled before it was written.
