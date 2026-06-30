## Context

getmatch.ru is a boardless marketplace adapter (`internal/sources/getmatch.go`).
Its per-offer detail endpoint (`/api/offers/{id}`, already fetched for the HTML
description) exposes structured fields the adapter currently ignores: `seniority`
(scalar) / `seniorities` (array), `specializations` (code array), `skills_objects`,
and `required_years_of_experience`.

freehire derives `seniority`/`category`/`skills`/`experience_years_min` only from
the title/description dictionaries via `jobderive.Derive`. `jobderive.Input`
already carries one structured signal — `WorkMode` — whose precedence is
"structured → location → description". The pipeline builds `jobderive.Input` from
the normalized `sources.Job` (`internal/pipeline/pipeline.go`, where
`WorkMode: j.WorkMode` is passed). This change extends that exact seam to the
other facets.

Measured impact: getmatch.ru returns ~204 jobs for `remote + senior/lead/c_level`;
freehire returns 73, because ~60% of getmatch jobs carry no seniority facet.

## Goals / Non-Goals

**Goals:**
- Let adapters emit structured `Seniority`, `Category`, `Skills`,
  `ExperienceYearsMin` on `sources.Job`, mirroring `WorkMode`'s contract.
- Give a structured source signal precedence over the dictionary in `jobderive`.
- Map getmatch's detail fields into freehire's vocabularies, dropping unmappable values.

**Non-Goals:**
- Salary (`salary_min/max/currency`): no `jobs` columns exist and it crosses the
  LLM-owned `enrichment` JSONB boundary — a separate change.
- Read-model changes: `jobview.FromRow` still serves the facets from `jobs`
  columns only; the LLM's enrichment values stay raw and unserved.
- Multi-valued seniority: getmatch's `seniorities` array is collapsed to the scalar
  `seniority` (see Risks).

## Decisions

### General seam over getmatch-local override
Extend `sources.Job` and `jobderive.Input` with the new fields and keep all
derivation in `jobderive.Derive`. Rationale: one derivation path stays testable and
reusable by future structured sources; it copies the working `WorkMode` pattern.
Alternative (writing derived values directly in the adapter, bypassing `jobderive`)
was rejected — it forks the derivation logic and does not generalize.

### Per-facet precedence
- `seniority`: source → title dict → description (extends the existing two tiers).
- `category`: source → title dict.
- `experience_years_min`: source → `jobfacts` text parse.
- `skills`: UNION of source skills and dictionary skills (skills is a set; both are
  facts, so neither replaces the other — unlike the scalar facets where source replaces).

### getmatch mapping is gatekept by freehire's vocabularies
- Grade: use the scalar `seniority`; pass through an explicit map to
  `SeniorityValues`; drop unknown grades.
- `required_years_of_experience` → `*int` (nil when absent).
- `skills_objects`: collect names, run them through `skilltag.Parse` (joining the
  names as text), keep only canonical resolutions — reusing the curated dictionary
  as the noise filter rather than adding new filtering logic.
- `specializations`: an explicit getmatch-code → `CategoryValues` map (e.g.
  `python`/`golang`/`java_scala`→`backend`, `android`→`mobile`, `dev_ops`→`devops`,
  `sre`→`sre`, `data_engineering`→`data_engineering`, `data_science`→`data_science`,
  `data_analyst_bi`→`data_analytics`, `information_security`→`security`,
  `project_management`→`project_management`, `engineering_management`→`management`).
  Unmappable codes (`business_analyst`, `system_analyst`, `architect`, …) are
  dropped. If the array resolves to more than one distinct category, emit empty —
  mirroring `getmatchWorkMode`'s "single distinct or empty" rule.

## Risks / Trade-offs

- **getmatch multi-grade collapses to one column** → `jobs.seniority` is scalar;
  taking the primary scalar `seniority` means a "middle/senior" offer shows only
  under its primary grade, unlike getmatch.ru which lists it under both. Accepted;
  the primary grade is the most useful single value.
- **`backfill-derive` cannot recover the signal** → it re-derives from stored
  columns, which lack the source's structured data. Mitigation: getmatch is
  boardless, so its next full crawl re-upserts every offer with the structured
  fields; no backfill is needed for getmatch specifically.
- **specialization map drift** → getmatch may add specialization codes over time;
  unmapped ones silently yield empty `category`. Mitigation: the map is explicit and
  unit-tested; new codes degrade to dictionary-derived category, never to a wrong one.
- **Vocabulary divergence** → getmatch grade strings must match `SeniorityValues`.
  Mitigation: an explicit map (not a raw passthrough), so a renamed/unknown grade is
  dropped rather than persisted out-of-vocabulary.

## Migration Plan

1. Ship the code (no schema/migration — columns already exist).
2. getmatch's next scheduled crawl re-ingests all offers with the structured fields.
3. Run `make reindex` so the refreshed `seniority`/`category`/`skills`/
   `experience_years_min` facets reach Meilisearch.
4. Rollback: revert the code; the columns remain valid (dictionary-derived values),
   so no data cleanup is required.

## Open Questions

None.
