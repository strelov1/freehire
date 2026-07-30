# internal/classify — Seniority & Category Tagging

Deterministic seniority/category tagging from job title, feeding enrichment facets.

## Design

- Parses the **job title** at ingest into canonical `jobs.seniority`/`jobs.category` columns.
- Values from `vocab.SeniorityValues`/`vocab.CategoryValues` — EN+RU aliases, whole-word matched. Russian forms listed as full surface forms (not stems) since matcher requires word boundaries. **Never guesses**.
- Same alias→canonical dictionary design as `internal/location` and `internal/skilltag`.

## Grade-blind phrases

Some role names **contain** a grade word without stating a grade: "Member of Technical Staff" is a generic IC title, "Lead Generation" is a marketing function, "Middle East" is a region, "Mid-training" is a model-training stage. `gradeBlindPhrases` is cut from the title (each occurrence replaced by a space) **before** the seniority match, so only the remaining words can state a grade.

Ordering alone could not fix this: `staff` outranks `senior`, so "Senior Member of Technical Staff" resolved to `staff`. The mask leaves honest grades untouched — "Senior Staff Engineer" is still `staff`.

The list holds only phrases that shadow a `seniorityTable` alias; the category match reads the untouched title.

## The two design crafts

`design` means product/visual/experience design. Engineering draughting —
mechanical, electrical, civil, and the architectural/BIM family — is the separate
`engineering_design` category, a `vocab.NonTechCategories` member (surfaced as a
facet, off the LLM and embedding budgets). Silicon design is neither: it resolves to
`hardware`, where the rest of that team already sits.

Three groups sit **before** the bare `designer`/`design` entries, because the word
`design` alone would otherwise claim every "… Design Engineer": the titles that name
another craft (`network design engineer`, `cloud design engineer`, the silicon
block), the product-side markers (`product design engineer`, `ux/ui/web design
engineer`, `design engineer, product`, `service/experience/sound/game design
engineer`), and then the draughting aliases themselves. The unqualified `design
engineer` closes the block and resolves to `engineering_design`: on this catalogue
that population is overwhelmingly mechanical.

Titles where a category alias appears but names no category at all — "Software Design
Engineer" is software engineering — carry the `categoryNone` sentinel as their table
entry, and `matchCategory` serves it as `""`. Deliberately NOT a pre-match mask like
`gradeBlindPhrases`: cutting the span exposes the aliases further down the table (which
are mostly the business categories, so "Software Design Engineer - Sales Tools" read as
`sales`) and is boundary-blind. Every exit translates the sentinel — `Parse`,
`Categories`, `CategoryAliases` — because the last two feed a CV profile and the
generated web contracts.

Two consumers of `vocab.TechCategories` DELETE — the ingest catalogue filter and the
prune title rule, both through `ConfirmedNonTech`, plus prune's business rule which
reads `NonTechCategories` directly. A resolved `engineering_design` vetoes all of
them: this dictionary and the non-tech title list describe the same physical trades,
so a match between them is not the accidental kind the veto was built for.

## Serving: dict-only

`jobview.FromRow` overwrites the nested `enrichment.seniority`/`enrichment.category` with the `jobs` column — the dictionary always wins, the LLM's value is never a fallback. They remain **nested under `enrichment`** so existing search facets, SPA, and generated contracts are unchanged.

## Convention

- Adding a value: add it to `vocab.SeniorityValues`/`vocab.CategoryValues` and the title-matching dictionary.
- Dictionary change needs `cmd/backfill-derive` + `cmd/reindex` to reach existing jobs (same caveat as geography/skills).
