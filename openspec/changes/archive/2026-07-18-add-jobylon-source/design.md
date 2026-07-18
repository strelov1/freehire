## Context

Jobylon is a Nordic ATS. Live probing established:

- **Per-company embed is capped.** `https://cdn.jobylon.com/jobs/companies/<id>/embed/v1/` returns
  a server-rendered job list but caps at ~32 postings regardless of `page_size` (verified: HEMA,
  994 jobs, returns 0–32; Tele2, 32). The Feed API (`/feeds/<hash>/?format=json`) is complete but
  needs a support-provisioned per-feed hash — not keyless. So a per-company board model is both
  incomplete and unscalable.
- **The global sitemap is complete and keyless.** `https://emp.jobylon.com/sitemap.xml` is a
  sitemap index whose one child is `https://emp.jobylon.com/sitemap-jobs.xml`, a flat `<urlset>`
  of ~8 600 `<url>` entries, each `https://emp.jobylon.com/jobs/<id>-<slug>/` with a `<lastmod>`.
- **Each job page carries a schema.org `JobPosting` ld+json.** Fields observed: `title` (with HTML
  entities, e.g. `&amp;`), `datePosted` (RFC3339 or RFC3339Nano-`Z`), `description` (HTML,
  ~7 KB), `hiringOrganization.name`, and `jobLocation` (a `Place` or array of them).

This is the exact successfactors/clinch/isolvedhire shape (`internal/sources/isolvedhire.go`): a
sitemap enumerates the postings, each posting's fields come from its detail page's ld+json. The
only structural differences are that Jobylon's sitemap is one global feed for all tenants (so the
adapter is a boardless aggregator, company per posting) and its size warrants HydratingSource
incremental hydration.

## Goals / Non-Goals

**Goals:**
- A keyless, boardless, aggregator `jobylon` adapter mapping the global jobs sitemap to `Job`s.
- Complete coverage of every Jobylon employer, including large ones the embed cannot serve.
- Incremental crawls: detail-fetch only new postings (HydratingSource), refresh the rest.
- Reuse the existing `sitemap.go`, `jsonld.go`, `schema.go`, and `sanitizeHTML` machinery; no new
  shared helper.

**Non-Goals:**
- A per-company board model / embed parsing. Rejected: the embed's ~32 cap loses a large
  employer's postings, and company ids are not uniformly exposed (some tenants use custom domains
  like `jobs.hema.com`). The global sitemap supersedes it.
- The keyed Feed API. It needs a support-provisioned hash; the keyless sitemap is complete.
- Structured `employmentType`/`workMode` facets. Jobylon's ld+json emits `employmentType`
  inconsistently (absent, a string, or an ARRAY like `["CONTRACTOR"]`) and no `jobLocationType`;
  the array form would fail the whole-posting unmarshal if modeled as a string, so the field is
  left unmodeled (Go ignores it) and the classify/enrich stages derive employment/work mode.

## Decisions

- **`ExternalID` = the numeric `<id>` from the job URL** (`/jobs/(\d+)`). It is the stable native
  posting id and is present in both the sitemap loc and the canonical page URL. A URL with no
  numeric id is not a job page and is skipped.

- **Enumerate via the sitemap index, not a hardcoded sub-sitemap URL.** `resolveSubSitemap` on
  `https://emp.jobylon.com/sitemap.xml` finds the `sitemap-jobs` child, then `sitemapJobLocs`
  returns each `<url>` loc that yields a job id. This survives a rename of the sub-sitemap file.
  The ~1.4 MB jobs sitemap fits the buffered `GetXML` cap (no streaming needed, unlike isolved's
  32 MB tenants).

- **Company from `hiringOrganization.name`** (aggregator). The board file `company:` is only a
  label; each `Job.Company` is the posting's own employer. A posting whose title or company
  resolves empty is dropped — an empty company breaks the public slug (mirrors weworkremotely /
  jobspresso).

- **Location from `jobLocation`**, decoded through the shared `schemaPlaces` (single-or-array) and
  each `Place` joined via `schemaAddress.Location()` (`locality, region, country`), places joined
  with `"; "` and deduped via `distinctJoin`. Most postings carry only `addressLocality` (the
  city; the country sits untyped inside `streetAddress` in a local language — `Danmark`,
  `Nederland` — so it is deliberately NOT parsed out); newer postings carry full
  `addressCountry`/`addressRegion`. The location dictionary derives the country from the city.
  `Remote` is inferred from the location text (`isRemote`); `WorkMode` is left empty (no structured
  workplace-type field) for the pipeline to resolve.

- **HydratingSource incremental.** `FetchNew(seen)` lists every sitemap job URL, then for each:
  a `seen(id)` posting is emitted as `Job{ExternalID, URL, SeenRefresh: true}` (liveness refresh
  by identity, no detail request, no content rewrite); an unseen posting is hydrated from its
  ld+json. `Fetch` (the list-only fallback used when the Store can't supply a seen set) detail-
  fetches every URL. Detail fetches run under the shared bounded `fetchDetails` pool. This bounds a
  routine crawl to (new postings) detail requests instead of ~8 600.

- **`title` and `description` are HTML-unescaped** before use (`html.UnescapeString`) — the ld+json
  carries entities (`TV &amp; Streaming`) — and the description is then run through `sanitizeHTML`,
  as isolvedhire does.

- **Boardless aggregator, keyless.** Add the `boardless()` and `aggregator()` markers; the config
  entry is a single boardless line with `company:` present only as a label. One line in
  `sources.All`. No proxy — a direct datacenter fetch of the public sitemap and job pages works.

## Risks / Trade-offs

- **~8 600-job first crawl.** The initial ingest (empty seen set) detail-fetches every posting
  (~1.3 GB, bounded by `defaultDetailWorkers`). → Accepted as a one-off; subsequent crawls hydrate
  only new postings via the seen set. The same shape as justjoin's ~20 k-offer first crawl.
- **`employmentType` array landmine.** Modeling it as a string would drop every posting that emits
  the array form. → Mitigated by not modeling the field at all (Go silently ignores unmodeled
  JSON fields); employment type is derived downstream. Pinned by a real-fixture test that includes
  the array form.
- **Country often absent from structured fields.** → Accepted; `addressLocality` (the city) plus
  the geo dictionary resolves the country. Parsing localized country names out of `streetAddress`
  was rejected as brittle.
- **Sitemap could split into multiple sub-sitemaps as Jobylon grows.** → `resolveSubSitemap`
  currently takes the first `sitemap-jobs` match; a future multi-shard sitemap is the seam to
  extend then, not now (noted, not built).

## Migration Plan

- No DB migration, no API change, no new dependency.
- Deploy: add a `cmd/ingest sources/jobylon.yml` cron schedule in freehire-ops.

## Open Questions

- None. The sitemap and job pages are keyless, public, and fetched directly; the mapping is pinned
  to real fixtures (a standard posting, an array-`employmentType` posting, a multi-location
  posting).
