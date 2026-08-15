Detailed steps, test bodies and code for every task live in
`docs/superpowers/plans/2026-08-15-industrytag.md`. This list is the tracked
breakdown; that plan is the how.

## 1. The industry dictionary

- [x] 1.1 Create `internal/industrytag` with `Canonicalize([]string) []string`: normalize case, separators and `&`/`and` to one lookup key, resolve through the alias map, pass already-canonical values through, return sorted and de-duplicated
- [x] 1.2 Write out the canonical vocabulary explicitly (74 values) and generate its aliases from observed labels (213), hand-checking acronym casing
- [x] 1.3 Add dictionary invariant tests: every canonical is a well-formed slug, every alias target exists, every alias key is in normal form, every canonical has a non-empty label
- [x] 1.4 Add resolution tests: separator variants collapse, curated synonyms collapse, unknown labels emit nothing, canonical values are idempotent, blank input is safe

## 2. Cooperative writes

- [x] 2.1 Add a failing integration test asserting `UpsertYCCompany` preserves an existing tagline, merges `company_info` keys without overwriting, and unions industries
- [x] 2.2 Change `UpsertYCCompany` to `COALESCE(NULLIF(...))` for tagline, `EXCLUDED.company_info || companies.company_info` for JSONB, and a sorted de-duplicated union for industries; regenerate with `make sqlc`
- [x] 2.3 Route `cmd/import-yc`'s mapped industries through `industrytag.Canonicalize`
- [x] 2.4 Update the query's leading comment, which currently claims company-info columns are refreshed

## 3. The normalization and merge worker

- [ ] 3.1 Add `ListCompanyIndustriesPage` (keyset, unfiltered — the merge pass must reach companies with no industries) and `SetCompanyIndustries` (guarded by `IS DISTINCT FROM` so a re-run reports no churn); regenerate with `make sqlc`
- [ ] 3.2 Add a failing test for the dump parser: each record indexed under both its own slug and a slug derived from its name, records whose every tag is unknown dropped entirely
- [ ] 3.3 Implement `cmd/import-company-industries` following the `worker.Bootstrap` convention: normalization pass, optional merge pass, non-zero exit on failure
- [ ] 3.4 Implement the dropped-label report: distinct count, total occurrences, and the most frequent unrecognized labels
- [ ] 3.5 Back up `companies.industries` on production, run the normalization pass, read the report, extend the dictionary and re-run

## 4. The facet

- [x] 4.1 Add failing tests asserting `industries` appears in the company index's filterable attributes and in the facet param map
- [x] 4.2 Add `industries` to `FilterableAttributes` and `companyFacets`, and accept the `industries` query parameter on `/companies`
- [x] 4.3 Emit `INDUSTRY_VALUES`/`INDUSTRY_LABELS` from `cmd/gen-contracts` and regenerate the web contracts, so the UI options derive from the Go dictionary rather than a hand-typed copy
- [x] 4.4 Add the industry filter to the companies page, and rename the YC-only `subindustries` facet to "YC industry" so the plain label goes to the facet that covers the catalogue
- [ ] 4.5 Verify in a browser that selecting an industry puts it in the URL and changes the result count (needs real data — do it with 5.3, after the worker has run on prod)

## 5. Rollout

- [ ] 5.1 Merge the external company dump through the worker
- [ ] 5.2 Reindex companies, first confirming no other reindex is in flight
- [ ] 5.3 Verify the facet answers on production and the counts look sane
