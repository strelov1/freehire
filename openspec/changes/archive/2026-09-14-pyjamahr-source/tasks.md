## 1. Listing decode

- [x] 1.1 GET `https://api.pyjamahr.com/api/career/jobs/?company_slug=<board>&page=1` and
      decode `count`/`next`/`results[]` (id, slug, title, min_experience, location,
      workplace_type)
- [x] 1.2 Walk `next` to exhaustion; a page-count safety ceiling reached while `next` is
      still non-empty, a listing fetch failure, or a decode failure all fail the whole
      `Fetch`
- [x] 1.3 An empty `results`/zero `count` yields no jobs, not an error

## 2. Detail hydration

- [x] 2.1 For each listing item, `GET https://api.pyjamahr.com/api/career/jobs/<id>/?
      company_slug=<board>` and decode description, job_type, workplace_type, remote,
      salary fields, skill, min_experience
- [x] 2.2 A failed detail fetch marks only that posting Unreadable
      (`unreadableDetail`/`detailUnreadable`), never the whole board
- [x] 2.3 Map `external_id` = the item's id, title, URL = the item's `public_link`-style
      constructed URL (`jobs.pyjamahr.com/<board>?job_uuid=<slug>`), `company` = the
      configured `CompanyEntry.Company` (neither payload carries a company name),
      `location` = the listing's own `location` string
- [x] 2.4 Map `job_type` to an employment type (`FULLTIME`→`full_time`, `INTERN`→
      `internship`, plus the defensive `PARTTIME`/`PART_TIME`→`part_time` and
      `CONTRACT`/`TEMPORARY`/`FREELANCE`→`contract` siblings)
- [x] 2.5 Map work mode via `workplaceTypeMode` (underscore-normalized) first, falling back
      to `workModeFromRemote`; `Remote` from the bare boolean OR'd with the text heuristic
- [x] 2.6 Map salary only when `is_salary_visible` is true and both bounds are non-null,
      with `salary_type` mapped `ANNUAL`→`year`/`MONTHLY`→`month` (else no salary fields)
- [x] 2.7 Map `skill` through `skilltag.Parse` (joined into one blob first, matching
      `humanbitSkills`'s mining approach) and `min_experience` (truncated) to
      `ExperienceYearsMin`
- [x] 2.8 No `Seniority`/`other_locations` mapping (see design.md's Non-Goals)

## 3. Registration

- [x] 3.1 Implement `fullBoardListing()` and register `pyjamahr` in `sources.All`
- [x] 3.2 Add the `jobs.pyjamahr.com` → `pyjamahr` `atsBoards` entry (`path` mode) plus
      `TestRecognize` cases (a posting link with `?job_uuid=`, and the bare host with no
      board)

## 4. Verification

- [x] 4.1 Write `pyjamahr_test.go` covering: paginated listing enumeration + detail
      hydration, the page-safety-ceiling failure, job-type/workplace-type/salary-visibility
      mapping, an unreadable detail marking only that posting, a listing failure aborting
      the whole `Fetch`, and an empty board
- [x] 4.2 Run the full adapter test suite plus `atsboard`
- [x] 4.3 Confirm `pyjamahr` needs no `cmd/harvest-boards` prober entry (falls back to
      `adapterProber`) — add it to `TestProberForFallsBackToTheProvidersAdapter`

## 5. Close the submission

- [x] 5.1 After merge and deploy, add `pyjamahr/dodo-payments` via `cmd/add-board --apply`
      on prod and delete `board_submissions` id 124, then re-verify live via
      `cmd/ingest pyjamahr` — succeeded on the first live crawl (`ingested=5 failed=0`),
      unlike scalis/humanbit/recrutei in this same initiative; the code-review-caught
      `ExperienceYearsMin` fix held up live
