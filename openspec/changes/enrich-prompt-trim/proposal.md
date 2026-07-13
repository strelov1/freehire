## Why

Every `cmd/enrich` LLM call currently asks the model to emit a set of enum/array
fields — `work_mode`, `seniority`, `category`, `skills`, `employment_type`,
`education_level`, `english_level`, `posting_language`, `experience_years_min` —
whose values are **never served**. These facets are served dict-only: `jobview`
overwrites them with the deterministic dictionary values (`internal/jobderive`),
and the search index is built from the dictionary-fed projection, so the LLM's
copies are captured only as unserved "discovery material" for later vocabulary
mining. We pay output tokens on every enrichment (including a full `skills`
array and multiple enums) for data no reader ever sees. The paid LLM is a
metered proxy, so this is direct, unconditional spend on unused output.

## What Changes

- Remove the nine dict-backed fields above from the enrichment system prompt
  (`internal/enrich/langchain.go`, `buildSystemPrompt`): drop their `enum(...)`
  lines, the discovery-facet "you MAY return a concise lowercase label of your
  own" exception paragraph (it only governs these), and the `skills` /
  `posting_language` / `experience_years_min` entries from the "Other keys" line.
- **Reverse the deliberate "prompt captures discovery facets raw" decision** for
  `work_mode`, `seniority`, `category`, `skills`: we stop mining novel
  out-of-vocabulary labels for these facets in exchange for lower per-call cost.
  The dictionaries remain the sole served source (unchanged), so no served output
  changes.
- **KEEP in the prompt** (genuinely served or hybrid, must not be trimmed):
  `summary` (synthesized), `salary_min/max/currency`, `visa_sponsorship`,
  `timezone_note`, `company_type`, `company_size`, `domains`, `relocation`, and
  `countries`/`regions` (the deliberate dict-then-LLM hybrid where the LLM fills
  the unpinned geographic bucket via `jobview.geoFacet`).
- **No contract change and no re-enrichment.** The `Enrichment` struct fields
  stay (they now simply read back empty from new enrichments); `enrich.Version`
  is NOT bumped and existing payloads are NOT re-enriched. The change is
  forward-only, exactly like the discovery-facet convention it amends.
- **Not in scope (deliberately excluded):** dropping `relocation` (it is a live
  search facet + profile-match input with no dictionary backing — removing it
  would break a user filter while saving only a single scalar enum); shrinking
  `maxDescriptionRunes` (input-token saving, unvalidated risk to summary/salary
  extraction depth — a separate change if pursued).

## Capabilities

### New Capabilities

_None._

### Modified Capabilities

- `ai-enrichment`: the requirement that the worker captures the discovery facets
  raw **via a prompt that asks for them** narrows — the prompt SHALL no longer
  request `work_mode`, `seniority`, `category`, `skills` (nor the non-enum
  `posting_language`, `experience_years_min`, and the served-but-dict-covered
  `employment_type`, `education_level`, `english_level`). The extraction scenario
  that asserts the returned `Enrichment` carries `seniority`/`work_mode`/`skills`
  is updated to reflect that these are no longer requested. Also reconciles a
  spec/code drift: `employment_type`/`english_level`/`education_level` are
  described as validated "served enum fields" but are dict-served and not in
  `servedScalarEnums`.

## Impact

- **Code:** `internal/enrich/langchain.go` (`buildSystemPrompt`) only. No change
  to `internal/enrich/enrichment.go` contract, `Validate`, or `Sanitize`
  (removed fields simply stop arriving; existing sanitize/validate logic is a
  no-op on an absent field).
- **No DB / migration / reindex / frontend** impact: served facets are dict-fed
  and unchanged; the search index is unaffected.
- **Observable effect:** new enrichment payloads stop carrying LLM copies of the
  nine facets (and any novel discovery labels for them); per-call output token
  count drops. Existing payloads are untouched.
