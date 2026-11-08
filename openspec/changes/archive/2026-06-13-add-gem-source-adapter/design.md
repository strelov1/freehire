## Context

`internal/sources` holds one adapter file per ATS platform behind the `Source` interface
(`Provider() string`; `Fetch(ctx, CompanyEntry) ([]Job, error)`), assembled in
`sources.All`. Several adapters whose list endpoint omits the description
(smartrecruiters, rippling, bamboohr) already use the shared `fetchDetails` bounded-pool
helper to fan out per-posting detail requests. Gem fits this exact shape.

The Gem contract was confirmed live against the `go-cadre` board:

- Endpoint: `POST https://jobs.gem.com/api/public/graphql` — public profile, no auth, no
  Cloudflare. Body is standard GraphQL `{operationName, variables, query}`.
- List `JobBoardList(boardId)` → `data.oatsExternalJobPostings.jobPostings[]`, each with
  `extId`, `title`, `locations[]{name, city, isoCountry, isRemote}`, and
  `job{locationType, employmentType}`. No description. No pagination arguments.
- Detail `ExternalJobPostingQuery(boardId, extId)` →
  `data.oatsExternalJobPosting{descriptionHtml, firstPublishedTsSec, startDateTs,
  companyUrl, …}`.
- `boardId` is the board's **vanity path** (`go-cadre`), confirmed by the schema field
  `jobBoardExternal(vanityUrlPath: $boardId)`.

## Goals / Non-Goals

**Goals:**

- A `gem` adapter that lists a board and yields normalized jobs with sanitized-HTML
  descriptions, reusing the existing list→detail pattern and helpers.
- One-line registration in `sources.All`; one `sources.yml` entry to onboard a board.

**Non-Goals:**

- Pagination of `JobBoardList` (the operation exposes no page arguments; Gem boards are
  single-company and small). Deferred until an observed board truncates.
- Any change to the pipeline, write path, config, or schema.
- The authenticated `/api/graphql` (APP) profile — only the public profile is used.

## Decisions

- **Two GraphQL operations over `PostJSON`.** `Fetch` issues `JobBoardList` once, then
  `fetchDetails(postings, gemDetailWorkers, …)` runs `ExternalJobPostingQuery` per posting
  under a bounded pool — identical isolation to smartrecruiters/rippling/bamboohr. A failed
  detail drops only that posting.
- **Query text is embedded as Go string constants**, sending the minimal field set the
  adapter consumes (not the browser's full selection set). Variables are
  `{boardId: e.Board, extId: posting.extId}`.
- **Job mapping:**
  - `ExternalID` = `extId` (the pipeline namespaces it as `"<board>:<extId>"`).
  - `URL` = `https://jobs.gem.com/<board>/<extId>`.
  - `Title` = `title`; `Company` = `e.Company`.
  - `Location` = `joinNonEmpty(city, isoCountry)` of the first location (existing helper).
  - `Description` = `sanitizeHTML(descriptionHtml)`.
  - `Remote` = the first location's `isRemote` **OR** `job.locationType == "REMOTE"` —
    Gem exposes structured flags, more reliable than the free-text `isRemote()` heuristic.
  - `PostedAt` = `parseEpochSeconds(firstPublishedTsSec)` — a new helper beside
    `parseEpochMillis`, returning nil for a zero/absent value. `firstPublishedTsSec` is
    chosen over `startDateTs` because it is a stable integer first-publication date, where
    `startDateTs` is a float that can shift.
- **No description in the list is the reason for the detail fetch**, matching the spec's
  existing "fetches detail when the list lacks the description" requirement.

## Risks / Trade-offs

- **Unpaginated list** — if a Gem board ever exceeds one page, jobs silently truncate.
  Mitigation: single-company boards are small (observed: 8); revisit only on evidence.
- **GraphQL schema drift** — Gem could rename fields. Mitigation: the adapter requests a
  minimal field set; a missing field surfaces as an empty value, and the board still
  ingests. Covered by table-driven tests over canned JSON responses, the same way existing
  adapters are tested (no live network in unit tests).
- **Public endpoint stability** — relies on the unauthenticated public profile remaining
  open; if Gem gates it, the adapter would need the same fetch-infra decision deferred for
  Dayforce. Out of scope here.
