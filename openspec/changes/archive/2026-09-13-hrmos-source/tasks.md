## 1. Link matching

- [x] 1.1 Write a host+path-shape matcher for an HRMOS job link: exactly `/pages/<board>/jobs/<jobID>`
      (the literal `jobs` segment, then the id) — never a looser single-segment match
- [x] 1.2 Confirm a link to the board's own root (`/pages/<board>`, no `/jobs/<id>`) is rejected

## 2. Listing discovery

- [x] 2.1 Page `hrmos.co/pages/<board>/jobs?page=N` via the shared `crawlAllPagedLinks` helper
      to exhaustion
- [x] 2.2 A failure fetching any page — including one after the first — fails the whole
      `Fetch` call

## 3. Job detail

- [x] 3.1 Fetch a job link and decode its `application/ld+json` `JobPosting` block via the
      shared `ldJobPosting` helper (title, description, datePosted, jobLocation, employmentType)
- [x] 3.2 Map to the normalized `Job` shape: `external_id` = the link's final path segment,
      sanitized HTML description, `posted_at` parsed from `datePosted`, `location` assembled
      from the first `jobLocation` entry's address
- [x] 3.3 Map `employmentType` onto `vocab.EmploymentTypeValues` via the closed table
      (case-fold + CONTRACTOR/INTERN rename); leave empty for anything else
- [x] 3.4 A failed job detail fetch marks only that posting Unreadable (`unreadableDetail`),
      never the whole board

## 4. Registration

- [x] 4.1 Implement `fullBoardListing()` and register `hrmos` in `sources.All`
- [x] 4.2 Add the `hrmos.co` → `hrmos` `atsBoards` entry (`path` mode) and the
      `reservedSegments["hrmos.co"] = []string{"pages"}` entry, plus `TestRecognize` cases
      (a job link, and a bare board root with no board)

## 5. Verification

- [x] 5.1 Run the full adapter test suite plus `atsboard`, `boardresolve`, `linksource`,
      `contribution`, `atsdetect`
- [x] 5.2 Confirm `hrmos` needs no `cmd/harvest-boards` prober entry (added to the existing
      `TestProberForFallsBackToTheProvidersAdapter` table)
