## Why

Follow-up to the dict-only doctrine (`dict-production-facets`) and `workmode-from-description`. The `classify` dictionary derives `seniority` from the job **title** only, but the grade is often stated in the **description** body. Measured on prod, ~56k open jobs (64% of the enriched set) carry a seniority the LLM found but the title dictionary cannot. Teaching `classify` to read the description — with high-precision, intent-anchored phrases — recovers the subset that explicitly states the grade, the next-largest coverage gap after work_mode.

## What Changes

- Add `classify.SeniorityFromDescription(desc string) string`: scans the lowercased description for **intent-anchored** seniority phrases (NOT the bare title aliases — a bare `senior`/`lead`/`head of` in prose matches "senior management", "lead the team", "report to the head of product"), priority c_level > principal > staff > lead > senior > middle > junior > intern, returning `""` when there is no clear grade statement (never guesses). Years-of-experience banding is deliberately excluded (the band boundaries are a judgment call, not a fact).
- Wire it into `jobderive.Derive` as the lowest-priority seniority source: title dictionary first, then the description. `category` is unchanged (deferred — its description signal is too noisy).
- No schema change, no new command, no LLM change: `cmd/backfill-derive` re-derives through `jobderive`, so a post-deploy backfill recovers existing jobs.

## Capabilities

### New Capabilities

(none)

### Modified Capabilities

- `deterministic-facets`: the seniority facet gains the description as a second, lowest-priority derivation source (after the title).

## Impact

- Code: `internal/classify/classify.go` + a new phrase set (e.g. `classify/description.go`), `internal/classify/*_test.go`; `internal/jobderive/jobderive.go` (one fallback) + its test.
- Deploy: re-derives on next ingest automatically; existing jobs recovered by the shared deferred deploy tail (`cmd/backfill-derive` + one `reindex`) of the dict-only sequence.
- Out of scope: category from the description (deferred — too noisy), skill-vocabulary expansion, any LLM change.
