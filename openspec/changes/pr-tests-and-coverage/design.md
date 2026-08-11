## Context

See proposal.md for motivation. Today CI runs `go test ./...` and `go test -tags=integration ./...` plus `pnpm run test` in `web/` with **no** `-cover` / Vitest coverage step. The Profile PR already ships SeedHistory on `GET /me/resume`, missing-blob → `ErrNotStored` / 409, and SPA Save-as-project (create employment + update atom). Gap-fill must prove those seams; tooling must make coverage visible without inventing a floor.

Constraints: keep CI time reasonable; integration coverage needs Docker/testcontainers like today’s tagged suite; do not store secrets in artifacts; AGENTS.md already mandates `go vet -tags=integration` before push — coverage sits beside that, not instead of it.

## Goals / Non-Goals

**Goals:**

- Close automated-test gaps for the PR behaviours named in the delta specs.
- Ship Make/pnpm targets and CI artifact uploads for Go + Vitest coverage.
- Keep coverage **informational** (artifacts + local HTML), not a failing threshold.

**Non-Goals:**

- Repo-wide coverage floors or Codecov/Sonar gates.
- Playwright/e2e for Save-as-project or Profile (optional later; prefer handler/store unit tests).
- Changing production LLM / Bedrock quota behaviour.
- Committing generated coverage HTML into git.

## Decisions

1. **Gap-fill in Go first, SPA second.**  
   SeedHistory on GetResume, missing-blob mapping, and create-project-then-attach are fully expressible with existing handler fakes / experience store tests. Vitest for MatchAnalysis idle / ResumeStructuredView placeless is optional if cheap; do not block the change on component harness work.

2. **Go coverage: two profiles.**  
   - Unit: `go test ./... -coverprofile=coverage-unit.out -covermode=atomic`  
   - Integration: `go test -tags=integration ./... -coverprofile=coverage-integration.out -covermode=atomic` (CI only or `make cover-integration` with Docker).  
   Expose `make cover` / `make cover-html` for unit by default. Alternatives considered: single combined profile — rejected because unit and integration compile different files and muddy the report.

3. **Web coverage: Vitest `@vitest/coverage-v8`.**  
   Add as a web (or root) package for `pnpm run test:coverage` writing `web/coverage/`. Alternatives: istanbul — V8 is the Vitest default and lighter.

4. **CI: upload artifacts, no fail-on-%.**  
   Use `actions/upload-artifact` for `coverage-*.out` / `web/coverage`. Do not add a minimum % check in this change. Alternatives: Codecov comment bot — deferred until the team wants PR comments.

5. **Integration tests for attach.**  
   Prefer extending `internal/experience` store tests (fake or integration-tagged) over new HTTP integration unless the handler JSON path is the only untested seam. The “invalid attach” scenario already has store coverage patterns (`TestStoreAtomCannotAttachToAnotherOwnersEmployment`).

## Risks / Trade-offs

- **[Risk] Integration coverage doubles backend CI time** → Mitigation: keep integration cover as a separate Make target; in CI run it only if the job already runs integration tests (same job, extra flag), or skip HTML generation in CI and keep raw profiles only.
- **[Risk] Coverage noise from generated `internal/db`** → Mitigation: optional `-coverpkg` scoped to `./internal/...` excluding generated packages if reports are unreadable; decide when first HTML is reviewed.
- **[Risk] Vitest coverage deps grow `web/node_modules`** → Mitigation: add `@vitest/coverage-v8` as a web `devDependency` only.
- **[Trade-off] No % gate** → Gaps can still merge; the PR gap-fill tasks are the hard requirement for the named behaviours.

## Migration Plan

1. Land gap-fill tests with the feature PR (or immediately after) so CI green proves the specs.
2. Land Make/pnpm/CI coverage in the same or follow-up commit; artifacts appear on the next PR run.
3. Rollback: remove CI steps / Make targets; no schema or runtime rollback.

## Open Questions

- Whether to exclude `internal/db` generated packages from `-coverpkg` after the first report is eyeballed (does not change specs).
