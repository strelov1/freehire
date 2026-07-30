## 1. Normalization

- [x] 1.1 Extend the decorated-title key in `SuppressAggregatorDuplicatesForCompany` to strip a trailing `: …` clause, ordered before the dash/pipe strip so both reduce; keep the two-hash-join UNION ALL shape
- [x] 1.2 Regenerate `internal/db` with `make sqlc`

## 2. Tests

- [x] 2.1 Integration test: an aggregator posting titled `Senior Software Engineer: Full-Stack with TypeScript` is suppressed by an ATS `Senior Software Engineer` at the same company
- [x] 2.2 Integration test: `Senior Software Engineer, Backend (Traffic)` is NOT suppressed by `Senior Software Engineer, Backend (Payments)` — the parenthetical is meaning-bearing
- [x] 2.3 Integration test: `Senior Software Engineer, Backend` is NOT suppressed by `Senior Software Engineer, Fullstack` — the comma clause is meaning-bearing

## 3. Verification

- [x] 3.1 `go build ./... && go vet ./... && gofmt -l . && go test ./...` clean
- [x] 3.2 `go test -tags=integration ./internal/db/` clean
- [ ] 3.3 Re-run the prod count after deploy and confirm the suppressed total rose by roughly the measured +16
