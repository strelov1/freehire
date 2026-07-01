## Why

Habr Career (`career.habr.com`) is a major Russian-language IT job board. Today freehire only
captures its vacancies opportunistically — `internal/linksource/habrcareer.go` resolves a Habr
vacancy only when a Telegram post happens to link to it. The board itself is never crawled, so
the catalogue misses ~hundreds of open Russian IT roles that Habr publishes through a clean,
public, keyless JSON API.

## What Changes

- Add a new `habr_career` source adapter that crawls `career.habr.com` through its public
  listing API `GET /api/frontend/vacancies` (paginated), fetching each vacancy's full HTML
  description from its public detail page.
- Register it as a **boardless aggregator** provider (many employers behind one feed), mirroring
  the existing `getmatch`/`tecla`/`jobstash` adapters — one new adapter file, one `sources.All`
  line, one `sources/custom.yml` entry, no pipeline changes.
- Yield vacancies under `source = "habr_career"` with an `external_id`/`url` **identical** to the
  existing linksource adapter, so board-crawled and Telegram-link-followed Habr vacancies dedup
  into one row.
- Extract the shared Habr detail-page description parse so the board adapter and the linksource
  adapter use one helper instead of duplicating it.
- Document a hard coverage ceiling: the public API exposes only ~748 of the ~974 vacancies it
  reports; the remaining ~225 are unreachable by any anonymous channel (verified) and are out of
  scope for v1.

## Capabilities

### New Capabilities
<!-- None. Reuses the source-ingest pipeline and write path unchanged. -->

### Modified Capabilities
- `source-ingest`: add a requirement that `habr_career` is a registered boardless aggregator
  provider — it enumerates Habr Career from the public `/api/frontend/vacancies` JSON feed
  (paginated, per-vacancy employer), fetches each vacancy's full HTML description from its
  `career.habr.com/vacancies/<id>` detail page, and yields the normalized job shape under an
  identity that dedups with the existing Habr linksource adapter.

## Impact

- **New code:** `internal/sources/habrcareer.go` (+ tests with captured fixtures).
- **Modified code:** `internal/sources/source.go` (`sources.All` registry line); a shared
  Habr description-parse helper reused by `internal/linksource/habrcareer.go`.
- **Config:** one new entry in `sources/custom.yml`.
- **Ops:** new adapter ships in the existing server/ingest binaries (full image rebuild, no
  Dockerfile change); relies on the existing `cmd/ingest sources/custom.yml` cron schedule. The
  per-provider stale-job sweep closes `habr_career` jobs unseen for 48h.
- **External dependency:** the public, keyless `career.habr.com` API; `robots.txt` permits
  `/vacancies`.
