## Why

The Profile / experience / parse / fit fixes about to ship in a PR already change behaviour, but coverage is uneven: some handler paths gained unit stubs, frontend flows (Save as project, placeless Profile rendering, fit cold-start Run) and several edge cases lack automated proof. Separately, CI runs unit and integration suites without publishing Go or web **line coverage**, so “do we have coverage?” is answered by grepping tests rather than a report — easy to miss gaps before merge.

## What Changes

- **PR gap-fill tests**: Add or extend unit and `//go:build integration` tests that lock the behaviours in the upcoming PR (SeedHistory on `GET /me/resume`, missing-blob retry → 409, bank projects on Profile composition, attach unplaced atom → project employment, fit analysis idle Run path where unit-testable). Prefer package-local tests; add integration only where the seam needs Postgres/Docker.
- **Coverage tooling**: Make targets (and CI steps) that produce Go coverage profiles for unit and integration runs and Vitest coverage for `web/`, upload or artifact the reports on PRs. **No hard coverage % gate in this change** — report first; a floor is a follow-up once the baseline is visible.
- Document the local commands in the change tasks (and briefly in AGENTS/Makefile help if a target is added) so “check coverage” is one command, not folklore.

## Capabilities

### New Capabilities

- `test-coverage-reporting`: Local + CI generation of Go and web coverage reports (artifacts), without failing the build on a percentage threshold.

### Modified Capabilities

- `resume-structured-profile`: Profile résumé read MUST expose bank jobs and projects via SeedHistory-shaped composition; retry parse MUST surface a clear conflict when the object store key is missing; automated tests MUST cover these paths.
- `experience-bank`: An owner MUST be able to promote an unplaced achievement onto a newly created project employment (create project + attach atom); automated tests MUST cover the service/API path the SPA uses.

## Impact

- `internal/handler` resume / experience tests (unit + possibly integration)
- `internal/resume` missing-object mapping tests (extend if gaps remain)
- `web` Vitest coverage config; optional component tests for MatchAnalysis idle / Experience promote if cheap without full browser
- `.github/workflows/ci.yml`, `Makefile` (cover targets)
- No production API shape change beyond what the PR already ships; this change is tests + tooling
