## Context

`web/src/routes/sitemap.xml/+server.ts` builds one `<urlset>` by paging the
public `searchJobs` (Meilisearch) and `listCompanies` (Postgres) endpoints,
capped at 5,000 jobs + 5,000 companies. Live catalogue: **~2.5M open jobs, ~88k
companies** — so the cap hides ~95% of pages. The file already documents a
"full sitemap-index + lightweight backend slug endpoint" as the intended
follow-up; this change is that follow-up.

Constraints:
- Sitemap protocol: ≤50,000 URLs and ≤50 MB per file; an index may reference
  many sub-sitemaps.
- Prod is resource-sensitive (past disk-full and reindex-freeze incidents), so
  the generation must not run expensive queries on every crawler hit.
- `jobs` has a numeric PK `id` (ordered, indexed); `companies` is keyed by a
  TEXT `slug` PK (no numeric id). Both have `updated_at`.
- Responses are HTTP-cached (`max-age=3600`); crawlers refetch, not humans.

## Goals / Non-Goals

**Goals:**
- Enumerate the full open catalogue (all open jobs + all companies) across a
  sitemap index and sub-sitemaps.
- Keep every request a bounded, index-friendly scan regardless of catalogue
  size.
- Serve only the fields the sitemap needs; never pull wide job rows or the
  search engine into sitemap generation.

**Non-Goals:**
- No content-quality filtering of which jobs to include (all open jobs ship;
  a future quality gate is a separate change — noted as a seam).
- No DB schema/migration changes.
- No change to `robots.txt` beyond it continuing to reference `/sitemap.xml`.
- No incremental/`lastmod`-diffed sub-sitemap invalidation beyond HTTP caching.

## Decisions

### Keyset pagination, not OFFSET
Sub-sitemap chunks are addressed by a cursor — the last `id` (jobs) / `slug`
(companies) of the previous chunk — and fetched with `WHERE id > $cursor ORDER
BY id LIMIT $n`. **Why:** `OFFSET n` over millions of rows forces Postgres to
walk and discard `n` rows; the deepest job chunk would skip ~2.5M rows, and 50+
such chunks refetched hourly would hammer prod. Keyset scans only the chunk via
the PK index. *Alternative rejected:* page-number + OFFSET — simpler URLs but
O(offset) cost per chunk.

### The index is built from chunk-boundary cursors, computed cheaply
`GET /sitemap.xml` needs the list of sub-sitemap URLs, i.e. each chunk's start
cursor. A dedicated backend query returns just the boundary cursors (the id at
every 50,000th open job, ordered) so the index is one small query, not a scan of
2.5M rows returned to the client. The index emits, per boundary, a
`/sitemap-jobs/<cursor>.xml` (and `/sitemap-companies/<cursor>.xml`) URL, plus a
single `/sitemap-pages.xml` for the handful of static routes. *Alternative
rejected:* client walks every chunk to discover cursors — fetches the whole
catalogue just to build the index.

### Chunk size = 50,000 (the protocol max)
Minimises the number of sub-sitemaps (~51 job files, ~2 company files) and thus
index size and per-crawl request count, while staying within the 50k-URL /
50 MB limits (a job URL line ≈ 110 bytes → ≈ 5.5 MB per file).

### Slim, read-only backend endpoints under `/api/v1`
New Fiber handlers backed by new sqlc queries return only `{slug, updated_at}`
(and `id` as the next cursor for jobs). Registered **before** the `/jobs/:slug`
and `/companies/:slug` catch-alls (same ordering the existing `/jobs/search`,
`/jobs/facets` static routes rely on) so `sitemap` is not captured as a slug.
Job cursor is the numeric `id`; company cursor is the `slug` string (`slug >
$cursor`), first chunk keyed by the empty string.

### Frontend route shape
- `sitemap.xml/+server.ts` → `<sitemapindex>` (rewrite of the existing file).
- `sitemap-pages.xml/+server.ts` → static URLs.
- `sitemap-jobs.xml/+server.ts` and `sitemap-companies.xml/+server.ts` →
  per-chunk `<urlset>`, addressed by a **`?after=<cursor>` query param** (id for
  jobs, slug for companies). *Chosen over a `[cursor].xml` path param:* the first
  company chunk's cursor is the empty string (sorts before every slug), which a
  path segment cannot carry (`/…/.xml` won't match `[cursor].xml`); a query param
  represents it cleanly as `?after=`. XML escaping and the `<url>`/`<lastmod>`
  builders live in a shared `$lib/sitemap.ts` (extracted from the current file).

## Risks / Trade-offs

- [Boundary-cursor query still scans the open-jobs index] → It reads only the PK
  index for boundaries (no wide columns), runs once per index request, and is
  HTTP-cached hourly — comparable to the existing hourly reindex scan. If it
  proves costly, a covering/partial index on `jobs (id) WHERE closed_at IS NULL`
  is the mitigation (a parallel `perf-jobs-indexes` worktree is already touching
  jobs indexes; coordinate rather than duplicate).
- [2.6M URLs may dilute crawl budget / include low-value non-tech postings] →
  Accepted for now; the include-set is a single query, leaving a clean seam to
  add a quality filter later without reshaping the mechanism.
- [Cursor in the URL couples routes to `id`/`slug` ordering] → Both are stable
  keys (immutable PK, slug); a re-slug would already require a reindex.
- [Sub-sitemap and index can momentarily disagree during writes] → Sitemaps are
  advisory hints; a missing/extra URL for one cache cycle is harmless.

## Migration Plan

Pure add/replace of read paths; no data migration. Deploy web + backend
together (the frontend routes call the new endpoints). Rollback = revert the
commit; the previous single-file sitemap returns. No reindex required.

## Open Questions

- Should companies with zero open jobs be excluded from the company sub-sitemaps
  (their page still renders)? Default: include all companies; revisit if noise.
