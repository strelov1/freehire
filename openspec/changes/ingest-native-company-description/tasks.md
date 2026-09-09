## 1. Adapter contract

- [x] 1.1 Add the `CompanyDescriber` optional interface to `internal/ingest/sources` (`CompanyDescription(ctx, CompanyEntry) (string, error)`).
- [x] 1.2 Document the contract addition alongside the existing `Source` interface docs (no per-posting duplication; called once per board).

## 2. Greenhouse adapter

- [x] 2.1 Implement `CompanyDescription` on the Greenhouse adapter: fetch `https://boards-api.greenhouse.io/v1/boards/{board}` (distinct from the `/jobs` endpoint), return the sanitized `content` field, or `("", nil)` when blank/absent.
- [x] 2.2 Reuse the existing `sanitizeHTML` helper on the returned text.
- [x] 2.3 Unit-test against fixtures: populated `content`, blank `content`, and a 404 board (treated the same as "no description", not a hard failure of the whole board crawl).

## 3. Pipeline wiring

- [ ] 3.1 After a board's `Fetch` succeeds, type-assert the adapter for `CompanyDescriber` and call it once; on error, log and continue (a failed company-description fetch SHALL NOT fail the board's job ingest).
- [ ] 3.2 Add the `FillCompanyDescriptionFromIngest`-shaped query (final name at implementation time) to `internal/platform/db/queries/companies.sql`: `INSERT ... ON CONFLICT (slug) DO UPDATE` touching only `tagline` (gap-fill), `company_info` (key-merge, gap-fill), `company_info_at`; `is_reference = false` on insert.
- [ ] 3.3 Run `make sqlc` and confirm the generated Go has no unrelated diff.
- [ ] 3.4 Call the new write from the pipeline step added in 3.1, keyed by the board's company slug.

## 4. Verification

- [ ] 4.1 Integration test (`-tags=integration`): a board whose adapter returns a company description fills `tagline`/`company_info` for a new company; a company that already has a `tagline` from any source is unchanged; a board whose adapter doesn't implement `CompanyDescriber` (or returns empty) makes no company-info write.
- [ ] 4.2 `gofmt -l .`, `go vet ./...`, `go test ./...`, `go vet -tags=integration ./...` all clean.
- [ ] 4.3 Run `go run ./cmd/ingest greenhouse` (or the relevant provider invocation) against a small set of known-content boards (e.g. `coinbase`, `figma`, `asana`) and confirm via `GET /api/v1/companies/{slug}` that `tagline` is now populated.

## 5. Documentation

- [ ] 5.1 Note the new optional adapter capability in `internal/ingest/sources/AGENTS.md`.
- [ ] 5.2 Note the new ingest-time company-info source in `openspec/specs/company-info/spec.md`'s eventual archived context (handled by `opsx:sync`/archive, not hand-edited now).
