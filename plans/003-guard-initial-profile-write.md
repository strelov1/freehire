# Plan 003: Preserve concurrent preferences during initial partial profile updates

## Status
- Priority: P1
- Effort: M
- Risk: MED
- Depends on: none
- Category: bug
- Planned at: commit `9a9c4aead`, 2026-11-11

## Why this matters
A partial MCP profile update computed from an absent row can overwrite a profile concurrently saved on the website. Fields omitted by the request must survive.

## Current state
`internal/api/handler/assistant_profile_write_tool.go:143-178` reads then merges preferences. Its missing-row branch currently calls `Save`; existing-row branch calls `SaveIfUnchanged`. The wrapper at lines 122-128 retries ErrConflict three times.
`internal/platform/db/queries/user_profiles.sql:14-25` uses INSERT ON CONFLICT DO UPDATE replacing all fields. `internal/identity/userprofile/userprofile.go:151-168` normalizes writes through one service. `repository.go` maps pgx no-row to domain ErrConflict for guarded writes.
Use this existing repository/service error convention. Full Save is intentionally replacement and must keep its behavior. sqlc outputs are generated, never edited manually. Read platform/db and identity AGENTS.

## Scope
`internal/api/handler/assistant_profile_write_tool.go`, `assistant_profile_write_tool_test.go`, `me_profile_test.go`;
`internal/identity/userprofile/` implementation and focused unit/integration tests;
`internal/platform/db/queries/user_profiles.sql` and generated sqlc output.
Other test-only Repository implementations may be updated minimally if needed to satisfy the interface (locate all with rg). No migrations, contract, public API, or normalization changes. No new Go package. Reviewer owns plans index.

## Drift and workflow
Run `git diff 9a9c4aead -- internal/api/handler/assistant_profile_write_tool.go internal/identity/userprofile internal/platform/db/queries/user_profiles.sql`. Compare current state on drift. Reuse existing isolated task worktree; no commits/push/merge.

## Steps
1. Read service Repository interface, fake repositories, and `userprofile_integration_test.go`. Add deterministic regression coverage for concurrent initial writers: the first Get observes missing, another writer creates a profile with skills, locations and exclusions, then partial update must retry and retain those unrelated fields.
   Verify: `go test ./internal/api/handler -run TestUpdateProfile -count=1` fails for the intended regression before implementation.
2. Add insert-if-absent query with ON CONFLICT DO NOTHING RETURNING; map no row to existing ErrConflict and normalize via service. Prefer a narrowly named CreateIfAbsent operation, preserving existing replacement API. Route missing-row partial update through it and reuse bounded retry. Update test doubles minimally. Test initial creation, conflict retry, preserved fields, bounded retry exhaustion, non-conflict errors.
   Verify: `make sqlc` exits 0; `go test ./internal/identity/userprofile ./internal/api/handler` passes.
3. Add DB integration coverage modeled after `internal/identity/userprofile/userprofile_integration_test.go`: insert-if-absent must never replace a row and should report conflict. This proves SQL behavior, not just mocks.
   Verify: `mise run db:up`, then `mise exec -- go test -tags=integration ./internal/identity/userprofile` pass. If Docker is unavailable, report that exact limitation and still run tagged vet.
4. Gofmt changed Go paths, run `go vet ./...`, `go test ./...`, and `go vet -tags=integration ./...`. All must pass. Verify generated output is produced by sqlc; `node scripts/check-file-size.mjs --all` passes.

## Done criteria
All runnable commands above pass, meaningful handler race test and DB conflict test exist, full saves retain replacement semantics, nil row creation can never unconditionally replace a competitor. Document unavailable infrastructure explicitly.

## STOP conditions
Report if fixing this needs applied migration changes, layer violations, public contract changes, or more than two attempts cannot resolve a check. Never hand-edit generated files. Don't run production workers.

## Maintenance
Future read-modify-write paths must guard both row absence and row version. Preserve lower-layer isolation and normalized dictionary values.
