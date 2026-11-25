## 1. Rollup

- [x] 1.1 Failing integration test for the pulse rollup (views vs uniques, exposure, close reasons, existence lifetime, inflow, duplicates/private)
- [x] 1.2 Migration `insights_role_pulse`, lint with squawk
- [x] 1.3 `RebuildInsightsRolePulse` + reads in `insights.sql`, `make sqlc`
- [x] 1.4 Wire the rebuild into `cmd/rollup-stats`

## 2. Endpoint

- [x] 2.1 Failing integration test for `GET /api/v1/insights/role-pulse` (200, 400s, zeros)
- [x] 2.2 Handler + route

## 3. Page

- [x] 3.1 `rolePulse.ts` helper (floors, direction) with vitest tests; `rolesIntro` takes the all-seniority count
- [x] 3.2 API client method and the tiles on `/insights/roles/<category>`

## 4. Verify

- [x] 4.1 gofmt, go vet (incl. integration tags), go test, integration tests
- [x] 4.2 svelte-check, eslint, vitest
