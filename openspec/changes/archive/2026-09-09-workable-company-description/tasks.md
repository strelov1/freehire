## 1. Implementation

- [x] 1.1 Implement `CompanyDescription` on the Workable adapter: fetch `apply.workable.com/api/v1/widget/accounts/{board}` (no `details=true`), return the sanitized `description` field, or `("", nil)` when blank/absent.
- [x] 1.2 Unit-test against fixtures: populated `description`, blank `description`, a fetch error, and that the request omits `details=true` (asserting the requested URL).

## 2. Verification

- [x] 2.1 `gofmt -l .`, `go vet ./...`, `go test ./...` clean.
- [ ] 2.2 After deploy, confirm via `GET /api/v1/companies/{slug}` on a known-filled board from the spike (e.g. `islacare`) that `company_info.summary` is populated once the next scheduled Workable crawl runs.

## 3. Documentation

- [x] 3.1 Update `internal/ingest/sources/AGENTS.md`'s `CompanyDescriber` bullet to name Workable alongside Greenhouse and correct the "empty in every board sampled" claim.
