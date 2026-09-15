## 1. Listing decode

- [x] 1.1 Fetch `https://jobs.wearestaffy.com/vacantes` and extract every distinct
      `/positions/<slug>` link from `<article class="job-card">` elements via the shared
      `jobLinks` DOM walker, plus the declared total from `.section-kicker-title`
- [x] 1.2 Verify the link count equals the declared total; a mismatch, a listing fetch
      failure, or finding no parseable total at all fails the whole `Fetch`
- [x] 1.3 A declared total of zero (and no links) yields no jobs, not an error

## 2. Detail hydration

- [x] 2.1 For each listing link, fetch the detail page and read its `.metadata` block's
      three `<span>` children (location, work arrangement, seniority) plus the page's
      `<h2>`-delimited prose sections (concatenated into the description)
- [x] 2.2 A failed detail fetch, or a page with no parseable `.metadata` block at all,
      marks only that posting Unreadable (`unreadableDetail`/`detailUnreadable`), never
      the whole board
- [x] 2.3 Map `external_id` = the slug from the posting URL, title = the page's own `<h1>`,
      URL = the detail page URL, `company` = the configured `CompanyEntry.Company`,
      `location` = the first metadata span verbatim
- [x] 2.4 Map the third metadata span to a seniority level, case-insensitively:
      `Junior`/`Jr`→`junior`, `Semi senior`/`Ssr`→`middle`, `Senior`/`Sr`→`senior`,
      `Staff`→`staff`, else `""` (widened after sampling 30+ of the 61 live postings —
      an initial ~5-posting sample missed `Jr`/`Ssr`/`Staff`/the `Semi Senior`
      capitalization variant entirely)
- [x] 2.5 Map the second metadata span to a work mode via a local Spanish-aware prefix
      check (`Remoto`→`remote`, `Hibrido`/`Híbrido`→`hybrid`, else `""`); `Remote` true
      only when the mapped work mode is `remote`
- [x] 2.6 No `Company` (per-posting), `EmploymentType`, `Skills`, or `Salary*` mapping
      (see design.md's Non-Goals)

## 3. Registration

- [x] 3.1 Implement `boardless()` and `fullBoardListing()`, register `staffy` in
      `sources.All`
- [x] 3.2 No `internal/ingest/atsboard` entry (see design.md)

## 4. Verification

- [x] 4.1 Write `staffy_test.go` covering: listing enumeration + detail hydration (using a
      realistic multi-posting fixture), the declared-total-mismatch failure,
      seniority/work-mode mapping (including the abbreviation and case-variant forms), a
      `<ul>`/`<li>` prose section rendering its list items through, a missing-metadata
      page marked Unreadable, an unreadable detail marking only that posting, a listing
      failure aborting the whole `Fetch`, and an empty board
- [x] 4.2 Run the full adapter test suite plus `atsboard`
- [x] 4.3 Confirm `staffy` needs no `cmd/harvest-boards` prober entry — boardless
      providers are refused a prober outright (`TestProberForRefusesBoardlessProviders`);
      add `staffy` to that list

## 5. Close the submission

- [x] 5.1 After merge and deploy, add the `staffy` board (boardless, no `--board` flag)
      via `cmd/add-board --apply` on prod and delete `board_submissions` id 193 (the row
      had been re-inserted with a new id, 195, after an earlier accidental deletion
      during triage — deleted at its actual id), then re-verify live via
      `cmd/ingest staffy` — succeeded cleanly on the first live crawl
      (`ingested=61 failed=0 unreadable=0`, exactly matching the declared total)
