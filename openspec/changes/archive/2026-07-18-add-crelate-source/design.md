## Context

Crelate hosts each customer's public careers page as a candidate portal at
`jobs.crelate.com/portal/<slug>`. The portal SPA reads jobs from a keyless JSON API keyed by an
`OrganizationId` GUID that is embedded in the portal page. One request returns every published
posting, fully populated, so no per-posting detail fetch or pagination is needed.

The ever-jobs recipe (`app.crelate.com/api3/jobs` with header `X-Api-Key: <slug>`) is outdated —
it returns 401 against live portals. The real, validated endpoint is the candidate-portal
`GetAllJobs` call. This was confirmed live against the Career Tree Network portal
(slug `careertree`, OrganizationId `ec546cba-84d5-4d8a-97e5-52e8ef47db08`, 154 jobs).

## Goals / Non-Goals

**Goals:**
- A keyless `crelate` adapter that crawls one portal's published jobs in a single request.
- Map the structured fields the feed provides (location, posted date, per-job employer, human job URL).
- Fit the existing `Source` interface and board-file conventions; no new infrastructure.

**Non-Goals:**
- No salary ingestion (the `Job` contract has no salary field; enrichment derives it).
- No work-mode facet — the feed exposes no structured remote flag; the pipeline's location/text
  dictionaries decide.
- No bulk board discovery in this change beyond seeding one validated board.

## Decisions

- **Endpoint (keyless):** `GET https://jobs.crelate.com/api/candidateportal/GetAllJobs` with a
  URL-encoded `requestEnvelope` query param `{"Locations":null,"OrganizationId":"<GUID>","SearchText":null,"Tags":null}`.
  Response: `{"Jobs":[...],"IsError":bool,"ErrorMessage":string}`. Treat `IsError:true` as a
  board-level failure. All jobs return in one call — no pagination.

- **Board format `<portalSlug>:<organizationId>`.** The GUID keys the API; the slug is needed to
  build the human job URL (the API's `Url` is a relative `/<JobCode>`). Split on the first colon;
  both parts required. Mirrors Bullhorn's `<cls>:<corpToken>` two-part board.

- **Aggregator.** A Crelate portal often fronts a staffing network of client companies (the
  per-posting `CompanyName` varies, e.g. "Career Tree Network (Discovery At Home)"). Mark the
  adapter `aggregator()` and set each `Job.Company` from `CompanyName`, falling back to config.

- **Field mapping (per job):**
  - `Id` (GUID) → `ExternalID` (drop postings with empty Id — dedup-key collision).
  - `Title` → `Title`.
  - Human URL → `https://jobs.crelate.com/portal/<slug>/job/<JobCode>` (JobCode = `Url` without
    the leading slash; falls back to the `JobCode` field).
  - `City`/`State`/`Country` → `Location` ("City, State", country fallback).
  - `LastPostedOnDate` (RFC3339, fractional seconds + Z) → `PostedAt` via `parseRFC3339`.
  - `Description` → `sanitizeHTML` (portals may emit HTML or plain text; sanitize is safe for both).
  - `CompanyName` → `Company` (fallback config).
  - `Compensation`, `Tags`, `SEO`, `SocialUrls` → dropped (no matching structured Job field).

## Risks / Trade-offs

- **OrganizationId discovery is manual** — the GUID is scraped from the portal page, not
  guessable. Acceptable: board files are curated; this change seeds one validated board and
  documents where the GUID lives for future additions.
- **No work-mode / seniority structured signal** — unlike JobScore, Crelate's feed omits these, so
  those facets rely entirely on the dictionaries. Accepted; the location and posted-date signal is
  still structured and reliable.
- **Two-part board is easy to misconfigure** — mitigated by validating both parts are non-empty
  and failing the board (not the run) with a clear error.
