## 1. Listing decode

- [x] 1.1 POST `https://api.recrutei.com.br/api/v2/vacancies/per-departments/<board>` with
      body `{"search":""}` and decode `data.total`/`data.departments`/`data.vacancies[].
      department`/`total`/`items[]` (id, title, company_name, regime, location, public_link)
- [x] 1.2 Verify `data.total` equals the sum of every department's `items` length; a
      mismatch, a listing fetch failure, or a decode failure all fail the whole `Fetch`
- [x] 1.3 An empty `vacancies`/zero `total` yields no jobs, not an error

## 2. Detail hydration

- [x] 2.1 For each listing item, fetch its `public_link` and decode the page's schema.org
      `application/ld+json` `JobPosting` block via the shared `ldJobPosting` decoder,
      reading only `description` and `datePosted` (a `"DD/MM/YYYY HH:MM:SS"` timestamp)
- [x] 2.2 A failed detail fetch marks only that posting Unreadable
      (`unreadableDetail`/`detailUnreadable`), never the whole board; a page that answers
      but carries no JobPosting block drops the posting
- [x] 2.3 Map `external_id` = the listing item's id, title, URL = `public_link`, `company` =
      `firstNonEmpty(listing company_name, configured company)`, location = the listing's
      own `location` array joined (never the detail page's `jobLocation`, which leaks the
      literal string `"undefined"` — see design.md)
- [x] 2.4 Map the listing's `regime` to an employment type (`CLT`→`full_time`,
      `Pessoa Jurídica`→`contract`, anything else→`""`) — never the detail page's
      ld+json `employmentType`, which is a platform-side constant (see design.md)
- [x] 2.5 `Remote` from the text heuristic only (`isRemote(title + location)`); no
      `WorkMode`, `Skills`, or `Salary*` mapping (see design.md's Non-Goals)

## 3. Registration

- [x] 3.1 Implement `fullBoardListing()` and register `recrutei` in `sources.All`
- [x] 3.2 Add the `jobs.recrutei.com.br` → `recrutei` `atsBoards` entry (`path` mode) plus
      `TestRecognize` cases (a posting link, and the bare host with no board)

## 4. Verification

- [x] 4.1 Write `recrutei_test.go` covering: listing enumeration + detail hydration, the
      completeness check (total vs. summed items) failing the whole board, regime→employment
      type mapping (including the ambiguous/unstated cases), an unreadable detail marking
      only that posting, a listing failure aborting the whole `Fetch`, and an empty board
- [x] 4.2 Run the full adapter test suite plus `atsboard`
- [x] 4.3 Confirm `recrutei` needs no `cmd/harvest-boards` prober entry (falls back to
      `adapterProber`) — add it to `TestProberForFallsBackToTheProvidersAdapter`

## 5. Close the submission

- [x] 5.1 After merge and deploy, add `recrutei/digisystem` via `cmd/add-board --apply` on
      prod and delete `board_submissions` id 115
