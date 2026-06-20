## Context

`career.avito.com` is Avito's server-rendered career site (Bitrix). It publishes ~100 open roles
across IT and non-IT functions. Each vacancy lives at `/vacancies/<category>/<id>/` and carries a
complete schema.org `JobPosting` JSON-LD block. The full vacancy list is enumerable from the
public sitemap index `sitemap.xml`, which fans out to per-iblock sub-sitemaps; the vacancy-detail
URLs currently live in `sitemap-iblock-2.xml`. No API key, no headless browser.

freehire already captures Avito opportunistically through the `avito_career` Telegram channel.
This change makes the board a first-class source, reusing the established sitemap-enumerate +
JSON-LD-detail adapter pattern (`radancy`, `successfactors`, `luxoft`, `globalpayments`).

## Goals / Non-Goals

**Goals:**
- A single-company boardless `avito` adapter that enumerates every vacancy from the sitemap and
  yields the normalized job shape under `source = "avito"`.
- Correct remote detection — Avito's JSON-LD reports the HQ city (`Москва`) even for remote roles,
  so the adapter must read the authoritative display city to catch "Удалённая работа".
- Reuse the shared HTTP client, `ldJobPosting`, `sanitizeHTML`, `fetchDetails`, `isRemote`.

**Non-Goals:**
- No new pipeline, write-path, or `cmd/ingest` changes — one adapter file, one registry line, one
  `sources/custom.yml` entry.
- No category/team/SEO landing pages — only `/vacancies/<cat>/<id>/` detail pages.
- No Dockerfile change; the adapter ships in the existing ingest binary.

## Decisions

### Enumeration: traverse the sitemap index, filter by the vacancy-detail URL pattern
Fetch `https://career.avito.com/sitemap.xml` (a `<sitemapindex>`), fetch each sub-sitemap, and keep
every `<loc>` matching `/vacancies/<category>/<id>/` (a numeric trailing id). Dedup by that numeric
id (first-seen wins) before fetching details.

- **Why over hardcoding `sitemap-iblock-2.xml`:** the iblock number is a Bitrix internal that can
  be renumbered; filtering by the URL pattern across all sub-sitemaps is self-correcting and only
  costs ~6 cheap XML fetches. The non-vacancy sub-sitemaps (directions, teams, files) contribute no
  matching locs, and the SEO/directions vacancy sub-sitemaps that repeat a vacancy under a
  different category path collapse via the id-dedup.
- **Why not crawl the HTML listing:** the `/vacancies/` listing page server-renders only a partial
  set and offers no clean pagination contract; the sitemap is the canonical, complete enumeration.

### `external_id` = the numeric vacancy id from the URL path
The JSON-LD `identifier` field holds the **category name** (e.g. "Продажи"), not the id. The stable
per-vacancy identifier is the trailing numeric segment of `/vacancies/<cat>/<id>/`. A loc without a
parseable id is skipped (it would collide on the dedup key). The vacancy page URL is the canonical
`url`.

- **Alternative considered:** JSON-LD `url` — but Avito emits it as `null`, so the fetched page URL
  is the only canonical link.

### Location and remote from the page `<title>`, not JSON-LD `addressLocality`
Avito's `JobPosting.jobLocation.addressLocality` is always the HQ city (`Москва`) even for remote
roles. The authoritative display city is the `<title>` suffix `… в городе <city>` (e.g. "Удалённая
работа", "Москва", "Санкт-Петербург"). The adapter parses that suffix as the location and falls
back to `addressLocality` when the suffix is absent. `Remote = isRemote(location) || isRemote(title)`
— `isRemote` already matches the Russian stem "удал", so "Удалённая работа" classifies correctly.

- **Why:** using `addressLocality` alone would silently mark every remote Avito role as on-site,
  losing the most valuable signal for RU remote seekers.
- **Trade-off:** the `<title>` parse depends on the "в городе" phrasing. If Avito changes it, the
  adapter falls back to the JSON-LD city (degrading remote detection, not breaking ingest).

### Title, description, post date from the JSON-LD `JobPosting`
`title` (the clean role name, without the "Вакансия Авито «…»" wrapper), `description` (HTML, run
through `sanitizeHTML` + `html.UnescapeString` like the other ld+json adapters), `datePosted`
(RFC3339, parsed via `parseRFC3339`/`parseLayout`). `Company` is the static `e.Company` ("Avito").

## Risks / Trade-offs

- **JSON-LD city is the HQ, not the work location** → mitigated by parsing the `<title>` display
  city as primary (above).
- **`<title>` phrasing drift** ("в городе") → mitigated by JSON-LD `addressLocality` fallback;
  ingest never breaks, only remote precision degrades.
- **Sitemap iblock renumbering** → mitigated by pattern-filtering across all sub-sitemaps rather
  than hardcoding the iblock file.
- **Small catalogue (~100 jobs)** → acceptable; the per-provider stale sweep only closes `avito`
  jobs after a successful crawl that ingested ≥1 job, so a transient sitemap outage can't mass-close
  the catalogue.

## Migration Plan

1. Add `internal/sources/avito.go` + table-driven tests with captured fixtures.
2. Register `"avito": NewAvito(client)` in `sources.All` (`internal/sources/source.go`).
3. Add one entry to `sources/custom.yml` (`company: Avito`, `provider: avito`).
4. Ship in the existing ingest image (full rebuild, no Dockerfile change); the existing
   `cmd/ingest sources/custom.yml` cron schedule picks it up.

Rollback: remove the `sources.All` line and the `sources/custom.yml` entry; existing `avito` rows
soft-close via the stale sweep.

## Open Questions

None blocking. Remote-detection precision relies on the `<title>` city parse, validated against the
live page; if future fixtures show a different phrasing, widen the parse.
