## Why

Canonical skill matching already treats `IaC` and `infrastructure as code` as the
same skill (`skilltag` aliases → one slug), but a literal ATS keyword screen (the
Taleo-style matcher) only sees the PDF text layer. A tailored CV that still says
`IaC` fails that screen when the JD wrote the long form — and today the tailor LLM
burns tokens rediscovering synonym pairs the dictionary already knows. A
deterministic pre-pass should align surface forms to the JD **before** any model
turn, so autopilot spends its budget on evidence and substance, not thesaurus work.

## What Changes

- **Deterministic JD surface-align pre-pass (this change / Phase 1).** From the
  vacancy description, resolve preferred surface forms per canonical skilltag slug;
  on the tailored CV, replace the candidate's aliases of those same canonicals with
  the JD's spelling and casing (mirror the JD — expand or shrink). Pure Go,
  dictionary-backed, no LLM.
- **Two replace tiers.** Skills chips and role/project stacks: any alias of the
  canonical. Summary and bullets: only unambiguous aliases (phrases and strong
  acronyms such as `IaC`, `k8s`); never ambiguous English-word aliases (`go`,
  `react`, `c`) or 1–2 letter tokens.
- **Same-canonical chip collapse.** After rewrite, duplicate skills-line items
  that now share one surface are collapsed (not family dedup).
- **Apply before the model runs.** Align the document at first mint (before
  insert). Re-align on reset-from-résumé and at autopilot start. Autopilot/reset
  writes go through the CV editor (revision), not a silent row overwrite. Repeated
  bootstrap of an existing copy does not re-align.
- **Brief interactive tailor and autopilot** that surfaces are already aligned so
  the model does not redo or undo the renames.
- **Optional alignment receipt** (applied `from`→`to` list) for logs / a short
  workspace note — not a new scored category in this change.

### Future phases (not in this change)

Recorded so later work can dedupe related skills without re-litigating scope:

- **Phase 2 — Skill families (core ↔ related members).** Curated links between
  distinct canonicals that name one capability at different grains, e.g. core
  `vector-databases` with members `pgvector`, `pinecone`, `weaviate`, `qdrant`,
  `chromadb`, `milvus` (and carefully gated looser members later). Keep facet
  values distinct; family is a relation for match/retrieval/tailor, not alias
  collapse. Enables: JD wants the core, CV has a member → family hit + optional
  surface plant of the JD term when evidence supports it.
- **Phase 3 — Family-aware ensure + dedup.** Use families to (a) avoid double-
  counting the same capability under two slugs in keyword/coverage scores, (b)
  prefer the JD's member or core term when rewriting, (c) dedupe skills-section
  chips that are family-redundant once the JD term is present
  (`pgvector` + `vector-databases` → keep what the JD asked for).
- **Phase 4 — Literal coverage diagnostic.** Report canonical matches that still
  miss the JD's literal surface on the rendered text (diagnose-only lens for the
  Taleo-style screen), building on Phase 1's surface map.

### Out of scope (this change and not assumed by future phases above)

- Replacing or weakening `skilltag` alias→canonical matching for ingest/facets.
- Collapsing related products into one facet value (filters must stay precise).
- Extending or merging `skilladjacency` peer substitutes (react↔vue) with families
  — peers stay in `skilladjacency`; families are a different relation.
- Asking the LLM to discover or invent synonym/family links.
- Inventing skills or experience the candidate does not have; paraphrasing bullets
  for tone; keyword stuffing gaps the bank cannot evidence.
- ATS-provider-specific modes gated on `source=taleo` (literal align is always-on
  for every vacancy).
- Changing fit-analysis `synonym-only` / evidence-strength semantics, or the
  evidence gate / provenance rules.
- Meilisearch schema / reindex work.

## Capabilities

### New Capabilities
- `jd-surface-align`: deterministic alignment of a tailored CV's skill surface
  forms to the vacancy's preferred spellings, applied before tailor/autopilot LLM
  turns.

### Modified Capabilities
- `tailor-autopilot`: autopilot start SHALL run (or confirm) surface alignment on
  the bound tailored CV before the unattended turn begins.
- `cv-tailoring`: tailor bootstrap SHALL surface-align a newly minted tailored
  copy against the bound vacancy before the first model turn; reset-from-résumé
  SHALL re-align the rebuilt copy.

## Impact

- **Backend:** new pure helper (likely under `internal/skilltag` or a thin sibling)
  that maps JD text → preferred surfaces and rewrites a `cv.Document`; mint aligns
  before `CreateTailored`; autopilot start and reset-from-résumé commit through
  `cvedit` (the only update path for `cvs.data`).
- **No migration, no reindex** — document rewrite only; facets unchanged.
- **Frontend:** optional receipt display later; not required for Phase 1 to ship.
- **Tokens:** fewer autopilot `cv_edit` rounds spent on acronym/expansion swaps;
  step ceiling freed for evidence work.
