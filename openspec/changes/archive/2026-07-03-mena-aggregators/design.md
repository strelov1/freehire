## Context

`internal/sources` maps a provider key to a `Source` adapter (`Provider()` + `Fetch`).
Most adapters share one jar-less `HTTPClient` (`All(c)`); `meta` is the sole exception —
Meta's edge 400s the default Go TLS+HTTP/2 fingerprint, so `All()` builds a dedicated
Chrome-fingerprint transport (`metaHTTP`, `bogdanfinn/tls-client` `Chrome_133`, dialing
through the `safehttp` SSRF guard) and hands it only to Meta. A spike confirmed Bayt and
GulfTalent sit behind the same class of edge (Akamai/Cloudflare): plain HTTP is 403'd, but
the Chrome-fingerprint client returns 200 with self-contained `JobPosting` JSON-LD. Both
are multi-company aggregators, matching the existing boardless-`aggregator()` marker used
by `tecla`/`jobstash`.

## Goals / Non-Goals

**Goals:**

- Two new aggregator adapters (`bayt`, `gulftalent`) that lift MENA coverage.
- One reusable Chrome-fingerprint transport shared by `meta`, `bayt`, `gulftalent` —
  no duplicated tls-client wiring, SSRF guard preserved.
- Adapters unit-tested over recorded fixtures; live-validated before board data lands.

**Non-Goals:**

- Apply-link mining (the aggregators are self-contained — the company is in the posting).
- Any geolocation-dictionary change (MENA resolution is already adequate).
- Bayt sitemap (403-walled even for the fingerprint client in the spike) — use the
  paginated HTML listings instead.

## Decisions

**1. Generalize `metaHTTP` into a shared fingerprint transport (not per-adapter copies).**
Rename/extract `internal/sources/metahttp.go` into a provider-neutral
`fingerprintHTTP` (new file `fingerprinthttp.go`) exposing the same `get`/`GetHTML`/
`GetXML` surface and the SSRF-guarded dialer. `All()` builds it **once** and passes it to
`NewMetaCareers`, `NewBayt`, `NewGulfTalent`. Meta keeps identical behavior (its test
`TestMetaHTTPBlocksInternalTarget` moves with it). *Alternative rejected:* a second
copy for the aggregators — duplicates the fragile fork wiring and the SSRF contract.

**2. `bayt` — paginated country listing → JSON-LD detail.** Configured entries are
country scopes (the `Board` field carries the Bayt country slug, e.g. `saudi-arabia`),
adapter is `boardless()` + `aggregator()`. `Fetch` walks
`/en/<country>/jobs/?page=N`, scrapes job-detail hrefs (`/en/<country>/jobs/<slug>-<id>/`),
fetches each detail via the bounded `fetchDetails` pool, and parses the `JobPosting`
JSON-LD (title, description, `hiringOrganization.name`, `jobLocation`, `datePosted`,
`identifier`). Pagination stops when a page yields no new detail links. `ExternalID` =
the numeric Bayt id from the URL/`identifier`. *Alternative rejected:* a Bayt search API —
none is exposed keyless; the sitemap is 403-walled.

**3. `gulftalent` — sitemap index → JSON-LD detail.** `boardless()` + `aggregator()`.
`Fetch` reads `/sitemap.xml`, follows the `jl0NN` job-listing sitemaps, collects detail
URLs, and parses each detail's `JobPosting` JSON-LD — the same shape as `successfactors`
(sitemap → schema.org). `ExternalID` = the stable id in the detail URL.

**4. Boardless-aggregator wiring.** Both register in `All()` alongside the other
aggregators, built with the fingerprint transport. Because they are `aggregator()`, they
stay in the source facet and are excluded from the per-company redundancy filter. Company
comes from `hiringOrganization`; the pipeline namespaces `ExternalID` by board via the
existing `NamespaceExternalID`.

**5. Conservative crawl rate.** The fingerprint transport does not retry (like `metaHTTP`).
The aggregators bound their detail fan-out (reuse `defaultDetailWorkers` = 8, or a lower
per-adapter const) and can add a small inter-request delay, so a large crawl does not draw
Akamai throttling. A dropped detail is skipped, not fatal; a failed listing/sitemap fails
that run's board without closing jobs.

## Risks / Trade-offs

- **Edge tightens the fingerprint check at crawl volume** → the spike issued only a few
  requests. Mitigation: bounded fan-out + inter-request delay + no-retry fail-soft; if the
  edge hardens, the `Chrome_133` profile is bumped in one shared place. A failed run closes
  no jobs (48h unseen sweep absorbs a skipped run).
- **JSON-LD markup drift** → resilient parsing errors loudly (a detail with no `JobPosting`
  is an error, not a silent empty), so a layout change surfaces rather than emptying the
  catalogue. Fixtures pin the current shape.
- **tls-client is a forked net/http** → already an accepted dependency (Meta). No new
  module; the fork stays isolated to the transport file.
- **Bayt id extraction depends on the URL/`identifier` shape** → a posting with no
  extractable id is dropped rather than persisted with an empty dedup key.

## Migration Plan

1. Ship the transport refactor + `bayt` + `gulftalent` behind their board files; no
   existing behavior changes (Meta is byte-for-byte equivalent).
2. Add `sources/bayt.yml` (country list) and `sources/gulftalent.yml`; wire cron ingest
   schedules (one per board file).
3. After the first ingest, run `cmd/backfill-derive` + `make reindex` so MENA jobs surface
   in the geography facet.
4. Mega-employer seed lands independently as board entries in existing `sources/*.yml`.
5. Rollback: remove the two board files / registry lines; no schema or data migration.

## Open Questions

- Exact GulfTalent detail-URL id pattern (confirm against a live detail page when writing
  the adapter fixtures) — does not block the design.
