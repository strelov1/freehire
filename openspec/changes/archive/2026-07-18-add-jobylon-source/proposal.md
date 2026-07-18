## Why

Jobylon (jobylon.com) is a Swedish/Nordic ATS powering ~1000 employers' career sites (Tele2,
Scandic, McDonald's Sverige, Etteplan, HEMA, Radboudumc, …). Its per-company embed widget caps at
~32 postings (page_size is ignored), so a per-company board model would silently lose 95 %+ of a
large employer's jobs (HEMA has 994). But Jobylon publishes a single, complete, keyless global
jobs sitemap at `https://emp.jobylon.com/sitemap.xml` → `sitemap-jobs.xml` (~8 600 job URLs with
`lastmod`), and every job's canonical page `https://emp.jobylon.com/jobs/<id>-<slug>/` server-
renders a schema.org `JobPosting` ld+json (title, datePosted, description, hiringOrganization,
jobLocation). Crawling the sitemap and hydrating each job from its ld+json gives complete coverage
across every Jobylon employer in one adapter — and, because each posting carries its own
`hiringOrganization`, it collects every Jobylon company automatically.

## What Changes

- Add a `jobylon` source adapter that enumerates the global jobs sitemap
  (`https://emp.jobylon.com/sitemap.xml`, resolving the `sitemap-jobs` sub-sitemap) into
  `emp.jobylon.com/jobs/<id>-<slug>/` job URLs, and maps each job's `JobPosting` ld+json to a
  normalized `Job`: the numeric `<id>` from the URL as `ExternalID`, the URL as the canonical
  `URL`, `title` (HTML-unescaped) as the role, `hiringOrganization.name` as the per-posting
  company, `jobLocation` joined into a free-text location, the sanitized `description` HTML as the
  body, remote inferred from the location text, and `datePosted` as `PostedAt`.
- The adapter is **boardless** (one global feed, no per-tenant board) and an **aggregator** (the
  company comes from each posting), so it stays in the source facet and inherits the reindex
  aggregator/ATS-duplicate suppression.
- The adapter is a **HydratingSource**: it lists every sitemap job URL every crawl but issues the
  per-job ld+json detail fetch only for postings the catalogue does not already have (`seen`);
  an already-ingested posting is emitted as a liveness `SeenRefresh` (identity only, no detail
  request), so a routine crawl over ~8 600 jobs costs only as many detail fetches as there are new
  postings. The list-only `Fetch` fallback (detail-fetch every URL) is used when no seen set is
  available.
- A posting whose ld+json is missing, or whose title or company resolves empty, is dropped rather
  than emitted with a broken slug / empty dedup key.
- Enroll `jobylon` in `sources.All` and add `sources/jobylon.yml` with a single boardless entry,
  crawled by its own `cmd/ingest` cron schedule.

## Capabilities

### New Capabilities
- `jobylon-source`: the `jobylon` adapter — its sitemap enumeration, per-job `JobPosting` ld+json
  mapping, `<id>` extraction from the job URL, drop rules for unusable postings, boardless-
  aggregator classification, and HydratingSource incremental (seen → `SeenRefresh`) crawl.

### Modified Capabilities
<!-- None. Boardless aggregator adapter; inherits the standard ingest sweep, aggregator dedup,
     HydratingSource seen-set, and board-health machinery unchanged. No spec-level behavior of an
     existing capability changes. -->

## Impact

- **New code:** `internal/sources/jobylon.go` (+ `_test.go`); `sources/jobylon.yml`.
- **Touched code:** one line in `sources.All` (registry).
- **Ops:** a new `cmd/ingest sources/jobylon.yml` cron schedule (deploy-time, in freehire-ops).
- **No migrations, no API changes, no new dependencies, no proxy (keyless, direct fetch works).**
