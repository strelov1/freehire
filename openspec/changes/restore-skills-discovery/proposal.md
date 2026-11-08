## Why

`enrich-prompt-trim` (#659) stopped asking the LLM for nine dict-backed fields —
`work_mode`, `seniority`, `category`, `skills`, `employment_type`,
`education_level`, `english_level`, `posting_language`, `experience_years_min` —
because `jobview` serves all of them from the deterministic dictionaries, so the
LLM's copies were paid-for output tokens nobody read.

`skills` is different from the other eight: `internal/skilltag`'s dictionary
covers a genuinely open-ended domain (new frameworks/tools/languages appear
continuously), unlike the other eight closed, slow-changing taxonomies. Before
#659, `skills` was one of the discovery facets the prompt deliberately let the
model answer outside the dictionary, specifically to surface vocabulary the
dictionary is missing. We want that discovery signal back to drive future
`internal/skilltag` dictionary expansion — the other eight stay dropped, since
none of their taxonomies benefit from discovery the way an open skills
vocabulary does.

## What Changes

- Re-request `skills` in the enrichment system prompt
  (`internal/enrich/langchain.go`, `buildSystemPrompt`): restore the
  `skills (array of lowercase tokens, e.g. go, postgresql)` entry to the "Other
  keys" line, exactly as worded before #659.
- Re-include `skills` in the LLM request schema (`internal/enrich/schema.go`):
  remove it from `unaskedFields`, so the strict JSON schema stops omitting it.
- Update the comments in `schema.go` and `enrichment.go` that currently describe
  `skills` as dict-only/unasked — they no longer hold once this ships.
- **Reuse the existing `Enrichment.Skills []string` field** (`json:"skills"`) —
  it was never removed from the contract by #659, only left permanently empty.
  No new field, no struct change, no DB/migration change.
- **No contract version change and no re-enrichment.** `enrich.Version` is NOT
  bumped; existing (empty) `skills` payloads are NOT backfilled. New and
  re-enriched jobs pick it up going forward, matching the forward-only
  convention `enrich-prompt-trim` itself established.
- **Not in scope:** the other eight fields `enrich-prompt-trim` dropped stay
  dropped — no revert for `work_mode`, `seniority`, `category`,
  `employment_type`, `education_level`, `english_level`, `posting_language`,
  `experience_years_min`. Also not in scope: any aggregation/dedup pipeline that
  turns raw `skills` values into `internal/skilltag` dictionary candidates —
  this change only restarts the raw capture; mining it into dictionary
  additions is a separate future change.

## Capabilities

### New Capabilities

_None._

### Modified Capabilities

- `ai-enrichment`: "Unserved discovery facets are captured raw, not validated"
  narrows again — `skills` re-joins `countries`/`regions` as a facet the prompt
  DOES request and MAY answer outside its (nonexistent) closed vocabulary,
  captured raw and unvalidated. The other eight fields named in that
  requirement are unaffected and stay unrequested.

## Impact

- Affected code: `internal/enrich/langchain.go`, `internal/enrich/schema.go`,
  `internal/enrich/enrichment.go` (comments only).
- Affected spec: `openspec/specs/ai-enrichment/spec.md`, requirement "Unserved
  discovery facets are captured raw, not validated".
- Cost: one array field re-added to LLM output per enrichment call (a small
  fraction of the nine-field output `enrich-prompt-trim` removed).
- No schema/migration/frontend change. `jobview`'s served `skills` projection
  (from `internal/skilltag`) is unchanged — this only restarts collection of the
  LLM's raw, unserved copy.
