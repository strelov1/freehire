## Why

freehire has no coverage of the EURES portal (europa.eu/eures), the EU's own cross-border public
employment service aggregating job vacancies from all 31 EU/EFTA countries' national employment
services (PES) and partner job boards. It is a keyless, well-documented public API and a natural
next multi-country aggregator alongside `arbeitsagentur`/`trudvsem`/`whatjobs`.

## What Changes

- New `internal/ingest/sources` adapter, provider key `eures`, for the EURES job search API
  (`https://europa.eu/eures/api/jv-searchengine`). Keyless, registered unconditionally in
  `sources.All`.
- Board = ISO 3166-1 alpha-2 country code (lowercase), one of the ~31 EURES-covered countries.
  Each board is added separately via `cmd/add-board`; this change only makes the provider
  crawlable, it does not itself populate boards for every country.
- IT scope is fixed in adapter code, not board-selectable: every crawl filters the search request
  by the ESCO/ISCO occupation-group URIs for "ICT professionals" (`C25`) and "ICT technicians"
  (`C35`) — verified live that this correctly narrows results to technical roles. The adapter sets
  `Job.IsTechHint = true` on every posting it yields, the same contract `profession.hu` uses for a
  source whose crawl scope itself guarantees technical relevance.
- Freshness window is fixed in code at `publicationPeriod: LAST_THREE_DAYS`, sorted
  `MOST_RECENT`, to stay within the API's measured ~10,000-result pagination depth cap even for
  the largest observed country/occupation-group combination (Germany + `C25`, measured at ~6,100
  records under this window vs. ~10,500 for `LAST_WEEK`, which does not fit). A hard page-count
  backstop keeps the adapter from ever issuing a request past the depth cap, which errors outright
  rather than degrading. If a run still hits the cap for some country, only the oldest tail of that
  window is missed for that run — it is picked up on the next run, well inside the 3-day window,
  since ingest runs far more often than every 3 days (the same self-healing reasoning as
  `arbeitsagentur`'s own recency-window bound).
- **The adapter carries the `aggregator` marker.** Live verification found EURES re-publishes
  postings sourced from national PES systems and partner boards rather than hosting first-party
  content: a German result's `source` field was `DE001` and its `applicationInstructions` linked
  directly to `arbeitsagentur.de/jobsuche/jobdetail/<same reference number>` already reachable
  through freehire's own `arbeitsagentur` adapter; other observed `source` values were
  `PES-SWEDEN`, `PSZ` (Poland's PES) and `JOBTHATMAKESENS` (a private French job board). This is
  the same shape `whatjobs`/`adzuna` are marked for, so the existing cross-source dedup pass can
  suppress an EURES copy wherever a first-party or more-authoritative source already carries the
  same posting, while still keeping EURES postings for countries/employers nothing else reaches.
- Listing enumeration reads the search endpoint (`POST .../jv-search/search`), which already
  returns each posting's full HTML description translated to `requestLanguage` when a translation
  exists — no per-posting fetch needed for title/company/description. A separate per-posting
  detail fetch (`GET .../jv/id/{id}`, via the existing `fetchDetails` helper) is used ONLY to
  enrich `Location`: the search response's `locationMap` carries opaque NUTS codes only, while the
  detail response's `locations[]` carries real city names/addresses. A failed or missing detail
  fetch does not drop the posting — it falls back to a plain country name, mirroring
  `arbeitsagentur`'s "missing description doesn't drop the posting" precedent.
- `Job.URL` is the EURES portal's own detail page
  (`https://europa.eu/eures/portal/jv-se/jv-details/{id}?lang=en`), not a URL scraped out of
  `applicationInstructions` — that field is free-text prose that varies by country/language and is
  sometimes entirely URL-less (Poland's is literally "directly to employer"), too unreliable to
  parse across 31 locales for a first version.
- `Job.Countries` is set from the search result's `locationMap` keys via
  `internal/dict/location.NormalizeCountry` (structured signal). `Job.EmploymentType` is set from
  the search result's `positionOfferingCode`/`positionScheduleCodes` where they map unambiguously
  onto freehire's vocabulary (`internship`/`contract` from the offering code, else
  `full_time`/`part_time` from the schedule code); `WorkMode`/`Seniority`/`Category`/`Skills` are
  left unset — the API exposes no structured remote/work-mode flag, so the pipeline's own
  dictionaries decide those.
- One line added to `sources.All` (`registry.go`) registering `NewEures(c)`.

## Capabilities

### New Capabilities
- `eures-source`: crawling one EURES-covered country's board through the EURES public search +
  detail APIs, scoped to ICT occupations, marked as an aggregator for cross-source dedup.

### Modified Capabilities
(none)

## Impact

- `internal/ingest/sources/eures.go` (+ `_test.go`): new adapter.
- `internal/ingest/sources/registry.go`: one new `NewEures(c)` line in the keyless multi-company
  aggregator section (alongside `arbeitsagentur`, `trudvsem`).
- No schema change, no new env var, no worker change, no new board files (boards are added
  separately via `cmd/add-board`, out of scope for this change).
