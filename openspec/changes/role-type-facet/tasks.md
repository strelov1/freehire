## 1. The dictionary

- [ ] 1.1 Add `RoleTypeValues = []string{"people_manager"}` to `internal/vocab`, and
  extend the existing vocabulary test so the new list is covered like the others.
- [ ] 1.2 Write `internal/roletype/roletype_test.go` first: unambiguous markers
  (`head of`, `director`, `vp`, `vice president`, `chief`, `supervisor`) resolve;
  craft-qualified managers (`engineering manager`, `data manager`, `qa manager`)
  resolve; a plain engineer title, an empty title and a whitespace title resolve to
  `""`. Tests fail.
- [ ] 1.3 Implement `internal/roletype/roletype.go` with `Derive(title) string`,
  whole-word matched via `internal/wordmatch` like its sibling dictionaries. Tests
  pass.
- [ ] 1.4 Add the IC-manager cases to the test — `Product Manager`,
  `Senior Product Manager`, `Project Manager`, `Program Manager`, `Account Manager`,
  `Marketing Manager`, and a bare `Manager` — all resolving to `""`. Tests fail.
- [ ] 1.5 Add the blind-phrase mask, sourcing its vocabulary from the entries
  `internal/classify/dictionaries.go` already curates. Tests pass.
- [ ] 1.6 Add the mask-does-not-shadow case: `Director of Product Management`
  resolves to `people_manager`. Fix the ordering if it fails.
- [ ] 1.7 Add the `Lead` cases: `Tech Lead`, `Lead Software Engineer`, `Team Lead`
  all resolve to `""`, with a comment carrying the 116,893/3,303 measurement so the
  omission reads as deliberate.
- [ ] 1.8 Add a test asserting every value `Derive` can return is in
  `vocab.RoleTypeValues`, mirroring `jobfacts`'s `TestValuesAreInVocabulary`.

## 2. The search index

- [ ] 2.1 Add a `document_test.go` case asserting a job titled
  "Head of Platform Engineering" produces a document whose top-level `role_type` is
  `people_manager`, and that a "Backend Engineer" produces an empty one. Test fails.
- [ ] 2.2 Add the `RoleType` field to `JobDocument` and derive it in `FromJob`
  alongside `Roles` and `AIArchetype`. Test passes.
- [ ] 2.3 Add `role_type` to the filterable-attribute list in
  `internal/search/client.go`, with a comment noting it is index-derived and
  top-level like `roles`.
- [ ] 2.4 Add a `query_filter_test.go` case for `role_type=people_manager` and
  `role_type_exclude=people_manager` producing filters on the bare attribute. Test
  fails, then register the facet in `query_filter.go`'s facet-attribute map and
  `query_params.go`. Test passes.
- [ ] 2.5 `gofmt -w` the touched files, then `go vet ./...` and `go test ./...`.

## 3. Contract and docs

- [ ] 3.1 Declare the `RoleType` parameter in `web/static/openapi.yaml`, referenced
  from every endpoint that takes the string facets, enumerating `people_manager`.
  The description must state that the absence of the value means no marker was
  found and explicitly NOT that the posting is individual-contributor work.
- [ ] 3.2 Add the facet to `web/src/lib/docs/filters.ts` and regenerate
  `docs/API.md` via `gen:api-docs`.

## 4. The SPA control

- [ ] 4.1 Add the `role_type` `FacetDef` to `web/src/lib/facets.ts` with its option
  label, `control: 'pills'`, `excludable: true`. Update the facets test that asserts
  every facet has options/labels if one exists.
- [ ] 4.2 Render `<ChipFacet param="role_type" label="Role type" />` in the
  `kind: 'experience'` pane of `FilterModal.svelte`, between the seniority pills and
  the years control.
- [ ] 4.3 Include `role_type` in the Experience entry's `selCount` so the rail badge
  counts all three controls.
- [ ] 4.4 Update the `filterSections` test that asserts every facet param is
  reachable in the UI, and any filter-modal test asserting the pane's contents.
- [ ] 4.5 Confirm no label anywhere — pill, summary chip, pane copy — describes the
  excluded state as individual-contributor work.

## 5. Verification

- [ ] 5.1 `pnpm test`, `pnpm run check`, and `eslint` on the touched web files.
- [ ] 5.2 `go vet -tags=integration ./...`.
- [ ] 5.3 Drive the app: the Experience pane renders three controls, the role-type
  pill cycles off → include → exclude, and each state reaches the URL as
  `role_type` / `role_type_exclude`.

## 6. Deploy

- [ ] 6.1 Patch `filterable-attributes` on BOTH `jobs` and `jobs_semantic` with the
  current list plus `role_type`, and wait for both Meilisearch tasks to succeed.
  This happens BEFORE the binary flips — otherwise `/api/v1/jobs/facets` hard-500s
  for every caller.
- [ ] 6.2 Deploy via `release.sh`.
- [ ] 6.3 Verify `/api/v1/jobs/facets?facets=role_type` answers 200, and that
  `/api/v1/jobs/facets` without it still answers 200.
- [ ] 6.4 After the next scheduled reindex, check the `people_manager` count against
  the 171,726 measured during design and report the difference.
