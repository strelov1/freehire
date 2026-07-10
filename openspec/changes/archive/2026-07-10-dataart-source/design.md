# Design

## Approach

Mirror the `epam` adapter (sitemap → per-vacancy ld+json detail), differing only
where DataArt differs:

- **Host is fixed**, not a board. DataArt is one company at `www.dataart.team`,
  so the adapter is **boardless** (`func (dataart) boardless() {}`) and hardcodes
  the host — no board id in the config entry (like `lumenalta`).
- **Plain sitemap.** `https://www.dataart.team/sitemap.xml` (not gzip); decode
  with `GetXML`.
- **Canonical-URL filter = dedup id.** Keep only
  `https://www.dataart.team/vacancies/{code}` where `{code}` is alphanumeric and
  there is no language prefix. `dataartVacancyCode(url)` returns the code (the
  `ExternalID`) or `""` for the listing root and `/es|ua|pl|bg/vacancies/...`
  localisations, so each vacancy is ingested once — the same trick as epam's
  `_en` filter.
- **Location from `jobLocation`.** Unlike EPAM, DataArt's `JobPosting` carries a
  `jobLocation` array of `Place`; `location()` joins each `"City, Country"`
  (falling back to whichever part is present), deduped, comma-separated.

Everything else reuses the shared helpers: `ldJobPosting`, `fetchDetails` +
`defaultDetailWorkers`, `sanitizeHTML`, `parseDate`, `isRemote`, `joinNonEmpty`.

## Why not the private JSON API

`/dataart-team/api/vacancy/{code}` returns richer structured data, but it is an
undocumented private endpoint; the standard schema.org ld+json is stable, public,
and already has every field we normalize — no reason to couple to the private API.
