## Why

"Do I manage people?" is a question none of the catalogue's facets answer, and it
is not a variant of one they do. Measured on production's 3,148,859 live postings,
171,726 carry an unambiguous people-management marker in the title — `head of`,
`director`, `vp`, `chief`, `supervisor`, or a craft-qualified manager such as
`engineering manager`. Of those:

| Reachable today via | Postings | Share |
|---|---|---|
| `category=management` | 15,760 | 9% |
| `seniority=c_level` | 43,027 | 25% |
| `seniority` in (`lead`, `c_level`) | 46,330 | 27% |
| **either existing facet** | **59,940** | **35%** |
| **nothing at all** | **111,786** | **65%** |

The overlap is small because the axes ask different questions. Category names the
craft, so "Director of Data Engineering" sits in `data_engineering` and "Head of
Security" in `security`; only the 15,760 whose craft *is* management land in that
category. Seniority names the grade, and the grade a manager carries varies.

The nearest available proxy is actively misleading. `seniority=lead` holds 116,893
postings, of which just 3,303 carry a management marker — **97% noise**, because
"Lead" in this catalogue overwhelmingly means Tech Lead or Lead Engineer, the
individual-contributor ladder. A candidate filtering on it to find management work,
or to avoid it, gets the wrong answer either way.

## What Changes

- A new `role_type` facet with a single value, `people_manager`, resolved
  deterministically from the job title.
- The facet is **derived at index time**, alongside `roles` and `ai_archetype` in
  `search.FromJob` — no `jobs` column, no migration, and no `cmd/backfill-derive`
  pass. A reindex populates it.
- `role_type` becomes a Meilisearch filterable attribute and a public query param,
  with the `_exclude` twin every string facet already carries.
- The SPA renders it as a pills control in the `Experience` pane, directly beneath
  the seniority pills.

**Deliberately one value, not two.** An `individual_contributor` counterpart is not
detectable: production carries 7,802 titles on the IC ladder
(`staff|principal|distinguished|fellow engineer`), 1,291 `member of technical
staff`, and **52** saying `individual contributor` outright — about 9,100 postings,
**0.3%** of the catalogue. A pill offering 9,100 results next to one offering
171,726 would read as broken, and inferring IC from the absence of a management
marker is exactly the guess this codebase's dictionaries are forbidden to make. The
existing three-state chip already serves the need honestly: excluding
`people_manager` means "no management marker", which is what we actually know.

**Deliberately title-only.** Seniority and category are title-derived too, and the
title is a short, un-TOASTed column. Description phrases (`direct reports`,
`manage a team of`) would add coverage at the cost of a far noisier signal and a
scan this design does not otherwise need. The seam stays open.

## Capabilities

### New Capabilities

- `role-type-facet`: resolving whether a posting is a people-management role from
  its title, and serving that as a filterable facet.

### Modified Capabilities

- `job-search`: the searchable index gains a top-level `role_type` filterable
  attribute and the matching query filter.
- `filter-modal`: the `Experience` pane gains the role-type pills between the
  seniority pills and the years-of-experience control.
- `api-documentation`: the documented facet vocabulary gains `role_type`, and
  `web/static/openapi.yaml` declares it on every endpoint that accepts facets.

## Impact

**Go:**
- `internal/roletype/` — new package: the marker dictionary, the blind-phrase mask,
  and `Derive(title) string`.
- `internal/vocab/vocab.go` — `RoleTypeValues`.
- `internal/search/document.go` — the new top-level field, derived in `FromJob`.
- `internal/search/client.go` — `role_type` in the filterable-attribute list.
- `internal/search/query_filter.go`, `query_params.go` — the facet param.

**Web:**
- `web/src/lib/facets.ts` — the `role_type` `FacetDef` and its option labels.
- `web/src/lib/components/filters/FilterModal.svelte` — the pills in the Experience
  pane, and the pane's staged count.
- `web/static/openapi.yaml`, `web/src/lib/docs/filters.ts` — the public contract.

**Ops:** no migration and no backfill. One hazard, which has fired three times
before: a new filterable attribute must be patched into the live index settings
**before** the binary that requests it goes live, or `/api/v1/jobs/facets` hard-500s
for every caller. Because this change adds no database column, rollback stays safe.
