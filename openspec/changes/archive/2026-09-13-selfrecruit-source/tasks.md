## 1. Listing discovery

- [x] 1.1 Write a posting-link matcher for a selfrecruit.ge listing page: a bare-root
      `/<uuid>` path (never the unrelated `/articles/<uuid>` CMS-content links the same
      site also carries)
- [x] 1.2 Page `https://<board>.selfrecruit.ge/vacancies/<offset>` (offset in steps of 10,
      starting at 0) via the shared `crawlAllPagedLinks` helper to exhaustion — no special
      case for the first page, since `/vacancies/0` is confirmed identical to the bare
      tenant root

## 2. Job detail

- [x] 2.1 Extract the title from the `vacancy_title_inner`-classed element via the shared
      `firstByClass` + `textContent` helpers
- [x] 2.2 Extract the description from the `pub_vac_text_detail`-classed element via
      `firstByClass` + `innerHTML`, sanitized through `sanitizeHTML`
- [x] 2.3 Map to the normalized `Job` shape: `external_id` = the link's UUID path segment,
      `Location` left empty (not exposed structurally; enrichment derives it from the
      description like `successfactors`)
- [x] 2.4 A failed job detail fetch marks only that posting Unreadable
      (`unreadableDetail`/`detailUnreadable`), never the whole board

## 3. Registration

- [x] 3.1 Implement `fullBoardListing()` and register `selfrecruit` in `sources.All`
- [x] 3.2 Add the `selfrecruit.ge` → `selfrecruit` `atsBoards` entry (`subdomain` mode) in
      `internal/ingest/atsboard/board.go`, plus `TestRecognize` cases (a posting link, and
      the bare tenant root with no posting)

## 4. Verification

- [x] 4.1 Write `selfrecruit_test.go` covering: single-page listing→link collection,
      multi-page pagination to exhaustion, a mid-listing page failure aborting the whole
      `Fetch`, detail extraction (title/description), UUID `external_id` derivation,
      unreadable-detail marking, and an empty-listing board returning no jobs without
      error
- [x] 4.2 Run the full adapter test suite plus `atsboard`
- [x] 4.3 Confirm `selfrecruit` needs no `cmd/harvest-boards` prober entry (falls back to
      `adapterProber`, per the design's Non-Goals) — add it to
      `TestProberForFallsBackToTheProvidersAdapter`

## 5. Close the submission

- [ ] 5.1 After merge and deploy, add `selfrecruit/dressup` via `cmd/add-board --apply` on
      prod and delete `board_submissions` id 8
