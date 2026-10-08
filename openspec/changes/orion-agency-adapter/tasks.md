## 1. Listing + pagination

- [x] 1.1 Write a failing test: given a fixture first-page response (captured shape:
      `items`/`pagination.total`/`pagination.to`), the adapter maps each item to a `Job`
      with Title/Description/Location/PostedAt/ExternalID populated from the listing
      alone.
- [x] 1.2 Implement the response structs (wrapped `.value` fields) and the mapping.
- [x] 1.3 Write a failing test: a fixture two-page crawl (page 1 full, page 2 partial)
      returns all items from both pages.
- [x] 1.4 Implement the pagination loop (stop on empty `items` or `to >= total`, capped
      by `orionMaxPages`).
- [x] 1.5 Write a failing test: a first-page fetch error makes `Fetch` return an error.
- [x] 1.6 Implement that error path.

## 2. Field mapping details

- [x] 2.1 Write a failing test: `Company` is always `"Orion Group"` regardless of
      posting content.
- [x] 2.2 Implement that constant.
- [x] 2.3 Write a failing test: `ExternalID` comes from the item's integer `id`, not
      `slug`/`url`.
- [x] 2.4 Implement that id extraction.
- [x] 2.5 Write a failing test: `postdate.value` (`DD/MM/YYYY`) parses to the correct
      `PostedAt`.
- [x] 2.6 Implement that date parsing (no existing `DD/MM/YYYY` layout found; used
      `parseLayout("02/01/2006", ...)` directly, one-off, not added to dates.go).
- [x] 2.7 (added beyond the original plan) `employment_type` maps onto
      `vocab.EmploymentTypeValues` via `orionEmploymentType`, mirroring
      `recruiterflowEmploymentType`'s shape — `Job.EmploymentType` was otherwise left
      unpopulated despite the data being available in the listing.

## 3. Markers and registration

- [x] 3.1 Add `boardless()` marker method.
- [x] 3.2 Register `orion` in `internal/ingest/sources/registry.go`'s `All()`.

## 4. Quality pass and review

- [ ] 4.1 Run `simplify` over the new/changed files.
- [ ] 4.2 Re-run `go test ./internal/ingest/sources/...` and confirm green.
- [ ] 4.3 Request code review; address Critical/Important feedback.

## 5. Post-merge (manual, not part of the tests)

- [ ] 5.1 After merge and deploy, run a manual crawl and sanity-check the job count
      (~220 expected).
- [ ] 5.2 Add the live board: `cmd/add-board --provider=orion --company='Orion Group'
      --apply` on the prod host.
