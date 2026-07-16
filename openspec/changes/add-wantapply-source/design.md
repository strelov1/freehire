## Context

Wantapply is a `directApply` job aggregator. Its `.com` host sits behind a WAF that 403s
non-browser clients; the `.cy` mirror serves identical content (same DB — sitemap `lastmod`
timestamps match to the millisecond) without the WAF. Each vacancy page carries a clean
`JobPosting` JSON-LD (`title`, `description` HTML, `datePosted`, `validThrough`, `employmentType[]`,
`hiringOrganization.name`+`sameAs`, `jobLocation[].address`, `jobLocationType`, `directApply`). The
sitemap lists ~3000+ vacancy slugs plus `/company/*` and `/jobs/*` taxonomy pages. Closed vacancies
drop from the sitemap / return an empty page.

This mirrors the existing `justjoin` adapter: a large boardless aggregator whose list omits the
body, so detail is fetched per posting — and only for postings the catalogue does not already have
(`HydratingSource`). The reused primitives are `internal/sources/jsonld.go` (`LDJobPosting`), the
shared HTTP client, and the `boardless`/`aggregator`/`HydratingSource`/`SeenRefresh` seams in
`source.go`.

## Goals / Non-Goals

**Goals:**
- Full Wantapply catalogue in the catalogue as `source='wantapply'`, with structured facets from the JSON-LD and a real close signal via the board unseen-sweep.
- Steady-state crawl fetches detail only for new vacancies.
- Widen the Wantapply Telegram footprint (4 new channels) without removing the existing one.

**Non-Goals:**
- Resolving `hiringOrganization.sameAs` to a company's native ATS (field is present on only a minority of pages; the stored apply URL is the wantapply page).
- Any cross-source dedup change to reconcile the site adapter with the retained Telegram channels — the small overlap is accepted.
- Crawling the `.com` host or bypassing its WAF.

## Decisions

- **Crawl `.cy`, store `.cy` URL.** The `.cy` mirror is the only host that reliably serves us content, and (per the user) the stored public apply URL is `https://wantapply.cy/<slug>`. Alternative — store `.com` (real browsers reach it) — rejected to keep the stored URL on the host we actually verified.
- **Boardless aggregator, not per-company boards.** One vacancy set spanning many employers; company comes from `hiringOrganization.name`. Modeled exactly on `justjoin`/`jobstash`. A single boardless entry in `sources/wantapply.yml` drives the crawl.
- **`HydratingSource.FetchNew` with `SeenRefresh`.** `Fetch` (list-only fallback) yields every sitemap vacancy without detail; `FetchNew` fetches detail for unseen slugs and emits a `SeenRefresh` job for seen ones — identical to `justjoin`, so seen rows keep their hydrated content and the unseen-sweep still sees them as live. Alternative — always fetch all detail — rejected (3000 fetches/run and it would clobber hydrated content).
- **ExternalID = slug.** Stable, unique, already the URL tail; no separate id field exists in the JSON-LD worth preferring.
- **Not self-closing; rely on the unseen-sweep.** The adapter re-lists all current vacancies each run, so the sweep is the close signal — this is the whole point vs. the telegram channel, which has none. No `selfClosing` marker.
- **Bounded, polite crawl.** Low/sequential detail concurrency with a browser-ish UA. `.cy` is open, but Wantapply may throttle like other CloudFront-fronted aggregators; conservative by default.
- **Telegram channels stay as board-file entries.** Adding 4 `kind: board` channels is config, not code; `cmd/tg-ingest`/`cmd/tg-extract` already handle them.

## Risks / Trade-offs

- **Duplicate rows for the TG↔site overlap** → accepted. The ~86 telegram jobs are a subset of the site's ~3000; cross-source dedup only suppresses aggregator copies against a first-party ATS, so both rows can coexist. The site adds large net-new coverage; the user chose to keep TG.
- **`.cy` throttling / WAF change on the mirror** → the per-board health sidecar cools the board on repeated failure; a `.cy` outage degrades to stale-but-open rows until the sweep, not a crash.
- **First run fetches ~3000 detail pages** → one-time cost, bounded concurrency; subsequent runs are near-zero detail (only new slugs).
- **Empty/closed page returning 200 with no JSON-LD** → treated as "drop this vacancy" (not an error); the sweep closes it once it also leaves the sitemap.

## Migration Plan

- Ship code + `sources/wantapply.yml` + telegram channels via the normal git→prod `sources` sync.
- Add a per-board ingest cron timer for `sources/wantapply.yml` (staggered like other boards).
- Rollback: remove the `wantapply` line from `sources.All` / drop the board file and timer; the telegram channels are independent and can be reverted separately. No schema/migration changes.

## Open Questions

- None blocking. Detail concurrency and crawl cadence can be tuned after observing the first prod runs.
