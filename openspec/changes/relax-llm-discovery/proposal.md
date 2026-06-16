## Why

The dict-only doctrine made the deterministic dictionaries the sole production source of the six dictionary-covered facets (`countries`, `regions`, `work_mode`, `skills`, `seniority`, `category`); the LLM's values for them are no longer served — they sit raw in the `enrichment` JSONB. That unblocks the final original goal: **relax the LLM on exactly those six facets** so its free-form output becomes a discovery signal. Mining that signal surfaces values the curated dictionaries miss, which the maintainer normalizes back into the dictionaries (closing the loop). Because the six facets are unserved, relaxing them cannot corrupt production data.

## What Changes

- Stop sanitizing/validating the six dict-covered facets in `internal/enrich`: `Sanitize` no longer blanks an out-of-vocabulary `work_mode`/`seniority`/`category` nor filters `regions`; `Validate` no longer rejects them. Their raw LLM values (including novel, out-of-vocabulary labels) persist in the `enrichment` JSONB.
- The **served** enrichment enum fields stay strict: `employment_type`, `relocation`, `salary_period`, `english_level`, `education_level`, `company_type`, `company_size`, and `domains` are still sanitized and validated (a stray value there would reach the served object), and salary clamping is unchanged.
- Loosen the **prompt** for the six discovery facets: prefer an allowed value, but emit a concise lowercase label of your own when none fits. The served fields keep the strict "use exactly one of the allowed values" instruction.
- **Going-forward only:** `enrich.Version` is NOT bumped and nothing is re-enriched. New enrichments accumulate the raw variety; existing (already-sanitized) payloads are untouched. No deploy tail (no backfill, no reindex) — only the enrich worker ships.

## Capabilities

### New Capabilities

(none)

### Modified Capabilities

- `ai-enrichment`: the validated-write-back path exempts the six unserved discovery facets from sanitize/validate (captured raw), while the served enum fields stay validated; the prompt permits novel labels for the discovery facets.

## Impact

- Code: `internal/enrich/enrichment.go` (Sanitize/Validate field partition), `internal/enrich/langchain.go` (prompt), plus their tests.
- Served data: unchanged — `jobview.FromRow` already ignores the six facets in `enrichment` (dict-only), so out-of-vocabulary values there are inert except as discovery material.
- Ops: deploy the enrich worker image; no backfill, no reindex, no version bump.
- Discovery use: a `GROUP BY enrichment->>'category'` over freshly-enriched jobs surfaces novel labels to fold into `classify`/the vocabularies.
- Out of scope: re-enriching existing jobs, category-from-description, any served-field change.
