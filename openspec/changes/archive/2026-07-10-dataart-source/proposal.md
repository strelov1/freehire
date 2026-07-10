# DataArt source adapter

## Why

DataArt (dataart.team) is one of the Eastern-roots companies our idagent audit
flagged as uncovered: it runs a custom careers SPA, not a supported ATS, so we
ingest none of its ~136 open vacancies. A feasibility spike found a clean,
keyless enumeration path — `sitemap.xml` lists every vacancy as
`/vacancies/{code}`, and each vacancy page server-renders a standard schema.org
`JobPosting` ld+json block (title, description, `jobLocation` country+city,
`datePosted`, `employmentType`). This is exactly the sitemap-plus-ld+json shape
our EPAM and SuccessFactors adapters already use, so a small adapter closes the
gap. The company slug `dataart` is already in `eastern_roots.txt`, so ingesting
it also lands the `eastern-roots` collection tag automatically.

## What Changes

- Add a `dataart` source adapter: enumerate `https://www.dataart.team/sitemap.xml`,
  keep the canonical English vacancy URLs (`/vacancies/{code}`, excluding the
  listing root and `/xx/vacancies/...` localisations), fetch each page, and map
  its `JobPosting` ld+json to a `Job` with the vacancy `code` as the stable
  `ExternalID`.
- It is a **boardless single-company** adapter (fixed host, one company), like
  `lumenalta` — registered in `sources.All`, excluded from the source facet.
- Add `sources/dataart.yml` (one boardless entry: `DataArt`, `provider: dataart`).

## Non-goals

- No use of DataArt's richer private JSON API (`/dataart-team/api/vacancy/{code}`);
  the standard ld+json is sufficient and matches the existing adapter pattern.
- No new anti-bot/fingerprint transport — the site serves the shared client fine.
- Other uncovered eastern-roots companies remain separate changes.

## Impact

- New: `internal/sources/dataart.go` (+ test), `sources/dataart.yml`,
  `specs/dataart-source`. One line in `sources.All`.
- Ops to land: a cron slot for `cmd/ingest sources/dataart.yml`; the company
  slug `dataart` already sits in `eastern_roots.txt`, so `cmd/import-collections`
  tags it once ingested.
