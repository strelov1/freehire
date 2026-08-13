## Context

`internal/sources/echojobs.go` is a boardless, hydrating aggregator adapter (see
`internal/sources/AGENTS.md` for the `HydratingSource` shape): `crawl()` pages a list
endpoint to discover postings inside a 14-day freshness window, `FetchNew` hydrates a
per-posting detail body only for postings the catalogue does not already have (`seen`),
and already-seen postings get a cheap liveness `touch()` instead of a full re-upsert (so a
content-less re-listing never wipes the description hydrated when the posting was new).

Both endpoints this design depended on — `GET /api/jobs?page=N` (list) and `GET
/api/jobs/<handle>` (detail) — now 404. Live verification (both from this machine and from
prod's own egress IP) shows the site itself still works; a headless-browser capture of
`https://echojobs.io/jobs` showed data now loads via Next.js App Router RSC payloads
(`?_rsc=<build-hash>`), and job pages moved from an API route to a page route
(`/job/<company>-<role>-<suffix>`). This reads as a permanent migration, not a transient
outage: the last successful ingest run using the old API was 2026-08-13 01:06 UTC, and
`/api/jobs` has stayed 404 since.

Two further findings de-risk the rewrite considerably:

1. **`https://echojobs.io/sitemap.xml`** is a public sitemap index pointing to
   `sitemap-jobs/1.xml` … `23.xml`, 10,000 URLs each. Sampling shard boundaries confirmed
   they are globally sorted newest-first by `<lastmod>` (shard 1 spans the last ~2 days,
   shard 23 the oldest). This replaces the list crawl's pagination without needing the
   dead API.
2. Every `/job/<slug>` page — fetched with a plain `GET`, no JavaScript execution needed —
   embeds a `<script type="application/ld+json">` block with `"@type":"JobPosting"`: the
   standard schema.org structured-data markup sites publish for Google for Jobs indexing.
   Verified live on both a sample posting (JLL, on-site) and the originally-reported Doowii
   posting (remote): it carries `title`, `hiringOrganization.name`, full `description`
   (HTML), `jobLocationType` (`"TELECOMMUTE"` for remote), `jobLocation.address` /
   `applicantLocationRequirements.name`, `skills` (comma-separated string), and
   `datePosted`. Slugs are unchanged from the old `job_handle` scheme (verified: the
   Doowii posting's slug, `doowii-full-stack-software-engineer-prmbc`, is byte-identical
   to the `ExternalID` already stored from before the outage), so no re-ingestion or
   duplicate rows result from the swap.

## Goals / Non-Goals

**Goals:**
- Restore echojobs ingestion (list discovery + description hydration) using the new site's
  publicly-available, intentionally-maintained data surfaces (sitemap + JSON-LD) — not a
  reverse-engineering of the RSC wire format, which is an internal implementation detail
  tied to a specific build hash and likely to break again on redeploy.
- Preserve the adapter's external contract: same `ExternalID` scheme, same 14-day
  freshness window, same `HydratingSource` behavior (detail fetched only for new
  postings), same request-volume order of magnitude per run.
- Fix the same root cause in `EchoJobsDescription` (backing `cmd/backfill-echojobs`) and
  `cmd/liveness/echojobs.go`, which call the same dead endpoint.

**Non-Goals:**
- Not scraping or headless-browser-rendering echojobs pages — the JSON-LD block makes
  that unnecessary.
- Not adding new Job fields (e.g. `educationRequirements`, `experienceRequirements`) the
  JSON-LD happens to carry but the current `Job` shape has no home for — out of scope,
  matches AGENTS.md's existing "don't guess a field's home" precedent (see echojobs.go's
  doc comment on fields it deliberately does not read).
- Not building a generic "extract schema.org JobPosting from HTML" package — echojobs is
  the only current consumer; a shared helper is a seam for later if a second adapter needs
  it (YAGNI per this repo's working principles), not now.

## Decisions

**List discovery: sitemap shards over RSC parsing or full HTML scraping.**
Considered three sources for "what postings exist, newest first": (a) parse the RSC
`?_rsc=` payloads the live site uses, (b) scrape rendered `/jobs` listing pages, (c) page
the sitemap. Rejected (a): RSC framing is undocumented, tied to a `dpl_...` build hash
that changes on every echojobs deploy, and not intended for external consumption — the
adapter would be back here after their next redeploy. Rejected (b): the listing page
needs JS execution to populate (confirmed via network capture), meaning per-run headless
browser sessions — expensive and slow at the volume this adapter needs. Chose (c): the
sitemap is a stable, publicly-documented format (referenced from `robots.txt`), requires
only plain HTTP GETs, and is already sorted the way the freshness-window walk needs.

**Detail hydration: JSON-LD over RSC parsing or HTML scraping.**
Same reasoning as above — the JSON-LD block is schema.org structured data, which sites
have a standing incentive (Google for Jobs indexing) not to break carelessly, unlike an
internal API route. It is also a plain `<script>` tag in server-rendered HTML, parseable
with `encoding/json` after a regex/string extraction of the tag body — no HTML-DOM
parsing library needed for the fields this adapter reads.

**Refresh strategy for already-seen postings: liveness-only `touch()`, not a full re-fetch.**
The 14-day freshness window covers 50,000+ postings (extrapolated from shard date
ranges). Re-fetching the JSON-LD detail for every postings in that window on every run —
simplest to implement — was explicitly rejected (by the user) as excessive load on
echojobs.io and a real rate-limit/block risk, and slower per run than the old adapter by
roughly two orders of magnitude. Instead: a posting whose slug is already in the
catalogue (`seen(externalID)`) never gets a detail fetch — same as today — and is
reported with `SeenRefresh: true` and an empty `Title`. Verified this is safe two ways:
`internal/pipeline.Runner.touch()` reads only `source` + `externalID` from the `Job`
(`internal/pipeline/pipeline.go:543-550`), and `classify.ConfirmedNonTech("", ...)` —
which gates a `SeenRefresh` posting through the same catalogue filter a real save would
hit — never matches an empty title against the non-tech dictionary, so it cannot
spuriously reject a posting that was already accepted once
(`internal/classify/nontech.go:202-207`).

**Field mapping notes** (JSON-LD → `Job`):
- `jobLocationType == "TELECOMMUTE"` → `Remote: true, WorkMode: "remote"`; anything else
  → `WorkMode: ""` (unknown), matching the existing adapter's principle of never guessing
  hybrid/onsite from an ambiguous signal.
- `jobLocation.address` (`addressLocality`, `addressCountry`) when present; otherwise
  `applicantLocationRequirements.name` for remote postings that carry no `jobLocation`.
- `skills`: a single comma-separated string in the new format (was a JSON array before) —
  split and trim, then run through the existing `skilltag.Canonicalize`.
- `datePosted`: ISO 8601 (`time.Parse(time.RFC3339, ...)`), replacing the old
  epoch-millisecond parsing.

## Risks / Trade-offs

- **[Risk] The sitemap or JSON-LD format could itself change or disappear.** Lower
  likelihood than the API (both are maintained for SEO purposes, a standing incentive
  the old private API never had), but not zero. → Mitigation: none built in; if it
  happens, the failure mode is the same as today's — `crawl`/`detail` return an error,
  the board fails cleanly and is retried next run, board-health cooldown applies as usual.
  No special handling beyond what every adapter already gets.
- **[Risk] Sitemap shard count/URLs-per-shard could change over time** (currently 23
  shards × 10,000). → Mitigation: the crawl walks `sitemap.xml` to discover shard URLs
  dynamically rather than hardcoding a shard count, and stops on the freshness-window
  cutoff rather than a fixed shard number either way.
- **[Trade-off] `SeenRefresh` postings carry no `Title` any more** (previously came from
  the list endpoint, which had it; the sitemap does not). Verified harmless for the one
  path that reads it (the catalogue filter — see Decisions above), but any *future* code
  added to the `SeenRefresh` branch that assumes a non-empty `Title` would silently regress
  for echojobs specifically. Not mitigated beyond this note — the existing code path
  doesn't need it today.
- **[Risk] Volume estimate (50,000+ postings/window) is extrapolated from shard date
  ranges, not measured against the new crawl in production.** → Mitigation: the design
  bounds the *expensive* operation (full detail fetch) to genuinely new postings, which
  regardless of the window's total size stays close to today's per-run new-posting count —
  the estimate only matters for confirming the *cheap* path (sitemap shard reads) is cheap
  enough, and even 50,000 lightweight XML entries across ~5-6 shard fetches is negligible.

## Migration Plan

No data migration. This is a pure adapter-internals swap: `ExternalID` scheme, `Job`
output shape, and freshness-window semantics are unchanged, so existing rows and the
dedup key are unaffected. Deploy is a normal `release.sh` build; the next scheduled
`freehire-ingest@echojobs.timer` fire picks up the new adapter automatically. After
deploy, `cmd/backfill-echojobs` should be run once by hand to repair the empty-description
rows accumulated during the outage window (2026-08-13 onward) — no timer runs it
automatically (see `internal/sources/AGENTS.md`'s note that it is a one-off worker).
Rollback is a normal `rollback.sh` to the prior color; no schema or on-disk state to undo.

## Open Questions

- None outstanding. The only real unknown going in — whether a lightweight,
  non-JS data source exists at all for the new site — was resolved during design
  (sitemap + JSON-LD), so no further spike is needed before implementation.
