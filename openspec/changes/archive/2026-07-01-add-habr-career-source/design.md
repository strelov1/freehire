## Context

Habr Career (`career.habr.com`) is a Rails app whose public vacancy listing is backed by a
keyless JSON API. Reconnaissance (live, 2026-06-19) established the surface:

- `GET /api/frontend/vacancies?type=all&sort=date&page=N` →
  `{ list: [...], meta: { totalResults, perPage: 25, currentPage, totalPages }, recommendedQuickVacancies: [] }`.
  At survey time `totalResults` was 974 across 30 `totalPages`.
- Each list item carries the employer (`company.title`), so Habr Career is a **marketplace
  aggregator**, not a single-company board — matching the existing `getmatch`/`tecla`/`jobstash`
  adapters: boardless, `aggregator()`-marked, with a placeholder company in the config entry.
- List item fields used: `id` (int), `href`, `title`, `company.title`, `remoteWork` (bool),
  `locations[].title`, `skills[].title`, `salary{from,to,currency,formatted}`,
  `publishedDate.date` (RFC 3339).
- The list has **no description**. The full description lives on the detail page
  `GET /vacancies/<id>` (HTML) as a `JobPosting` ld+json `description` — the exact shape the
  existing `internal/linksource/habrcareer.go` already parses.
- `GET /api/frontend/vacancies/<id>` → `404` (no detail JSON API).
- `robots.txt` permits `/vacancies` (only sub-paths like `/responses`, `/profile` are blocked).
- Request headers `Accept: application/json` + `Referer: https://career.habr.com/vacancies`
  are sent (the public Umalanif/Career.habr-parser project uses the same; reduces block risk).

**Coverage ceiling (hard limitation).** The API reports `totalResults: 974` but paginates only
to a flat ~748. This cap sits on the underlying result-set, not on a single ordering — verified
empirically that the union of: all sort orderings (`date`/`salary_asc`/`salary_desc`), every
`s[]` specialization filter, every `qid` qualification filter, and the RSS feed
(`/vacancies/rss`, 15 pages) all return the **same ~748 ids** (0 new beyond the flat set). The
sitemap lists ~114k historical URLs with no open/closed signal (unusable for an "open only"
crawl). The remaining ~225 are unreachable by any anonymous channel — they would require an
authenticated/personalized session, which is out of scope and ToS-sensitive. v1 therefore
ingests the reachable ~748 freshest; the existing Telegram linksource already pulls some of the
hidden ones in via dedup.

## Goals / Non-Goals

**Goals:**

- Ingest Habr Career into the catalogue under `source = "habr_career"` via the existing
  pipeline, with full HTML descriptions and a structured work mode where unambiguous.
- Follow the established boardless-aggregator adapter pattern so the change is one new adapter
  file + one registry line + one board file, with no pipeline changes.
- Dedup board-crawled vacancies with the existing Habr linksource adapter by emitting an
  identical identity; extract the shared detail-page parse into one helper.

**Non-Goals:**

- Reaching the hidden ~225 vacancies (authenticated/personalized access).
- Specialization/qualification slicing — empirically adds zero ids beyond the flat 748.
- Salary ingestion (no field in the raw `Job` shape; enrichment owns compensation).
- Skill ingestion from the listing's `skills[]` (the project derives skills via
  `internal/skilltag` from the description).
- `StreamingSource` / incremental persistence (noted seam; the ~748 detail fan-out is moderate).

## Decisions

**1. Boardless aggregator, mirroring `getmatch`/`tecla`.** Habr lists many employers behind one
feed, so the adapter implements `boardless()` (config entry needs no `board`) and `aggregator()`
(it stays in the source facet). Like the other aggregators it gets a **dedicated**
`sources/habrcareer.yml` (one placeholder entry), not a line in `custom.yml`, so its per-vacancy
detail fan-out crawls on its own cron slot without blocking the hourly single-source providers.

**2. JSON listing + HTML detail, via a private combined client interface.** Like `breezy`, the
adapter declares a private interface embedding `HeaderJSONGetter` (listing needs custom headers)
and `HTMLGetter` (detail page). The shared `*sources.Client` satisfies both. Listing uses
`GetJSONWithHeaders` with the `Accept`/`Referer` headers; detail uses `GetHTML`.

**3. Pagination.** Request `page=1`, read `meta.totalPages`, then fetch `page=2..totalPages`,
stopping early on an empty `list`. A max-page guard (e.g. 50) prevents a bad `totalPages` from
looping. First-page failure → board error; a later-page failure → stop and return what we have
(mirrors `getmatch`).

**4. Identity shared with linksource.** `external_id` = numeric vacancy id; `url` =
`https://career.habr.com/vacancies/<id>`. This is byte-identical to what
`internal/linksource/habrcareer.go` emits, so the pipeline's `(source, external_id)` dedup key
collapses board and link-followed rows into one.

**5. Extract the shared Habr detail parse.** The `JobPosting` ld+json description parse currently
lives inline in `internal/linksource/habrcareer.go`. Extract a small helper in `internal/sources`
(e.g. `habrVacancyDescription(node) string`, reusing `sources.LDJobPosting` + `sources.SanitizeHTML`)
and call it from both the board adapter and the linksource adapter, so the two never diverge.
The board adapter only needs the **description** from the detail page (title/company/location/
date already come from the listing); linksource keeps deriving the rest from the page as it does
today.

**6. Posted date from the listing, not `basic-date`.** Use the listing item's
`publishedDate.date` (clean RFC 3339). The linksource comment claims the detail page's
`datePosted` ld+json runs ~a month ahead and trusts `<time class="basic-date">` instead — but
recon showed `basic-date` is the page **render** time (it equalled "now" on fetch), so it is
also wrong as a publish date. The listing `publishedDate.date` is the authoritative value; this
change uses it for the board adapter and leaves the linksource path's date handling untouched
unless the extraction refactor naturally touches it (note as a follow-up, do not expand scope).

**7. Work mode / remote.** Mark remote and set structured work mode `remote` iff the listing
item's `remoteWork` is `true`; otherwise leave work mode empty so the pipeline's location parser
decides. (Habr's structured signal is a boolean, so we cannot distinguish onsite vs hybrid.)

**8. Description fallback.** If the detail page has no `JobPosting` ld+json or the request fails,
yield the vacancy anyway with an empty description (listing metadata is still useful and
enrichment/skilltag degrade gracefully). A single dropped detail must not drop the vacancy.

## Risks / Trade-offs

- **~748/974 coverage.** Documented hard ceiling, not a fixable seam; mitigated by the existing
  linksource pulling some hidden vacancies. Re-survey if Habr changes the cap.
- **~748 detail fetches per run.** Comparable to existing detail-per-job adapters (USAJobs,
  Reed, getmatch) under the shared rate-limited client; acceptable for a daily cron. If it grows,
  `StreamingSource` is the noted upgrade path.
- **Refactor risk on linksource.** Extracting the shared parse touches a working adapter; its
  existing tests must stay green, and the extraction must preserve current linksource behavior.

## Migration / Rollout

- New adapter ships inside the existing server/ingest binaries — full image rebuild, **no**
  Dockerfile change.
- Needs a new `cmd/ingest sources/habrcareer.yml` cron line (an ops change, mirroring the
  existing `getmatch`/`tecla` aggregator cron slots) so the detail fan-out crawls independently.
- The per-provider stale-job sweep closes `habr_career` jobs unseen for 48h; a reappearing
  vacancy reopens via the upsert. No migration, no reindex required at ingest time (the standard
  facet derivation + enrichment run downstream).
