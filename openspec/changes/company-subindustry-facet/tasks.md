## 1. Subindustry storage & population

- [x] 1.1 Add the `companies.subindustry TEXT` (nullable) migration; add `Record.Subindustry` to `internal/ycdir` set from `subindustryLeaf(e.Subindustry)` (driven by a `ycdir` unit test), and write it in the `UpsertYCCompany` query + `cmd/import-yc`; regenerate `internal/db` via `make sqlc`

## 2. Subindustry filter on the company list

- [x] 2.1 Add the scalar `subindustries` membership filter (`subindustry = ANY($)`, NULL matches nothing — mirroring `maturity`) to `ListCompanies` and `CountCompanies`, regenerate via `make sqlc`, and parse the `subindustries` param in `internal/handler/companies.go`. Driven by failing integration tests in `internal/db` / `internal/handler` covering the OR-within / AND-across / NULL-excluded scenarios

## 3. Dynamic subindustry facet-values endpoint

- [x] 3.1 Add the distinct-subindustry-with-counts query (`WHERE subindustry IS NOT NULL GROUP BY subindustry ORDER BY count DESC, subindustry`), a `GET /api/v1/companies/subindustries` handler returning `{data:[{value,count}]}`, and its route wiring. Driven by failing handler/integration tests

## 4. Frontend facet

- [x] 4.1 In `web/src/lib/facets.ts` add a searchable `subindustries` facet labelled "Industry" whose options load from `GET /api/v1/companies/subindustries`, and relabel the existing `domains` facet "Industry" → "Domain"; wire the option loading into the companies filter modal. Verify via `svelte-check` + a visual check of the filter

## 5. Verify & rollout

- [x] 5.1 Full verification: `go build ./...`, `go vet ./...`, `go test ./...`, `go test -tags=integration ./internal/db/ ./internal/handler/`, and a manual run of the subindustry filter end-to-end
- [x] 5.2 Record post-deploy ops (apply the `subindustry` migration, then re-run `cmd/import-yc` to populate; no reindex) and offer a `/blog` changelog entry
