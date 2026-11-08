## Why

The catalogue is heavily US/EU/BR biased: only ~1.3% of companies are MENA (2,357 of
184,067), despite the Middle East being a material share of global IT hiring. The
geolocation dictionary already resolves MENA countries/regions, so the gap is **source
coverage**, not normalization — and the recently added ATS platforms (paylocity, apploi,
hireology, isolvedhire, applicantpro) are US-SMB centric and do not lift the region.

## What Changes

- Add a **`bayt` aggregator adapter** — Bayt.com is the dominant Gulf job board. Crawl
  paginated per-country listings (sa/ae/qa/kw/bh/om/eg/jo) and parse each job's
  self-contained `JobPosting` JSON-LD (company in `hiringOrganization`).
- Add a **`gulftalent` aggregator adapter** — GulfTalent is the other major Gulf board.
  Enumerate its sitemap index (`jl000..jl006`) and parse each detail page's JSON-LD.
- **Generalize the Meta fingerprint transport.** Both aggregators sit behind
  Akamai/Cloudflare; plain Go HTTP is 403'd. A spike VALIDATED that a Chrome TLS+HTTP/2
  fingerprint client (`bogdanfinn/tls-client` `Chrome_133`, already in the repo as the
  Meta-only `internal/sources/metahttp.go`) returns 200 with real data. Extract that
  transport into a reusable Chrome-fingerprint HTTP client (keeping the SSRF-guarded
  dialer) shared by Meta, `bayt`, and `gulftalent`.
- Register both as **boardless aggregator sources** (`sources.All` + aggregator marker)
  with `sources/bayt.yml` (country seed list) and `sources/gulftalent.yml`.
- **Mega-employer seed** (data, not a spec): curate ~40–60 of the largest ME employers,
  detect each one's existing-supported ATS, and add one board entry per company to the
  matching `sources/<provider>.yml`. Independent of the adapters.

Out of scope: apply-link mining (redundant — the aggregators are self-contained); any
geolocation-dictionary change.

## Capabilities

### New Capabilities

- `bayt-source`: Bayt.com aggregator adapter — paginated per-country listing crawl plus
  JSON-LD detail parsing over the shared Chrome-fingerprint transport.
- `gulftalent-source`: GulfTalent aggregator adapter — sitemap-index enumeration plus
  JSON-LD/HTML detail parsing over the shared Chrome-fingerprint transport.

### Modified Capabilities
<!-- None: the fingerprint-transport generalization is an implementation refactor that
     preserves existing Meta behavior; no source-ingest requirement changes. -->

## Impact

- **New code:** `internal/sources/bayt.go`, `internal/sources/gulftalent.go`, a
  generalized fingerprint transport (refactor of `internal/sources/metahttp.go`),
  `sources/bayt.yml`, `sources/gulftalent.yml`, adapter unit tests + `testdata/`.
- **Modified code:** `internal/sources` registry (`sources.All`) to register the two
  new providers as aggregator-marker boardless sources; Meta adapter re-pointed at the
  shared transport.
- **Dependencies:** reuses the existing `bogdanfinn/tls-client`/`fhttp` dependency; no
  new module.
- **Ops:** two new cron ingest schedules (one per board file); after first ingest,
  `cmd/backfill-derive` + `make reindex` to surface new MENA jobs in facets. Scale-risk:
  conservative per-crawl rate-limit to absorb possible Akamai throttling at volume.
- **Board data:** `sources/<provider>.yml` gains ~40–60 mega-employer entries.
