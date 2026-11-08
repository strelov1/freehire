## Context

Remote reach lives only in `enrichment.countries` (ISO 3166-1 alpha-2). Today:

- `jobs.remote BOOLEAN NOT NULL DEFAULT TRUE` — raw flag set by adapters (explicit
  API boolean for Recruitee/SmartRecruiters/Ashby/Workable; `isRemote(location)`
  text heuristic for Greenhouse/Lever). Also fed to the enrichment LLM as a hint
  and surfaced publicly (`jobview.Job.remote`, `?remote=true`, SPA fallback).
- `enrichment.work_mode` — `remote`/`hybrid`/`onsite`.
- `enrichment.countries []string` — country eligibility.

Two cases collapse to empty `countries` and become indistinguishable: **global**
(open anywhere) vs **unknown** (not stated / not yet enriched). Regions (EU…)
can't be expressed at all. Enrichment is async via the `enrichment_outbox` queue,
and everything lives in the `jobs.enrichment` JSONB — no schema migration is
needed to add fields. MVP stage: no persistent data, architecture stays fluid.

## Goals / Non-Goals

**Goals:**

- Make a remote role's reach (`global`, regions, key countries) explicit and
  distinguishable from `unknown`.
- Back one curated reach facet ("Region") with one field, filtered the same way
  every other enrichment facet is.
- Collapse the public notion of "remote" to one source of truth — `work_mode`
  (the format) + `regions` (the reach) — removing the parallel raw flag.

**Non-Goals:**

- Changing the `jobs.remote` column, the source/pipeline `Remote` field, or the
  LLM hint path (all kept).
- A DB migration or `enrich.Version` bump (no persistent data at MVP).
- Reworking the granular `countries` eligibility facet (stays) or UI localization.

## Decisions

### 1. One reach field: `regions` (flat, mixed-level vocabulary)

Add a single `Regions []string` to `enrich.Enrichment`. Its vocabulary mixes
levels deliberately, because that is how reach is actually expressed and filtered:

- `global` — open anywhere (an explicit value, never inferred)
- macro-regions — `eu`, `emea`, `eea`, `uk`, `americas`, `north_america`,
  `latam`, `apac`, `mena`, `africa`
- select countries as reach areas — `us`, `ru` (extensible)

Empty `regions` = **unknown**; `global` present = open-anywhere. That single
explicit value is what makes global ≠ unknown — no discriminator field is needed.
`regions` is meaningful only when `work_mode = remote`. "remote" itself is just
the `work_mode` format.

**Why one field, not a `remote_scope` + `regions` + derived `remote_type` triple:**
the normalized version stored the reach across three fields and then flattened it
back into one derived field for the actual facet — a "decompose then recompose"
round-trip whose only payoff (global ≠ unknown) is achieved more simply by making
`global` an explicit vocabulary value. One field is the thing we store *and*
filter on.

*Trade-off — `us`/`ru` live in both `regions` (as reach) and `countries` (as ISO
eligibility):* accepted. They are distinct facets with distinct purposes (curated
reach quick-filter vs full-ISO eligibility), and the duplication is small and
explicit.

### 2. Filter `regions` directly (no derived field)

`regions` is nested under enrichment, so the search layer filters it via the dot
path `enrichment.regions` — exactly like `countries`, `skills`, `domains`. Add
`enrichment.regions` to the index filterable attributes and map a `regions`
search param to it. The curated SPA facet (Global / Russia / Europe / USA → codes
`global`/`ru`/`eu`/`us`) is a frontend curation over that one param; the field's
vocabulary can hold more than the facet surfaces.

### 3. Demote `jobs.remote` to an internal enrichment hint

`jobs.remote` has three roles; only the LLM-hint role is non-redundant:

| Role | Verdict |
|------|---------|
| Public wire `jobview.Job.remote` | dup of `work_mode=remote` → **remove** |
| Search `?remote=true` + filterable `remote` | dup of `work_mode` facet → **remove** |
| SPA `workArrangement()` fallback | dup; splits truth → **remove** |
| LLM input hint (prompt + `enrich.Provider`) | source's explicit boolean, can diverge from location text → **keep** |

The column, the source/pipeline/provider `Remote` field, and the prompt line stay
as the channel that persists the source's remote signal across the async
ingest→enrich boundary. After removal, public "remote" = `work_mode` (+ `regions`
for reach) only.

## Risks / Trade-offs

- **Losing the explicit remote boolean as a public/filter signal** → mitigated:
  it is kept as an enrichment input; public filtering moves to `work_mode`, which
  the SPA already exposes.
- **`us`/`ru` duplicated between `regions` and `countries`** → accepted (see
  Decision 1); the two back different facets.
- **Unenriched jobs show no reach** → acceptable and consistent: every other
  enrichment facet behaves this way; the facet simply won't match them, same as
  seniority/category filters.

## Migration Plan

No DB migration and no `enrich.Version` bump. Deploy code; the next ingest +
enrich run populates `regions` from the updated prompt. Recreate the dev volume
(`docker compose down -v && make up`) if a DB holds stale enrichments. Rollback =
revert the code; the unchanged column and JSONB tolerate absent fields.

## Open Questions

None — design simplified and approved mid-implementation (collapse to a single
`regions` field).
