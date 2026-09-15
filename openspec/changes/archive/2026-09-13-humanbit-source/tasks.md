## 1. Listing decode

- [x] 1.1 Fetch `https://jobs.humanbit.ai/<board>` and decode its flight's `"jobs":[...]`
      array via `flightArray` (confirmed live: `"jobs":` is the only occurrence of that key
      on the page) into each posting's id and the platform's own `org_name` — the listing's
      other per-posting fields (title/location/salary/description) are redundant with the
      detail page's own, richer copy, so they are not decoded at all
- [x] 1.2 A listing fetch or decode failure fails the whole `Fetch`; an empty `jobs` array
      yields no jobs, not an error

## 2. Detail hydration

- [x] 2.1 For each listing entry, fetch `https://jobs.humanbit.ai/<board>/jobs/<id>` and
      decode its flight's `"job":{"id"` object via `bracketSlice`
- [x] 2.2 A failed detail fetch marks only that posting Unreadable
      (`unreadableDetail`/`detailUnreadable`), never the whole board
- [x] 2.3 Resolve the description's `"$<id>"` reference against the DETAIL page's OWN
      flight text rows (verified live: the listing's flight does not carry the same row),
      sanitized
- [x] 2.4 Map `external_id` = the posting's native id, title, location, `company` =
      `firstNonEmpty(listing org_name, configured company)`
- [x] 2.5 Map the detail's `employment_type` array onto `vocab.EmploymentTypeValues`
      (first recognized element wins), `remote` boolean onto `work_mode`/`Remote`
      (`true`→`"remote"`; `false` defers to the location/title heuristic), and `skills`
      verbatim
- [x] 2.6 No `salary`/`salary_period` mapping (see design.md's Non-Goals)

## 3. Registration

- [x] 3.1 Implement `fullBoardListing()` and register `humanbit` in `sources.All`
- [x] 3.2 Add the `jobs.humanbit.ai` → `humanbit` `atsBoards` entry (`path` mode) plus
      `TestRecognize` cases (a posting link, and the bare host with no board)

## 4. Verification

- [x] 4.1 Write `humanbit_test.go` covering: listing enumeration + detail hydration,
      description reference resolution, employment/remote/skills mapping, an unreadable
      detail marking only that posting, a listing failure aborting the whole `Fetch`, and
      an empty board
- [x] 4.2 Run the full adapter test suite plus `atsboard`
- [x] 4.3 Confirm `humanbit` needs no `cmd/harvest-boards` prober entry (falls back to
      `adapterProber`) — add it to `TestProberForFallsBackToTheProvidersAdapter`

## 5. Close the submission

- [x] 5.1 After merge and deploy, add `humanbit/scrabble-jigsaw` via `cmd/add-board --apply`
      on prod and delete `board_submissions` id 177
