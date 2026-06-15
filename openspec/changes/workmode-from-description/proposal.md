## Why

Under the dict-only doctrine (change `dict-production-facets`), `work_mode` is served from the deterministic dictionary columns only. But the work-mode parser reads only the short ATS **location** string, while the work arrangement is very often stated in the job **description** body. Measured on prod, ~75k open jobs (85% of the enriched set) carry a work_mode the LLM found but the dictionary cannot — because it never reads the description. This is the single largest coverage gap blocking the dict-only deploy. Teaching the deterministic parser to read the description closes most of it.

## What Changes

- Add `location.WorkModeFromDescription(desc string) string`: scans the lowercased description for a **conservative, high-precision** phrase set (distinct from the short-location markers — e.g. `fully remote` / `remote-first`, not a bare `remote` that matches "remote team"; `hybrid role`, not a bare `hybrid` that matches "hybrid cloud"), priority hybrid > remote > onsite, returning `""` when there is no clear signal (never guesses).
- Wire it into `jobderive.Derive` as the lowest-priority work_mode source: the net order becomes **structured (ATS) → parsed location → description**. The description only fills a work_mode the structured signal and the location marker both left empty.
- No schema change, no new command: `cmd/backfill-derive` already re-derives work_mode via `jobderive`, so a post-deploy backfill recovers the gap for existing jobs.

## Capabilities

### New Capabilities

(none)

### Modified Capabilities

- `job-geography`: the "Work mode is resolved by precedence across sources" requirement gains the description as a third, lowest-priority source.

## Impact

- Code: `internal/location/location.go` (new `WorkModeFromDescription` + its phrase set), `internal/location/*_test.go`; `internal/jobderive/jobderive.go` (one fallback) + its test.
- Deploy: re-derives on next ingest automatically; existing jobs recovered by running `cmd/backfill-derive` + one `reindex` (the same deploy tail the `dict-production-facets` change already needs). This change is a prerequisite for that deploy being safe.
- Out of scope: seniority/category from description, skill-vocabulary expansion, any LLM change.
