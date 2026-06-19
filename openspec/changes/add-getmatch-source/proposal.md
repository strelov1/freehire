## Why

[getmatch.ru](https://getmatch.ru/vacancies) is a curated Russian IT job marketplace that
publishes salaries up-front and aggregates ~760 active postings from many employers (X5, Т-Банк,
VK, МегаФон, CIAN, Островок, …). We do not ingest it yet — it is a net-new coverage gap in the
Russian-market segment we already serve (yandex, vk, tbank, sber, telegram).

Its listing is backed by a **public, keyless JSON API**: `GET https://getmatch.ru/api/offers`
returns a paginated (`offset`/`limit`) feed where every offer already carries its own employer,
title, salary, locations (with a structured work-mode per location), and a short description. A
companion detail endpoint `GET https://getmatch.ru/api/offers/{id}` adds the full HTML
description. No headless browser, no private API, no auth (the `/api/vacancies` endpoint that
demands login is a personalized surface — the public feed lives at `/api/offers`).

## What Changes

- Add a `getmatch` source adapter (`internal/sources/getmatch.go`) speaking the existing
  `Source` interface, registered with one `NewGetmatch(c)` line in `sources.All`. Like
  `tecla`/`jobstash` it is a **boardless aggregator** (`boardless()` + `aggregator()`): one
  global feed, the employer comes from each posting, and the `sources/getmatch.yml` entry's
  company is only a validation placeholder.
- **Fetch**: paginate `GET https://getmatch.ru/api/offers?offset=N&limit=100` using
  `meta.total` to stop (with a `maxPages` safety bound, as `tecla`). For each offer, issue a
  detail request `GET https://getmatch.ru/api/offers/{id}` to obtain the full HTML
  `description`, falling back to the list's short `offer_description` when the detail
  description is empty (e.g. one-day-offer event cards).
- **Field mapping** per offer:
  - `ExternalID` = the numeric `id`.
  - `URL` = `https://getmatch.ru` + the relative `url`.
  - `Title` = `position`.
  - `Company` = `company.name`.
  - `Description` = `sanitizeHTML(detail.description)`, falling back to `offer_description`.
  - `Location` = the distinct `location_items[].label` values, joined.
  - `WorkMode` (structured) = derived from `location_items[].format`
    (`remote`→`remote`, `hybrid`→`hybrid`, `office`→`onsite`; the `relocation_*` flags are
    ignored as they are not work modes). Emitted **only when a single distinct work mode** is
    present across the offer's locations; otherwise left empty so the pipeline's location-string
    parser decides (keeping the structured field's provenance clean). `Remote` = `WorkMode ==
    "remote"`.
  - `PostedAt` = `published_at` (zone-less timestamp layout, as `tecla`).
- Include all `offer_type` values (`vacancy` and the handful of `one_day_offer` event cards).
- Add `sources/getmatch.yml` with one boardless placeholder entry, and a cron schedule for
  `cmd/ingest sources/getmatch.yml` (ops, out of this repo's code scope).

## Capabilities

### New Capabilities
<!-- None. Reuses the source-ingest pipeline and write path unchanged. -->

### Modified Capabilities
- `source-ingest`: add a requirement that `getmatch` is a registered boardless aggregator
  provider — it enumerates the getmatch.ru marketplace from the public `/api/offers` feed
  (paginated, per-offer employer), fetches each offer's full HTML description from the
  `/api/offers/{id}` detail endpoint, and yields the normalized job shape with a structured
  work mode derived from the offer's per-location formats.

## Impact

- **New code**: `internal/sources/getmatch.go` + `getmatch_test.go` (inline real-shaped JSON
  fixtures, mirroring `tecla_test.go`); one registration line in `sources.All`.
- **Dependencies**: none new (HTML sanitize + JSON stdlib already present).
- **Config**: one new board file (`sources/getmatch.yml`). No new env vars (keyless).
- **Cron**: one new schedule for `sources/getmatch.yml` — ops change, out of code scope.
- **DB**: none — reuses `UpsertJob` (`source = "getmatch"`, namespaced `external_id`).
- **Out of scope (known seams)**:
  - **Salary** (`salary_display_from`/`_to`/`_currency`) is not mapped — the raw `Job` shape
    carries no salary field; compensation is enrichment's domain.
  - **`skills_objects`** are not mapped — the project derives skills from its own deterministic
    `internal/skilltag` dictionary at ingest.
  - **`StreamingSource`** is not implemented — the ~755 per-offer detail requests are a moderate
    fan-out; the streaming/incremental-persist seam (as `eightfold`) is noted for later if the
    catalogue grows or the crawl gets slow.
