## Why

Avito's career site (`career.avito.com`) is a major Russian-language employer board publishing
hundreds of open roles (IT and beyond). Today freehire captures it only opportunistically — the
`avito_career` Telegram channel in `sources/telegram.yml` is crawled and LLM-extracted, so a
vacancy reaches the catalogue only if it happens to be posted there. The board itself is never
crawled, even though every vacancy page exposes a complete, public `JobPosting` JSON-LD object and
the full vacancy list is enumerable from a public sitemap — no headless browser, no API key.

## What Changes

- Add a new `avito` source adapter that crawls `career.avito.com` by enumerating its public
  `sitemap-iblock-2.xml` (vacancy URLs of the form `/vacancies/<category>/<id>/`) and parsing each
  vacancy page's `JobPosting` JSON-LD for title, HTML description, post date, and location.
- Register it as a **single-company boardless** provider (one employer, no per-tenant board id),
  mirroring the existing `ozon`/`luxoft` adapters — one new adapter file, one `sources.All` line,
  one `sources/custom.yml` entry, no pipeline changes.
- Derive `external_id` from the numeric vacancy id in the URL path (the JSON-LD `identifier` field
  holds the category name, not the id) and use the vacancy page URL as the canonical job URL.
- Reuse the established sitemap-enumerate + JSON-LD-detail pattern already proven in
  `luxoft`/`globalpayments`/`habr_career`, including the shared HTTP client and `sanitizeHTML`
  description cleanup.

## Capabilities

### New Capabilities
<!-- None. Reuses the source-ingest pipeline and write path unchanged. -->

### Modified Capabilities
- `source-ingest`: add a requirement that `avito` is a registered single-company boardless
  provider — it enumerates `career.avito.com` from the public `sitemap-iblock-2.xml`, fetches each
  vacancy page, parses its `JobPosting` JSON-LD into the normalized job shape, and yields jobs
  under `source = "avito"` keyed by the numeric vacancy id from the URL.

## Impact

- **New code:** `internal/sources/avito.go` (+ tests with captured fixtures).
- **Modified code:** `internal/sources/source.go` (`sources.All` registry line).
- **Config:** one new entry in `sources/custom.yml` (`company: Avito`, `provider: avito`).
- **Ops:** new adapter ships in the existing server/ingest binaries (full image rebuild, no
  Dockerfile change); relies on the existing `cmd/ingest sources/custom.yml` cron schedule. As a
  non-board source crawled per run, the per-provider stale-job sweep closes `avito` jobs unseen
  for 48h.
- **External dependency:** the public, keyless `career.avito.com` sitemap and vacancy pages.
