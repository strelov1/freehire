## Context

`jobderive.Derive` already unifies the three curated dictionaries
(`location`/`skilltag`/`classify`) into one call producing the six facets
(countries, regions, work_mode, skills, seniority, category), used by both the
ingest pipeline and the moderator write path. The *derivation* is unified; the
*read-time sourcing* and the *backfill tooling* are not.

`jobview.FromRow` is the single chokepoint that builds the public wire shape for
the API and the search index. Today it merges the deterministic columns with the
LLM enrichment: it unions `countries`/`regions`/`skills`, and lets the LLM win for
`work_mode` (and lets the LLM win with dict fallback for `seniority`/`category`).
This couples production facets to the LLM. A later change wants to relax the LLM
into a free-form discovery signal; doing that today would corrupt the served
facets. This change decouples them: the dictionaries become the sole production
source of the six facets, and the LLM's values for them stop being served.

Operationally, three separate backfill workers (`backfill-geo`/`-skills`/`-class`)
each rewrite one facet column and each need a reindex. They are collapsed into one
`backfill-derive` pass.

## Goals / Non-Goals

**Goals:**
- `jobview.FromRow` sources the six facets from the `jobs` columns only (dict-only).
- The LLM's values for the six facets are excluded from the served wire shape but
  left intact in the `jobs.enrichment` JSONB.
- One `cmd/backfill-derive` re-derives all six facet columns in a single pass via
  `jobderive.Derive`, replacing the three per-facet commands.

**Non-Goals:**
- Relaxing the LLM / dropping `Sanitize`/`Validate` / capturing discovery output.
- Enriching the dictionaries (more aliases/coverage).
- Any normalization tooling for the captured LLM variability.
- Changing the LLM-only enrichment fields (salary, employment_type, english_level,
  education_level, domains, company_type/size, relocation, visa) — they are
  untouched and still served from enrichment.
- Re-slugging or any change to slug derivation.

## Decisions

**Decision: dict-only, not dict-wins-with-LLM-fallback.** `FromRow` serves each of
the six facets from the `jobs` column alone; an unresolved (empty) facet is served
empty, never filled from the LLM. *Why over fallback:* the end-state is "LLM =
discovery"; any LLM contribution to the six facets would leak free-form values once
the LLM is relaxed, re-coupling production to it. A clean cut now makes the later
relax change safe by construction. *Trade-off accepted by the user:* coverage dips
(notably `skills`) until the dictionaries are enriched in a later change.

**Decision: leave the enrichment JSONB untouched.** `FromRow` simply stops reading
the six fields from it; the stored payload keeps the raw LLM values as discovery
material. *Why:* avoids a destructive migration and preserves the very signal the
next change will mine.

**Decision: one `backfill-derive` using `jobderive.Derive`, rewriting only the six
facet columns.** It does not touch slugs (re-slug is a separate deliberate
command). A single new combined sqlc `UPDATE` writes all six columns per row. The
three old commands and their per-column queries are removed. *Why over keeping
three:* one pass, one reindex, one cron entry; the derivation is already unified in
`jobderive`, so three commands are now redundant.

**Decision: work_mode backfill preserves a set value.** As today, the pass fills
`work_mode` from the parsed location only when the row's `work_mode` is empty,
because the structured ATS signal is unavailable at backfill time and a re-crawl
will refresh it.

## Risks / Trade-offs

- **Served facet coverage drops (esp. skills)** until dictionaries are enriched →
  Mitigation: accepted explicitly; the `backfill-derive` pass maximizes what the
  current dictionaries can resolve, and the follow-up dictionary-enrichment change
  closes the gap.
- **Stale search index after deploy** (facets change shape) → Mitigation: deploy
  tail runs `backfill-derive` then a single `reindex`; documented in the migration
  plan and tasks.
- **Removing three cmd binaries breaks the image build / cron if missed** →
  Mitigation: explicit tasks for the Dockerfile (−3/+1) and the `freehire-ops`
  cron; `go build ./...` and an image build catch a missed reference.
- **Behavior change is observable to API/SPA consumers** (a job may now report
  fewer skills/regions) → Mitigation: this is the intended doctrine; no contract
  field is added or removed, only the values' provenance narrows.

## Migration Plan

1. Ship code: `FromRow` dict-only, new `cmd/backfill-derive`, removed three
   commands + their queries, regenerated `internal/db`, Dockerfile and cron updated.
2. Deploy the app image (rebuild from origin/main).
3. Run `cmd/backfill-derive` once to re-derive the six facet columns on existing
   rows.
4. Run a single `reindex` so the Meilisearch facets reflect dict-only sourcing.
5. Rollback: revert the `FromRow` change (restores union/precedence); the
   `backfill-derive` writes are idempotent and non-destructive (they only rewrite
   the deterministic columns the dictionaries already own), so no data rollback is
   needed.

## Open Questions

None — scope, the dict-only decision, and the single-change boundary are settled.
