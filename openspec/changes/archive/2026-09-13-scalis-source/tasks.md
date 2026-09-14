## 1. Listing decode

- [x] 1.1 Decode the `"initialData":{"results"` object out of a page's flight via the
      existing `bracketSlice` primitive, into a `results []scalisPosting` + `count int`
      struct
- [x] 1.2 Page `https://<board>.scalis.ai/jobs?page=N&limit=10&sortBy=SORT_BEST_MATCH`
      (N starting at 1) until a page's `results` is empty; any page fetch/decode failure
      fails the whole `Fetch`

## 2. Job mapping

- [x] 2.1 Resolve `descriptionHtml`'s `"$<id>"` reference via `nextFlightTextRows`,
      sanitized; an unresolved reference degrades to an empty description
- [x] 2.2 Map `external_id` = the posting's native `id`, `title`, `company.name` (fallback
      to the configured company), location from `locations[].city`/`.country`
- [x] 2.3 Map the `employment` enum onto `vocab.EmploymentTypeValues` and the `workplace`
      enum onto `work_mode`; leave empty for an unrecognized value
- [x] 2.4 Map `skills` verbatim and non-null `salary.min`/`.max`/`.currency`, with
      `salary_period` from the `payment` enum (`SALARY`→`year`, `HOURLY`→`hour`)
- [x] 2.5 A posting with no id is dropped (would collide on the dedup key)

## 3. Registration

- [x] 3.1 Implement `fullBoardListing()` and register `scalis` in `sources.All`
- [x] 3.2 Add the `scalis.ai` → `scalis` `atsBoards` entry (`subdomain` mode) plus
      `TestRecognize` cases (a posting link, and the bare tenant root with no posting)

## 4. Verification

- [x] 4.1 Write `scalis_test.go` covering: single-page listing, multi-page pagination to
      exhaustion, a mid-listing page failure aborting the whole `Fetch`, description
      reference resolution, employment/workplace/salary mapping, and an empty board
- [x] 4.2 Run the full adapter test suite plus `atsboard`
- [x] 4.3 Confirm `scalis` needs no `cmd/harvest-boards` prober entry (falls back to
      `adapterProber`) — add it to `TestProberForFallsBackToTheProvidersAdapter`

## 5. Close the submission

- [ ] 5.1 After merge and deploy, add `scalis/boldbusiness` via `cmd/add-board --apply` on
      prod and delete `board_submissions` id 29
