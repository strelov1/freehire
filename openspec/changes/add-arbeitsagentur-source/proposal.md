## Why

The Bundesagentur für Arbeit (Germany's federal employment agency) runs the country's largest job
board. A spike VALIDATED that its `jobsuche-service` REST API is reachable keyless with the
well-known public `X-API-Key: jobboerse-jobsuche` and enumerates postings by professional field
(`berufsfeld`), returning employer, title, location, publish date, and a stable `refnr`. Adding a
first-party IT slice widens German-market coverage — a market we otherwise reach only indirectly.

## What Changes

- Add an `arbeitsagentur` source adapter that queries the search API
  (`https://rest.arbeitsagentur.de/jobboerse/jobsuche-service/pc/v4/jobs`) with the static public
  `X-API-Key: jobboerse-jobsuche` header, filtered by `berufsfeld` (the professional field, carried
  as the board file entry's `board`), paginating `size=100&page=N` until the result set or the
  API's `page*size ≈ 10 000` depth cap is reached.
- **First-party only.** ~92% of keyword hits and a large share of `berufsfeld` hits carry an
  `externeUrl` — arbeitsagentur re-lists postings that live on other boards. The adapter SHALL drop
  every posting that carries an `externeUrl`, keeping only the agency's own first-party postings
  (no external destination), which are the non-redundant value and far less likely to duplicate our
  existing ATS/board sources.
- **Description via the SSR detail page.** The search response carries no description and the detail
  API returns 403, but the public `https://www.arbeitsagentur.de/jobsuche/jobdetail/<refnr>` page is
  server-rendered and carries the `Stellenbeschreibung`. The adapter fetches that page per kept
  posting (reusing the shared `fetchDetails` pool) and extracts the description; the same URL is the
  posting's canonical/apply URL.
- Map each kept posting to a normalized `Job`: `refnr` → `ExternalID`, `titel` → title, `arbeitgeber`
  → company, `arbeitsort` (`ort`, `region`, `land`) → location, `aktuelleVeroeffentlichungsdatum` →
  posted-at, the jobdetail page → URL and description. Bound per-run work with `veroeffentlichtseit`
  (published-within-N-days) so each crawl is a fresh, incremental window rather than the full backlog.
- The key is a public constant (not a secret), so `arbeitsagentur` registers unconditionally in
  `sources.All` — unlike the env-keyed USAJobs/Reed.
- Add `sources/arbeitsagentur.yml` with one entry per IT `berufsfeld` (`Informatik`,
  `Softwareentwicklung und Programmierung`, `IT-Netzwerktechnik, -Administration, -Organisation`,
  `IT-Systemanalyse, -Anwendungsberatung und -Vertrieb`); overlap across fields collapses on the
  `refnr` dedup key.

## Capabilities

### New Capabilities
- `arbeitsagentur-source`: the `arbeitsagentur` adapter — its keyed `berufsfeld` search + pagination,
  first-party (`externeUrl`-drop) filter, SSR jobdetail description fetch, posting→`Job` mapping, and
  board-as-`berufsfeld` classification.

### Modified Capabilities
<!-- None. Multi-company board-based adapter; inherits the standard ingest sweep, dedup, and
     board-health machinery unchanged. No spec-level behavior of an existing capability changes. -->

## Impact

- **New code:** `internal/sources/arbeitsagentur.go` (+ `_test.go`); `sources/arbeitsagentur.yml`.
- **Touched code:** one line in `sources.All` (registry).
- **Ops:** a new `cmd/ingest sources/arbeitsagentur.yml` cron schedule (deploy-time, in freehire-ops).
- **Language:** postings are German; skill tags (Python/AWS…) survive but seniority/category
  classification (EN+RU-tuned) will be sparse. Accepted.
- **No migrations, no API changes, no new dependencies, no secret key (public static key).**
