## 1. The Posted pane absorbs the reality facet

Already implemented in the worktree, uncommitted — it is the first commit of this
change because the `filter-modal` and `jobs-list-controls` specs currently forbid it.

- [x] 1.1 Drop the standalone `Posting reality` rail entry from `RAIL`
      (`web/src/lib/filterSections.ts`); record `reality` in `HOSTED_ELSEWHERE` in
      `filterSections.test.ts` and assert it has no rail entry of its own
- [x] 1.2 Render the `reality` ChipFacet in the `posted` pane of
      `FilterModal.svelte`, and include `selCount(f, 'reality')` in the pane's badge
- [x] 1.3 Remove the `Hide evergreen` toggle from `JobsView.svelte` (with its
      `EyeOff`/`signOf` imports) and update the `ListToolbar.svelte` comments that
      name it
- [x] 1.4 Commit: pane + toolbar, tests green

## 2. `created_ts` reaches the index

- [x] 2.1 Add `CreatedTS int64 \`json:"created_ts"\`` to `JobDocument`
      (`internal/search/search/document.go`), documenting it as document-only like
      `PostedTS`, and set it unconditionally in `FromJob` from `j.CreatedAt`
- [x] 2.2 Test: a document built by `FromJob` carries `created_ts` equal to the row's
      `created_at` in unix seconds, and carries it even when `posted_at` is absent
      (where `posted_ts` falls back to the same instant)
- [x] 2.3 Declare `created_ts` in `FilterableAttributes`
      (`internal/search/search/client.go`) with a comment recording the
      settings-before-binary ordering

## 3. The `open_within_days` filter

- [x] 3.1 Test first: `filterFromValues` with `open_within_days=7` emits
      `Gte("created_ts", now-7*86400)` against the injected clock
- [x] 3.2 Test: absent, empty, `0`, negative and non-numeric values each impose no
      restriction
- [x] 3.3 Test: `open_within_days` and `posted_within_days` together emit both bounds
      as a conjunction
- [x] 3.4 Implement the bound in `internal/search/search/query_filter.go`, mirroring
      the `posted_within_days` block
- [x] 3.5 Add `open_within_days` to `scalarFilters`
      (`internal/search/search/query_params.go`) so `UnknownParams` stops reporting a
      working filter as unread; confirm the existing "each scalar filter narrows a
      query" test covers it
- [x] 3.6 `gofmt -w`, `go vet ./...`, `go test ./...`, `go vet -tags=integration ./...`

## 4. The web filter state

- [x] 4.1 Test first: `filtersToParams` writes `open_within_days` and the parser reads
      it back, rejecting zero, negative and non-integer values as `null`
      (`web/src/lib/facetModel.ts`)
- [x] 4.2 Add `openWithinDays` to `JobFilters` + `emptyFilters`, serialize, parse, and
      count it in `activeFilterCount`
- [x] 4.3 Add the setter to `FilterStore` (`filters.ts`) and `StagedFilters`
      (`stagedFilters.svelte.ts`), matching how `postedWithinDays` is written
- [x] 4.4 Add `openWithinEnabled(env)` to `web/src/lib/features.ts` reading
      `PUBLIC_OPEN_WITHIN`, default OFF, with a test for the unparseable-is-off rule

## 5. The controls

- [x] 5.1 Render the `Open within` slider above `Posted within` in the `posted` pane,
      gated on the flag, each labelled for whose date it bounds; include
      `openWithinDays` in the pane's badge count
- [x] 5.2 Point the above-list select at `openWithinDays` and label it `Open` when the
      flag is on; keep it on `postedWithinDays` labelled `Posted` when off
- [x] 5.3 Show the new bound in `FilterSummary.svelte` as its own removable chip,
      distinct from the posted one
- [x] 5.4 Verify in the browser: both bounds, the off-preset stop, and a shared link
      carrying the bound with the flag off

## 6. The contract

- [x] 6.1 Document `open_within_days` in `web/static/openapi.yaml` beside
      `posted_within_days`, stating which date each bounds
- [x] 6.2 Mirror it in `web/src/lib/docs/api-spec.ts`
- [x] 6.3 `pnpm check:links`, and confirm the OpenAPI document still validates
      (the `artifacts` CI job)

## 7. Ship

- [ ] 7.1 Record the deploy order in the PR body: live index settings → binary →
      stop `freehire-reindexw.timer` → full `make reindex` → verify by hand → flag
- [ ] 7.2 PR, CI green, merge, deploy
- [ ] 7.3 Patch the live index settings, then confirm `/api/v1/jobs/facets` still
      answers 200
- [ ] 7.4 Confirm ≥45 GB free, stop the reindex timer, run the full rebuild
- [ ] 7.5 Verify the rebuild POSITIVELY before revealing anything: a posting created in
      the last day must be RETURNED by `?open_within_days=3`, and the hit count must be
      of catalogue scale rather than a handful. Exclusion alone proves nothing — a
      document still missing `created_ts` fails `created_ts >= …` exactly as a genuinely
      old one does, so "the stale posting is gone" is equally consistent with "the
      rebuild never ran"
- [ ] 7.6 Then confirm the negative case too — a posting known to carry a rewritten
      posting date is excluded by `?open_within_days=3` while `?posted_within_days=3`
      still returns it. That pair is the whole point of the change
- [ ] 7.7 Set `PUBLIC_OPEN_WITHIN=1`, restart web, re-enable the reindex timer
