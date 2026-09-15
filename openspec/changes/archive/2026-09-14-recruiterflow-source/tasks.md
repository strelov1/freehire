## 1. Listing decode

- [x] 1.1 Fetch `https://recruiterflow.com/<board>/jobs` as raw text and extract
      `window.jobsList = {...};` via `bracketSlice`, decoding the `department` key (an
      array of `[name, items[]]` pairs, needing a small tuple `UnmarshalJSON`) into a flat
      list of items
- [x] 1.2 A listing fetch, extraction, or decode failure fails the whole `Fetch`; an empty
      `department` yields no jobs, not an error

## 2. Detail hydration

- [x] 2.1 For each listing item, fetch its `apply_link`-derived URL
      (`https://recruiterflow.com/<apply_link>`) and decode the page's schema.org
      `application/ld+json` `JobPosting` block via the shared `ldJobPosting` decoder,
      reading only `description`
- [x] 2.2 A failed detail fetch marks only that posting Unreadable
      (`unreadableDetail`/`detailUnreadable`), never the whole board; a page that answers
      but carries no JobPosting block drops the posting
- [x] 2.3 Map `external_id` = the item's `job_id`, title = `job_name`, URL = the derived
      apply link, `company` = the configured `CompanyEntry.Company`, `location` = the
      item's own `details` string verbatim
- [x] 2.4 Map `employment_type` to an employment type (`Full time`→`full_time`, `Part
      time`→`part_time`, `Contract`→`contract`, plus a defensive `Internship`→`internship`
      sibling) and `remote_type` to a work mode via the shared `workplaceTypeMode` helper
- [x] 2.5 `PostedAt` from `last_opened` (RFC3339 with a non-colon numeric offset); no
      `Skills`/`Salary*` mapping (neither field exists in either payload)

## 3. Registration

- [x] 3.1 Implement `fullBoardListing()` and register `recruiterflow` in `sources.All`
- [x] 3.2 No `internal/ingest/atsboard` recognizer entry — `recruiterflow.com` is a bare
      apex domain shared with the platform's own marketing site (confirmed live:
      `/pricing`, `/blog` etc. all answer 200), and no existing recognizer mode can
      require the `/jobs` second segment every real tenant URL carries without a fragile
      marketing-page denylist (see design.md)

## 4. Verification

- [x] 4.1 Write `recruiterflow_test.go` covering: listing enumeration + detail hydration
      (using a realistic multi-department `window.jobsList` fixture), employment-type/
      remote-type mapping, an unreadable detail marking only that posting, a listing
      failure aborting the whole `Fetch`, and an empty board
- [x] 4.2 Run the full adapter test suite plus `atsboard`
- [x] 4.3 `proberFor` gates on `sources.BoardKeyedProviders` (the `boardless` marker),
      unrelated to `atsboard` recognition — `recruiterflow` is still board-keyed and
      needs no bespoke prober (falls back to `adapterProber`), so it is added to
      `TestProberForFallsBackToTheProvidersAdapter` same as every prior adapter

## 5. Close the submission

- [x] 5.1 After merge and deploy, add `recruiterflow/radhires` via `cmd/add-board --apply`
      on prod and delete `board_submissions` id 26, then re-verify live via
      `cmd/ingest recruiterflow` — succeeded on the first live crawl (`ingested=12
      failed=0`), unlike scalis/humanbit/recrutei; the independent review's field-shape
      scrutiny held up this time
