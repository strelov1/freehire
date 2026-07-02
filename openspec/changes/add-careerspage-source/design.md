## Context

careers-page.com hosts each client's career site on a `<tenant>.careers-page.com`
subdomain. A spike established:

- **Listing** (`https://<board>.careers-page.com/?page=N`) is server-rendered HTML.
  Each job card is an `a.job-title-link` anchor to `/jobs/<uuid>`, with
  `data-job-id` / `data-job-city` / `data-job-country` attributes. Pagination uses
  `?page=1,2,3…`.
- **Detail** (`https://<board>.careers-page.com/jobs/<uuid>`) server-renders a clean
  `application/ld+json` `JobPosting`: `title`, `datePosted`, `validThrough`,
  `employmentType` (`FULL_TIME` etc.), `jobLocation` (`address.addressLocality`,
  `address.addressCountry`), `hiringOrganization.name`, `baseSalary`, and a
  `description` HTML string (~4.5 KB observed).
- Everything is keyless. The platform rate-limits aggressive bursts (429), so the
  crawl must stay within the shared client's pacing.

This matches the existing JSON-LD detail adapters (`icims`, `freshteam`,
`successfactors`), which the codebase already supports via `jsonld.go`
(`ldJobPosting`) and `html.go` (`jobLinks`, DOM helpers).

## Goals / Non-Goals

**Goals:**
- A board-based `careerspage` adapter that returns every live posting for one
  tenant, keyless, reusing existing helpers.
- Stable dedup identity from the job UUID.
- Robust pagination and per-detail isolation (one bad page never fails the board).

**Non-Goals:**
- A harvest prober / tenant auto-discovery (`cmd/harvest-boards`). Only one board
  is known today; a prober is a later seam if the board list grows.
- Streaming ingest (`StreamingSource`) — the boards are small; the buffered
  `Fetch` path is sufficient (matches icims/freshteam).
- Parsing `baseSalary` / `validThrough` into structured facets — out of scope; the
  pipeline's dictionaries and the LLM own enrichment.

## Decisions

- **Enumerate from the listing HTML, not a sitemap.** Unlike icims (sitemap.xml),
  careers-page.com exposes the postings directly in the paginated listing. Use the
  shared `jobLinks(base, root, isJob)` helper with `isJob` = href contains
  `/jobs/` and a UUID segment. This reuses the deduping/absolutizing already in
  `html.go`.
  - *Alternative considered:* parse the `data-job-id` attributes off the cards.
    Rejected — the anchor href already carries the UUID and canonical detail URL,
    and `jobLinks` is the established enumeration helper.

- **Stop pagination when a page yields no new job links.** Fetch `?page=N` for
  `N = 1,2,…`, accumulating links; stop when a page contributes zero new links, or
  at a safety cap (e.g. 100 pages) to guard a misbehaving listing — mirrors the
  traffit adapter's never-ending-feed guard.

- **ExternalID = job UUID** extracted from the `/jobs/<uuid>` path via a compiled
  regexp, exactly as icims extracts its numeric id. Stable across crawls → dedups.

- **Map JSON-LD like icims.** Decode a `careerspagePosting` struct with just the
  fields we need through `ldJobPosting`; build the location from
  `jobLocation[0].address` via `joinNonEmpty`; company via
  `firstNonEmpty(hiringOrganization.name, e.Company)`; description via
  `sanitizeHTML(html.UnescapeString(...))`; `PostedAt` via `parseRFC3339`.

- **Work mode:** the JSON-LD `employmentType` is `FULL_TIME`/`PART_TIME`, not a
  workplace-type enum, and no `jobLocationType` was observed, so there is no
  STRUCTURED remote signal. Leave `WorkMode` empty and set `Remote` only from the
  free-text location heuristic (`isRemote`), keeping provenance clean (structured
  fields carry structured signal only).

- **Transport interface = `HTMLGetter` only** (both listing and detail are HTML),
  narrower than icims's `XMLGetter + HTMLGetter`.

## Risks / Trade-offs

- **429 rate limiting** → the bounded `fetchDetails` pool (`defaultDetailWorkers`)
  plus the shared client's pacing keeps bursts modest; a dropped detail is skipped,
  not fatal. If 429s persist in production, lower the worker bound at the call site
  (the codebase's documented escape hatch).
- **Listing markup drift** (class names / pagination scheme change) → the adapter
  breaks for this platform only; covered by a table-driven test over a captured
  `testdata` fixture, the same safety net every HTML adapter has.
- **Pagination that never empties** → bounded page cap prevents an infinite loop
  (the yandex runaway-cursor lesson).

## Migration Plan

Additive and read-only: new adapter file, one registry line, one board file. No
schema/API/migration changes. Deploy is a normal binary roll; a cron schedule for
`sources/careerspage.yml` is an ops follow-up (like every other board file).
Rollback = remove the registry line / board file.

## Open Questions

- None blocking. Tenant auto-discovery (a prober) is deferred until the board list
  justifies it.
