## 1. Category vocabulary & partition

- [x] 1.1 Add the 10 new values to `CategoryValues` in `internal/enrich/enrichment.go`
- [x] 1.2 Place `business_analysis`, `solutions_engineering`, `developer_relations`, `technical_writing` in `TechCategories`
- [x] 1.3 Place `recruiting`, `hr`, `finance`, `legal`, `operations`, `customer_success` in `NonTechCategories`
- [x] 1.4 Confirm `techcategories_test.go` partition test is green (union == CategoryValues, disjoint)

## 2. Title-alias dictionary (ordering is load-bearing)

- [x] 2.1 Add `business_analysis` aliases (EN+RU) ABOVE the terminal `{"analyst","data_analytics"}` fall-through
- [x] 2.2 Add BI aliases into the existing `data_analytics` block; add RevOps/Sales Ops into the existing `sales` block
- [x] 2.3 Add `finance` + `legal` + `operations` aliases ABOVE the terminal `{"manager","management"}` and `{"analyst",…}` fall-throughs
- [x] 2.4 Add `recruiting` + `hr` aliases
- [x] 2.5 Split `customer_success` out of `support` (retarget `customer success` and add its aliases); leave `customer service`/`help desk` on `support`
- [x] 2.6 Add `solutions_engineering` aliases with `sales engineer` ABOVE bare `{"sales","sales"}`
- [x] 2.7 Add `developer_relations` aliases (anchor `developer/technical evangelist`)
- [x] 2.8 Add `technical_writing` aliases with `ux writer`/`content designer` ABOVE `{"ux",…}`/`{"designer",…}`; keep `copywriter`/`content writer` on `marketing`
- [x] 2.9 Verify `csm` is NOT added and no bare `operations`/`ops`/`controller`/`evangelist`/`analyst`/`manager` alias was introduced

## 3. Classification tests

- [x] 3.1 Add classify tests: each new-role title resolves to its category (10 roles)
- [x] 3.2 Add fall-through-guard tests: financial/business analyst not stolen by `analyst`; operations/finance manager not stolen by `manager`; sales engineer beats sales; ux writer/content designer beat ux/designer
- [x] 3.3 Add customer_success-vs-support split test

## 4. Skill dictionary

- [x] 4.1 Add the new-role skill aliases to `internal/skilltag` (per design.md skill lists)
- [x] 4.2 Add skilltag tests for a representative skill per new role; confirm unknown terms still emit nothing

## 5. Domain vocabulary & prompt

- [x] 5.1 Edit `DomainValues` in `internal/enrich/enrichment.go`: remove `saas`; add `devtools`, `cybersecurity`, `ai`, `hrtech`, `proptech`, `mobility`, `climatetech`
- [x] 5.2 Add the per-domain one-line gloss to the enrichment prompt in `internal/enrich/langchain.go` (with the `ai` core-product scoping)
- [x] 5.3 Decide + implement the `saas` historical-row handling (silent drop via `keepKnown` vs one-off remap)

## 6. Frontend labels & contracts

- [x] 6.1 Add `CATEGORY_LABELS` entries for the 10 new categories in `web/src/lib/labels.ts`
- [x] 6.2 Update `DOMAIN_LABELS` (drop `saas`, add the 7 new domains) in `web/src/lib/labels.ts`
- [x] 6.3 Regenerate `web/src/lib/generated/contracts.ts` (`cmd/gen-contracts`) and confirm category/domain filter option lists pick up the new values

## 7. Re-derive existing data

- [x] 7.1 `go build ./... && go vet ./... && go test ./...` all green
- [ ] 7.2 Run `go run ./cmd/backfill-derive` to re-classify existing rows on the expanded dictionaries
- [ ] 7.3 `make reindex` to re-index the re-derived categories/skills
- [ ] 7.4 (Optional/ops) trigger re-enrichment so historical `domains` pick up the revised vocabulary

## 8. OpenSpec validation

- [x] 8.1 `openspec validate expand-role-taxonomy --strict` passes
