## Context

micro1 (`jobs.micro1.ai`) is a job board of remote, output-based engineering and
specialist roles. Investigation of the live site established:

- The board root redirects to a Webflow marketing listing; postings are **not**
  enumerable from that page. They **are** fully enumerable from
  `https://jobs.micro1.ai/sitemap.xml` (~285 `/post/<uuid>` entries, robots allows
  all).
- Each `/post/<uuid>` page is a Next.js App Router route that server-renders the
  complete job payload into the RSC flight stream (`self.__next_f.push([1,"…"])`).
  There is **no** standalone `<script type="application/ld+json">` tag; both the
  schema.org `JobPosting` and the richer native `data` object live inside the
  flight, and `data.job_description` is a flight **reference** (e.g. `$15`) that
  resolves to an HTML chunk elsewhere in the same stream.
- The native `data` object carries `client_job_id` (UUID), `job_role_name`,
  `required_skills[]`, `ideal_hourly_rate {min,max}`, `domain_slug`, `job_status`,
  `location_type`/`location_name`, `create_datetime`, and `client_details`
  (clients are anonymized to `client_name: "micro1"`).

This is the same **sitemap → per-posting detail** shape as the `dataart` adapter,
differing only in the detail parser (RSC flight vs. a clean ld+json tag).

## Goals / Non-Goals

**Goals:**
- Ingest all open micro1 postings as normalized `Job`s via a keyless, boardless
  single-company adapter that reuses existing pipeline contracts.
- Parse the RSC-flight `data` object and resolve the referenced description.
- Emit structured `Skills` from `required_skills` into the facet seam.

**Non-Goals:**
- No use of the `prod-api.micro1.ai/api/v1` backend (no public listing endpoint
  found; the sitemap is simpler and stable).
- No per-client company modeling — clients are anonymized, so the source is one
  company (`micro1`).
- No dedicated close-detection; the pipeline sweep closes postings that drop out
  of the sitemap.

## Decisions

- **Enumerate via sitemap, not the API.** The sitemap is public, complete, and
  stable; the API path is obfuscated behind a Webflow bundle and needs no auth we
  can rely on. Mirrors `dataart`. Alternative (reverse-engineer the API) rejected
  as fragile for no gain.
- **Parse the RSC flight `data` object, not the schema.org JobPosting.** Both are
  flight-embedded (neither is a clean tag), but `data` is richer (skills, hourly
  rate, status, domain). We locate the object with `bracketSlice(flight, "data":, '{', '}')`
  (the key occurs exactly once and immediately holds `client_job_id`), `json.Unmarshal`
  it, then resolve `job_description`'s `$N` reference from the flight's text rows and
  strip its `T<hex>,` length-prefix marker before `sanitizeHTML`.
- **Reuse the existing shared RSC primitives; do not write new flight parsing.** The
  `deel`/`vouch`/`alignerr` adapters already read this exact flight format via
  `decodeNextFlight` + `bracketSlice` (in `nextflight.go`). micro1's `$N`→text-row
  resolution is identical to deel's `deelTextRows`/`deelTextRowMarker`; since micro1
  is the second consumer, promote that pair into `nextflight.go` as shared
  `nextFlightTextRows`/`nextFlightTextRowMarker` and repoint deel's one call site.
  The adapter therefore depends on `HTMLGetter` (+ `decodeNextFlight`), not a raw
  `TextGetter`, matching deel.
- **Boardless single-company**, exactly like `dataart`: one `sources/micro1.yml`
  entry with `company: micro1`, `boardless()` marker, excluded from the source
  facet. `ExternalID = client_job_id` gives a stable dedup identity.
- **Interface**: adapter depends on `XMLGetter` + `HTMLGetter`; enumeration and
  fan-out reuse `fetchDetails(urls, defaultDetailWorkers, …)`.

## Risks / Trade-offs

- **RSC flight format is undocumented and can change** → Isolate all flight
  parsing in small, unit-tested helpers over a saved fixture; a parse miss skips
  only that posting (ok=false), never aborts the crawl.
- **Description reference marker (`T<hex>,`) is a Next internal detail** → Strip it
  defensively; if the marker is absent, fall back to the raw resolved chunk rather
  than dropping the description.
- **gig/AI-training postings differ from classic jobs** (product concern, already
  accepted) → Ingest as-is; downstream enrichment/facets handle them like any
  other remote role.

## Migration Plan

Additive only. Ship the adapter + `sources/micro1.yml`; add a
`cmd/ingest sources/micro1.yml` crawl target to the ingest schedule. No schema,
API, or migration changes. Rollback = remove the crawl target / board file.

## Open Questions

- ~~Do non-`open` `job_status` values ever appear in the sitemap?~~ RESOLVED: a
  30-post sample was all `open` (the sitemap tracks live postings). The adapter
  still skips a posting whose `job_status` is present and not `open`, as a cheap
  defensive freshness guard.
