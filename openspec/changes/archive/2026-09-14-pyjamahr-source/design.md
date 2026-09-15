## Context

Confirmed live against `jobs.pyjamahr.com/dodo-payments` via headless-browser network
capture (CDP): the platform's frontend calls a completely separate host,
`api.pyjamahr.com` (not `app.pyjamahr.com`, where every earlier static-probing guess
landed on the SPA's own `index.html` shell instead of JSON — the same false-positive-200
trap `jobs.recrutei.com.br` had).

- **Listing**: `GET https://api.pyjamahr.com/api/career/jobs/?company_slug=<board>&page=N`
  — a plain, keyless, standard DRF-style paginated response: `{"count":N,"next":<url-or-
  null>,"previous":...,"results":[...]}`. Each result carries `id`, `slug`, `title`,
  `min_experience`/`max_experience` (numbers), `country`, `location` (an already-formatted
  display string, e.g. `"Bengaluru, Karnataka, India"`), `other_locations` (array, empty on
  every sampled posting), `department_name`, `workplace_type` (`"ON_SITE"` confirmed live;
  `REMOTE`/`HYBRID` not observed but the same self-evident English-enum family), `product`
  — but no description or company name.
- **Detail**: `GET https://api.pyjamahr.com/api/career/jobs/<id>/?company_slug=<board>`
  returns the full object: `description` (rich HTML), `job_type` (`"FULLTIME"`, `"INTERN"`
  confirmed live), `workplace_type` (same field as the listing), `remote` (bool, `false` on
  every sampled posting), `min_salary`/`max_salary`/`currency`/`salary_type`
  (`"ANNUAL"`/`"MONTHLY"` confirmed live) — only meaningful when `is_salary_visible` is
  `true` (both bounds were `null` on every posting where it was `false`), `skill` (already
  a plain lowercase string array — `["stakeholder management", "cross-functional
  collaboration", ...]`, compound phrases, not atomic canonical tokens), `seniority`
  (string array — `["entry-level"]`, `["associate"]` confirmed live), `min_experience`/
  `max_experience` (numbers, whole values on every sample), `created_at` (RFC3339 with a
  numeric zone offset), `valid_through`.
- No `company_name`/employer field anywhere in either payload — a PyjamaHR board is
  single-tenant, so the configured `CompanyEntry.Company` is the only source, the same
  posture the Scalis prod-bug fix already established for a single-tenant board.

## Goals / Non-Goals

**Goals:**
- Crawl a PyjamaHR tenant's open postings via a paginated listing (enumeration +
  location/workplace-type) walked to a proven end (`next` becomes `null`), plus one detail
  fetch per posting for description and the richer structured fields.

**Non-Goals:**
- No `Seniority` mapping. The detail's `seniority` array uses PyjamaHR's own vocabulary
  (`"entry-level"`, `"associate"` confirmed live), neither of which is an unambiguous match
  to `vocab.SeniorityValues` (`intern`/`junior`/`middle`/`senior`/`lead`/`staff`/
  `principal`/`c_level`) — `"associate"` in particular could plausibly read as either junior
  or middle. Guessing a mapping from two ambiguous data points risks being wrong more often
  than leaving it for `internal/dict/classify`'s own title-based inference, so it stays
  empty.
- No `other_locations` mapping. Empty on every sampled posting; nothing observed to design
  the multi-location join against.
- No company-name mapping. Neither payload carries one; `CompanyEntry.Company` is used
  directly for every posting on a board — see Context.

## Decisions

- **`fullBoardListing` applies, with an explicit ceiling failure.** The `next`-URL walk is
  the completeness proof: reaching a null `next` proves the whole board was seen. Unlike
  `manatal.go` (a similar DRF `next`-URL adapter that is deliberately NOT marked
  `fullBoardListing`, because its own page-count ceiling silently stops the walk without
  erroring if ever reached — a live truncation risk this codebase has already been burned
  by once, `teamtailor.ttMaxPages`), this adapter's page-count ceiling is a hard `Fetch`
  failure if `next` is still non-empty when it's reached, the same posture
  `scalis-source`'s page ceiling already established.
- **Employment type comes from the detail's `job_type`** (`FULLTIME`→`full_time`,
  `INTERN`→`internship`), with the same defensive plain-English sibling spellings
  (`PARTTIME`/`PART_TIME`→`part_time`, `CONTRACT`/`TEMPORARY`/`FREELANCE`→`contract`) this
  codebase already gives a self-evident business-vocabulary field even before every
  spelling has a live example (`humanbitEmploymentType`'s own precedent) — unlike
  Recrutei's opaque Portuguese `regime`, which needed a live example per value before
  mapping anything.
- **Work mode comes from `workplace_type` first, falling back to the bare `remote`
  boolean** — `firstNonEmpty(workplaceTypeMode(strings.ReplaceAll(t, "_", "-")),
  workModeFromRemote(remote))`, the exact composition `ashby.go` already uses for the same
  "richer enum over a bare boolean" shape. The shared `workplaceTypeMode` helper already
  handles `on-site`/`onsite`/`on site` after the underscore-to-hyphen normalization
  `gr8people.go`/`getro.go` already apply to their own underscore-separated enums — no new
  mapping code needed.
- **Salary maps only when `is_salary_visible` is true and both bounds are present**;
  `salary_type` maps `ANNUAL`→`year`, `MONTHLY`→`month` (both confirmed spellings in
  `vocab.SalaryPeriodValues`), any other value reports nothing — the same "bounds with
  their unit, or nothing" posture `scalis-source`'s salary mapping already established.
- **Skills are canonicalized through `skilltag.Parse`, not passed through raw** — live
  entries are compound phrases (`"stakeholder management"`, `"cross-functional
  collaboration"`), the same shape `humanbitSkills`/`micro1Skills` already document needing
  mining rather than whole-string matching, even though PyjamaHR's own field already reads
  as plain English words.
- **`ExperienceYearsMin` maps from `min_experience`** (truncated to an int) — a clean,
  unambiguous numeric field, unlike the ambiguous `seniority` string array above.
- **A detail-fetch failure marks only that posting Unreadable, never the whole board** —
  the listing already proves the posting exists and names it; the same posture every other
  listing-then-detail adapter in this package gives (`humanbit`, `recrutei`, `geekhunter`).

## Risks / Trade-offs

- [Only one tenant confirmed (`dodo-payments`, 5 postings), with no live `REMOTE`/`HYBRID`
  `workplace_type` example and no `salary_type` other than `ANNUAL`/`MONTHLY`] → the
  Non-Goals above are deliberately revisitable once a real example exists, not permanent
  exclusions; `workplace_type`/`job_type` reuse this codebase's own established
  self-evident-English-enum defensive-mapping convention rather than waiting for a live
  example of every spelling.
- [`api.pyjamahr.com` is the platform's own internal frontend API, not a documented public
  one] → the same posture this codebase already accepts for `recrutei`'s equivalent
  internal API and for the RSC-flight-based adapters (`scalis`, `humanbit`): it could
  change without notice.

## Migration Plan

No data migration. Ship the adapter, merge, deploy, then add `pyjamahr/dodo-payments` by
hand via `cmd/add-board` to close `board_submissions` id 124.
