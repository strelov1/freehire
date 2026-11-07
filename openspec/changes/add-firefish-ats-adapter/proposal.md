## Why

A visitor submitted a posting hosted on `careers.arnoldclark.com/job/…` (board-submission id
633). Arnold Clark runs its careers site on Firefish (firefishsoftware.com), a UK recruitment
CRM freehire does not crawl at all today: no `atsboard` recognizer, no `internal/ingest/sources`
adapter. There is no JSON API, but every Firefish site — the tenant's own domain AND its
`<tenant>.firefishsoftware.com` spelling — publishes one unpaginated RSS document at
`/rss/adverts/latest.aspx` carrying every open vacancy with its full HTML body and structured
facets (reference number, job type, location area, posted/closing dates, structured pay bounds
with currency and rate). Verified live on 2026-11-07 against `careers.arnoldclark.com`
(342 vacancies, one request) and against the vendor's own `jobs.firefishsoftware.com` (3
vacancies); pages answer a plain client with no edge in front.

## What Changes

- Add a new `firefish` ingest source adapter (`internal/ingest/sources`) that, given a board id
  (the tenant's whole careers host, e.g. `careers.arnoldclark.com`), reads that one RSS document
  and maps every vacancy whole — body, structured facets, salary. One GET per board, no detail
  request, no pagination.
- Add an `atsboard` recognizer so any path on `firefishsoftware.com` subdomain tenants resolves
  to `(firefish, <tenant host>)`, collapsing to the bare host; the vendor's marketing (`www`)
  and careers (`jobs`) hosts are declined. Tenant career sites on their own domains (like
  `careers.arnoldclark.com`) are vanity hosts and stay unrecognised, like every other ATS's.
- Register `firefish` in the provider registry so it appears in the source facet and the
  generated `SOURCE_VALUES`.

## Capabilities

### New Capabilities
- `firefish-source`: crawling Firefish-hosted career sites into the job catalogue via each
  site's public RSS document (one request per board, vacancy normalization, structured
  salary/type/date facets).

### Modified Capabilities
(none — `board-harvest`'s existing requirements already cover "one more recognized platform";
only its recognizer registry gains an entry, no requirement changes)

## Impact

- `internal/ingest/sources/firefish.go` (new adapter) + one registration line in
  `internal/ingest/sources/registry.go`.
- `internal/ingest/atsboard/board.go` (new `modeHost` row + `platformLabelsByApex` entry).
- Generated source-facet values (`contracts.ts` / `SOURCE_VALUES`) regenerated, not hand-edited.
- No schema migration expected — Firefish fits the existing `jobs`/`boards` shapes.
