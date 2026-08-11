## Context

`internal/classify/dictionaries.go` resolves a job title to a `category` via an ordered
list of `(phrase, category)` pairs — first match wins, phrase-substring, so a bare terminal
entry like `{"analyst", "data_analytics"}` or `{"manager", "management"}` acts as a catch-all
that a more specific phrase must be placed ABOVE to avoid being stolen (the doctrine
established by `expand-role-taxonomy`, PR#1331-adjacent work). Categories already carry a
tech/non-tech partition (`TechCategories`/`NonTechCategories` in `internal/enrich/enrichment.go`)
that this change does not touch — every alias added here routes to a category whose
partition membership is already decided.

Prod-DB sampling (see proposal) confirmed the gap is pure alias coverage, not missing
categories: `project_management`, `security`, `devops`, `data_engineering`, `data_analytics`
are all live and well populated. The exceptions are titles phrased in ways the dictionary
never learned — most commonly the gerund/discipline form ("Platform Engineering" vs the
already-aliased "Platform Engineer") or a newer/compound title (MLOps, DevSecOps, Analytics
Engineer) that didn't exist when the category was first stocked.

## Goals / Non-Goals

**Goals:**
- Route the ~20 sampled title clusters (and their close variants) to their correct existing
  category, following the alias-ordering doctrine exactly.
- Add SAFe/CSM/PSM/PMP as skill tags via the existing category-scoped acronym mechanism
  (`internal/skilltag`), scoped to `project_management` so the ambiguous bare acronyms never
  leak into other categories' text.
- Re-derive and re-index the existing catalogue so the fix reaches already-ingested rows,
  not just future ingests.

**Non-Goals:**
- No new category values, no changes to `CategoryValues`/`TechCategories`/`NonTechCategories`.
- No attempt at exhaustive coverage of every possible IT title — this closes the specific
  gaps the research surfaced, not a general-purpose title-parsing rewrite.
- `compliance` (bare) and `safe` (bare) are explicitly OUT — sampled and rejected as word-traps.

## Decisions

**MLOps → `devops`, not `ml_ai` or `data_engineering`.** An MLOps title is about the
operational lifecycle of ML systems — CI/CD for models, deployment, monitoring, infra — the
same shape of work as DevOps, specialized to ML artifacts rather than general services.
`ml_ai`/`ai_engineering` are reserved for building/researching the models themselves;
`data_engineering` for data pipelines. Keeping MLOps in `devops` keeps that split consistent:
category = "what kind of work", not "what domain object it touches".

**DevSecOps → `security`, not `devops`.** The role's defining trait is embedding security
tooling (SAST/DAST, container/IaC scanning, policy-as-code) into a pipeline that already
exists — the security responsibility is why the title exists at all, not an incidental
tag. This mirrors the existing bare `{"appsec", "security"}` / `{"cybersecurity", "security"}`
entries. Alternative considered: `devops`, rejected because it would make DevSecOps
indistinguishable from plain DevOps in the facet, defeating the purpose of adding it.

**Analytics Engineer → `data_analytics`, not `data_engineering`.** The title (popularized by
dbt) sits between data engineering and analysis, but its output — governed, tested data
models consumed by analysts/BI — is analytics-facing, and it already sits next to the
existing `{"bi analyst", "data_analytics"}` / `{"bi developer", "data_analytics"}` entries.
`data_engineering` stays for raw pipeline/platform work (`data engineer`, `data platform`,
`data steward`, `data governance`).

**Add gerund/discipline-noun form alongside the existing role-noun alias.** Discovered
sampling "Platform Engineer" misses: the dictionary has `{"platform engineer", "devops"}`
but not `{"platform engineering", "devops"}`, so team/discipline-named titles ("Platform
Engineering Team Leader", "Senior Platform Engineering Lead") fall through even though the
role-noun form is covered. Apply the same check to every alias added in this change — add
both forms where the gerund is plausible as a title fragment.

**SAFe/CSM/PSM/PMP use the category-scoped acronym mechanism, not new skilltag phrase
entries.** `internal/skilltag` already has this exact shape for `RAG` (ambiguous generally,
safe within `ai_engineering`/`ml_ai`). CSM collides with Customer Success Manager, PSM/PMP
are common enough short strings to be dangerous unscoped, and bare SAFe collides with the
common English word — restricting all four to job text already categorized
`project_management` is precision-first and requires no new normative rule, only new data
in the acronym allow-lists. Full phrase forms ("Certified ScrumMaster", "Professional Scrum
Master", "Scaled Agile Framework", "Project Management Professional") are added as ordinary
unscoped aliases since they're unambiguous regardless of category.

## Risks / Trade-offs

- **[Risk] A newly added phrase alias steals a title that should resolve elsewhere entirely**
  (not just falls into a worse bucket, but a category that was previously correct) →
  Mitigation: every alias is placed only above the SPECIFIC terminal fall-throughs it could
  collide with (per the ordering doctrine), never reordered relative to other specific
  aliases; task list requires a fall-through-guard test per new alias cluster, mirroring
  `expand-role-taxonomy`'s task 3.2.
- **[Risk] `backfill-derive` re-classifies rows whose category was hand-corrected or already
  fine, causing churn** → Mitigation: `backfill-derive` is idempotent and already the
  established playbook (used by `expand-role-taxonomy`); it only changes rows where the
  newly-expanded dictionary derives a different value than what's stored.
- **[Trade-off] Coverage stays sub-100% for several clusters** (e.g. GRC "governance, risk
  and compliance" phrasing varies more than sampled) — accepted; this change closes the
  clusters the research quantified, not a promise of exhaustive coverage.

## Migration Plan

1. Ship the dictionary + skilltag changes (pure code, no schema change, no migration).
2. Deploy via the standard `release.sh freehire` blue/green flow.
3. Run `go run ./cmd/backfill-derive` on host-2 to re-classify existing rows (deterministic
   facets only — no LLM cost).
4. Run `make reindex` (facet reindex) to push corrected categories/skills into Meilisearch —
   never stack with `reindex-companies` (documented deadlock hazard).
5. Spot-check a handful of the sampled title clusters against `/api/v1/jobs/facets` and the
   live `category=`/`skills=` filters post-reindex.

No rollback complexity beyond the standard `release.sh`/`rollback.sh` blue/green flip;
`backfill-derive`/`reindex` are re-run safely if a dictionary entry needs correcting.

## Open Questions

None — the three ambiguous category assignments (MLOps, DevSecOps, Analytics Engineer) are
resolved above.
