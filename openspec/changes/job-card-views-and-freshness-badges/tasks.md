## 1. Shared count formatting

- [x] 1.1 Move `formatCount` (and its `trimZero` helper) from
  `web/src/lib/activityChart.ts` to `web/src/lib/utils.ts`, and import it back
  into `activityChart.ts` from its new home. No behaviour change: `697191` still
  renders `697K` and `3354251` still renders `3.4M`.
- [x] 1.2 Add unit tests for `formatCount` in `web/src/lib/utils.test.ts` if the
  move left it uncovered — the card is now a second caller, so the boundaries
  (999, 1000, 99999, 1e6) need pinning.
- [x] 1.3 Verify no other module imported `formatCount` from `activityChart.ts`;
  update any that did.

## 2. The card's reality-requiring badge gate

- [x] 2.1 In `web/src/lib/freshness.test.ts`, write failing tests for a new
  exported wrapper: it returns no badges when the reality signal is absent, and
  delegates to `freshnessBadges` when it is present (fresh 1-day-old job with 0
  applies → both badges; non-fresh → none).
- [x] 2.2 Add the wrapper to `web/src/lib/freshness.ts` with a comment naming why
  the card is stricter than the shared rule (the `/jobs` list and the `Card`
  projection carry no reality signal, so the date would stand alone on exactly
  the postings the gate exists to suppress).
- [x] 2.3 Confirm `freshnessBadges` itself is unchanged and all 12 existing tests
  still pass — the detail page's behaviour must not move.

## 3. The card: view count and glyph swap

- [x] 3.1 In `web/src/lib/components/JobRow.svelte`, derive the view count from
  `'view_count' in job ? job.view_count : undefined` so the `Card` projection
  (which has no such field) degrades rather than throwing.
- [x] 3.2 Render the count in the header rail to the left of the `timeAgo` stamp:
  an `Eye` icon plus `formatCount(views)`, rendered only when the count is above
  zero. Give it an accessible label so the glyph is not the only carrier of
  meaning.
- [x] 3.3 Swap the personal "you have viewed this" marker from `Eye` to `Check`
  (update the import and the `aria-label`), so the rail's eye means the public
  count and nothing else. Update the comment above it explaining the split.
- [ ] 3.4 Verify the rail still respects the `pr-9` save-button gutter and that a
  long company name plus a five-digit count still keeps the rail on one line.

## 4. The card: freshness badges

- [x] 4.1 In `JobRow.svelte`, compute the badges via the new wrapper, passing the
  job's `posted_at`, its `reality` (already derived in the component) and its
  `applied_count` where present.
- [x] 4.2 Render each badge in the existing signal row using `Badge`
  `variant="brand"`, positioned after the reality/ghost chip and before the facet
  tags. Carry each badge's `tooltip` through as its `title`.
- [x] 4.3 Widen the signal row's render guard to include the badges, so a
  badge-earning job with no reality chip, no tags, no countries and no
  credentials still opens the row.
- [ ] 4.4 Check the row's wrapping on a narrow (phone-width) viewport with both
  badges plus several facet chips present.

## 5. Backend: make `view_count` sortable

- [x] 5.1 Add `"view_count"` to `SortableAttributes` in
  `internal/search/search/client.go`, extending the comment above it to say that
  the counter rides the embedded job projection and needs no document change.
- [x] 5.2 Add `"view_count": "view_count"` to `searchSortable` in
  `internal/api/handler/search.go`, with a comment recording that this entry must
  never ship before the live index declares the attribute (an undeclared sort
  attribute makes Meilisearch reject the whole query, which the error mapping
  turns into a 500).
- [x] 5.3 Write/extend a handler test asserting `searchSort` resolves
  `sort=view_count` to `view_count:desc` by default and honours
  `order=asc`.
- [x] 5.4 Extend the search settings test to assert `view_count` is among the
  declared sortable attributes.
- [x] 5.5 Run `gofmt -w` on the touched files, then `go vet ./...` and
  `go test ./...`; run `go vet -tags=integration ./...` before pushing.

## 6. Frontend: the `Most viewed` ordering

- [x] 6.1 In `web/src/lib/facetModel.test.ts`, write failing tests: `views`
  serializes to `sort=view_count`; `sort=view_count` deserializes to `views`;
  `views` survives clearing the query text (unlike `relevance`); `sortOptionsFor`
  offers `views` with and without query text and with and without a profile.
- [x] 6.2 Extend `JobSort` with `'views'`, add the `SORT_PARAM` and `SORT_LABEL`
  entries (`view_count` / `Most viewed`), and include it unconditionally in
  `sortOptionsFor`.
- [x] 6.3 Update the `effectiveSort` collapse rule's comment to note that `views`
  does not collapse when the query empties, and confirm no existing
  `facetModel.test.ts` case regresses.
- [x] 6.4 Confirm the sort control now renders on a signed-out browse with no
  query (two unconditional options), and that the previously-passing
  "no control rendered" expectation is updated in whichever test asserted it.

## 7. Documentation

- [x] 7.1 Add `view_count` to the documented sortable values in
  `web/static/openapi.yaml` (the integration contract) and in
  `web/src/lib/docs/api-spec.ts` / `web/src/lib/docs/filters.ts` if either
  enumerates the accepted `sort` values.
- [x] 7.2 Record the settings-before-binary ordering for this attribute in
  `internal/search/search/AGENTS.md`, beside the existing filterable-attribute
  and embedder hazards, including that a hand patch must send the complete
  `sortableAttributes` list.

## 8. Verification

- [x] 8.1 Run the full web test suite (`pnpm test` in `web/`) and lint; run
  `go test ./...` and `go vet -tags=integration ./...`.
- [x] 8.2 Confirmed against production rather than a local stack (the browse feed
  needs Meilisearch plus a populated catalogue): the served feed HTML carries the
  `Most viewed` option, both badge labels and the screen-reader view count, and
  `sort=view_count` returns `122, 102, 94, 93, 91` descending against `0, 0, 0, 0,
  0` ascending.
- [x] 8.3 Confirmed, and it corrected this plan: the company page is
  **search-backed** (`companies/[slug]/+page.server.ts` calls `searchJobs` with a
  `company_slug` facet), so it carries the reality signal and DOES show badges.
  The surfaces the gate actually bounds are the `Card`-projection ones — tracking
  board, saved and hidden lists, assistant deck — which have no `reality` field.
  The plain `/jobs` list has no caller in the SPA at all.
- [x] 8.4 Job detail page renders and still shows its counts. Not compared pixel
  by pixel; `JobView.svelte` is untouched by this change.

## 9. Rollout

`release.sh` builds the Go binary and the SvelteKit app in ONE blue/green flip, so
9.3 and 9.4 are a single release. 9.2 is therefore a hard gate, not a step to do
"around the same time": if the release lands first, every caller who picks the new
ordering — or opens a shared `?sort=view_count` link — gets a 500 on an
SSR-rendered page, because Meilisearch rejects the whole query for an undeclared
sort attribute.

- [x] 9.1 Queue held one small `documentAdditionOrUpdate` (86 documents) and no
  rebuild, so the settings task would not queue behind hours of work.
- [x] 9.2 Patched the live index with all five attributes at 17:46 UTC on
  2026-09-03 (task 681454). It reported `enqueued`, then `processing` for 91
  seconds while Meilisearch rebuilt the sort data over the catalogue, then
  `succeeded`. Read back: `created_at, enrichment.salary_max,
  enrichment.salary_min, posted_at, view_count` — `posted_at` survived, so the
  default ordering is intact.
- [x] 9.3 The autodeploy timer saw the merge at 17:47 and deferred ("still has 4
  check(s) running"), so the setting was live before any binary carrying the
  handler entry could roll out. The release landed on the following tick;
  `/jobs/search?sort=view_count` now returns 200 with `122, 102, 94, 93, 91`.
- [x] 9.4 Verified on production: the count and badges on a browse card, the
  detail page still rendering its counts, and — see 8.3 — badges on the company
  page too, which corrected this plan rather than confirming it.

## 10. Left undone

- [ ] 10.1 Look at the rendered card. Everything above was verified through served
  HTML and API responses, which prove the markup and the data but not the layout:
  the signal row carrying two brand badges plus a reality chip plus facet chips at
  phone width (tasks 3.4 / 4.4), and the header rail's `pr-9` save-button gutter
  with a five-digit count beside a long company name.
