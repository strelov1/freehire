## Context

`internal/enrich/langchain.go:buildSystemPrompt` asks the LLM for the full
`Enrichment` contract. Nine of those fields are served dict-only: `jobview`
overwrites `work_mode`/`seniority`/`category` with the `internal/jobderive`
dictionary values, serves `skills`/`posting_language`/`experience_years_min`/
`employment_type`/`education_level`/`english_level` from the same deterministic
derivation, and the Meilisearch facets are built from that dict-fed projection.
The LLM's copies are captured only as unserved "discovery material" (per the
`ai-enrichment` "Unserved discovery facets are captured raw" requirement).

The paid LLM is a metered proxy (per-token spend), so every enrichment pays
output tokens — including a full `skills` array and several enums — for data no
reader ever sees. A read-only prod spike over `jobs.enrichment` (1.52M enriched
rows) confirmed these fields are dict-served across the catalogue.

## Goals / Non-Goals

**Goals:**
- Stop asking the LLM for the nine dict-backed fields, cutting per-call output
  tokens with zero change to any served value.
- Keep the change forward-only and contract-stable: no `enrich.Version` bump, no
  re-enrichment, no struct/DB/frontend change.

**Non-Goals:**
- Dropping `relocation` — it is a live search facet (`enrichment.relocation` in
  `internal/search`) and a profile-match input (`web/src/lib/facetModel.ts`) with
  no dictionary backing; removing it would break a user filter for a single
  scalar enum's worth of tokens. Out of scope.
- Shrinking `maxDescriptionRunes` (input-token saving) — unvalidated risk to
  `summary`/salary extraction depth; a separate change if pursued.
- Removing the fields from the `Enrichment` struct — kept so old payloads parse
  and the contract stays stable; they simply read back empty going forward.

## Decisions

- **Prompt-only edit.** The only production change is `buildSystemPrompt`: remove
  the `enum(...)` lines for `work_mode`, `seniority`, `category`,
  `employment_type`, `education_level`, `english_level`; drop `skills`,
  `posting_language`, `experience_years_min` from the "Other keys" line; and
  remove the discovery-facet "you MAY return a concise lowercase label of your
  own" exception paragraph **except** for `countries`/`regions`, which stay
  requested (the hybrid geographic bucket). `Validate`/`Sanitize` are untouched —
  they are a no-op on an absent field and must still capture a raw
  `countries`/`regions` or an old payload's discovery value.
- **Keep the geographic hybrid.** `countries`/`regions` remain in the prompt with
  their novel-label allowance; the "regions is the job's geographic area…"
  guidance and the "If the Location field is empty, the URL path…" hint stay.
- **No version bump.** Consistent with the discovery-facet convention this
  amends: the change is going-forward only; existing payloads keep their
  now-orphaned discovery copies untouched.

## Risks / Trade-offs

- **Lost discovery material.** We stop mining novel out-of-vocabulary labels for
  `work_mode`/`seniority`/`category`/`skills` (the deliberate purpose of the
  amended requirement). Accepted: the mining has no active consumer, and
  `countries`/`regions` discovery is retained. Reversible by re-adding the enum
  lines.
- **Budget-model prompt sensitivity.** The enrichment prompt is order- and
  wording-sensitive (a budget model can drop trailing keys). Removing keys
  shortens the prompt and should not regress the retained fields, but the tests
  MUST assert the retained keys (`summary`, salary, `company_type/size`,
  `domains`, `countries`/`regions`, `relocation`) are still requested, not only
  that the removed ones are gone.
- **Spec/code drift reconciled.** The amended requirement corrects the
  served-and-validated enum set to the actual `servedScalarEnums`+`domains`
  (`relocation`, `salary_period`, `company_type`, `company_size`, `domains`),
  dropping `employment_type`/`english_level`/`education_level` from that claim
  (they are dict-covered, not in `servedScalarEnums`).
