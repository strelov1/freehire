# Public figures: catalogue scale, per-source stats, sign-up attribution

The rules behind every number freehire publishes about itself, and behind the one it records
about where its accounts come from. View counts have their own rule in
[internal/application/viewlog/AGENTS.md](../../internal/application/viewlog/AGENTS.md).

## Catalogue scale

Every public figure describing how big the catalogue is — `GET /api/v1/stats/catalog`, the
jobs list's `meta.total`, the `/about` and `/open` strips — reads ONE snapshot published by
`cmd/rollup-stats` (`internal/ingest/catalogstats`).

- **Never count on a request path.** `catalogstats.Load` takes no exact counter, so it
  cannot.
- **A read never fails.** A cold cache, an unreachable Redis or no cache at all degrades to
  the planner estimate with `exact: false`, which zeroes the figures that exist only in the
  database — pass `exact` through to whatever renders them, because a zero must not reach a
  page as if it were a measurement.
- **Two open counts.** `open_jobs` is the raw count (every open posting, reposts across
  boards included, never duplicate-suppressed); `unique_open_jobs` is the de-duplicated
  figure Meilisearch's own index holds, what `/jobs` would show unfiltered. It is measured
  like `telegram_channels`: `cmd/rollup-stats` makes one `search.Client.Search` call (limit
  1, no filters) before publishing, best-effort — a missing `MEILI_MASTER_KEY` or an
  unreachable Meilisearch costs that one figure, not the snapshot.

## Per-source figures

`GET /api/v1/sources` and the public `/sources` page read ONE snapshot, `source_stats`,
rebuilt by a pass inside `cmd/rollup-stats` (`internal/ingest/sourcestats`) as an atomic
delete-and-reinsert.

- **The same two-count pair as `catalogstats`.** `open_jobs` is the raw open count —
  duplicates included — and the denominator the ATS-overlap figures are arithmetic on;
  `browsable_jobs` is Meilisearch's de-duplicated count, what `/jobs?source=<key>` shows and
  therefore what the PAGE displays. A card whose number the search it links to contradicts is
  the trap the pair exists to avoid.
- **`browsable_jobs` is NULL when Meilisearch could not be measured, never 0.** An
  unreachable index costs that one figure, not the run; a 0 would state on a public page
  that every source carries nothing.
- **The spine is `sourcestats.Union` over THREE sets**, each covering a hole the others
  leave: the adapter registry (without it a source with postings and no adapter —
  `telegram`, absent from `sources.Taxonomy()` — is dropped), this run's scan (without it an
  adapter whose postings have all closed is dropped, the case the page most needs to state),
  and the KEYS of the snapshot being replaced (without it a source in neither of the first
  two vanishes the moment it goes quiet). Only names carry over; every figure is
  re-measured. `GET /api/v1/sources` calls the same `Union` rather than keeping its own copy.
- **ATS overlap** counts rows the dedup pass matched to a first-party ATS posting
  (`jobs.duplicate_of_aggregator`); the remainder is `ats_unmatched` and is **never** called
  exclusive, on the wire or on the page — the pass finding no pair is absence of evidence.
  Two tests enforce the wording: one over the rendered card, one over the source of every
  file on the surface, including the page's own prose. It is carried for aggregators ONLY:
  the suppression pass never marks any other row, so a 0 there would read as "fully
  exclusive".
- The aggregate scan reads no `description` column (a guard test on the generated SQL) and
  excludes `is_private`.

### Source logos

Resolved in the browser from the source's DISPLAY NAME (`sourceLogoUrl`), never from a
posting host: the first version sampled `min(url)` per source and served the WRONG brand,
because an ATS posting's URL usually lives on the employer's own domain (on prod,
`greenhouse` sampled `bankrate.com`, `successfactors` a staffing agency), while a per-tenant
subdomain (`jobs.smartrecruiters.com`) 404s. By name, 14 of 15 resolve, and 7 of 8 sampled
sources return the byte-identical image whether asked by name or by an explicit `?domain=`.

The proxy answers a name it resolves WRONGLY with a 200, so a mismatch is invisible to every
consumer (see the `g2i` case behind `cmd/publish-logo-domains`). `SOURCE_LOGO_DOMAINS`
(`web/src/lib/logo.ts`) is the curated, measured escape hatch: of the 145 sources carrying
500+ jobs, 14 had no logo by name and all 14 resolve with the platform's own domain passed as
`?domain=`, which wins over the proxy's name map. It is an OVERRIDE, not a registry: a source
missing from it resolves by name, so the map going stale costs a logo rather than breaking
one. A curated domain implies a curated display name, and a test holds the two maps
together.

A miss still 404s, so every call site renders through the `EntityLogo` primitive rather than
a hand-rolled `<img>`: besides the `onerror` fallback it catches a 404 that landed BEFORE
hydration, which a server-rendered page needs because the browser fetches the image while no
handler exists yet.

### Excluded sources

A user's `excluded_sources` are seeded into the search filter by `filtersFromProfile`, the
same function the saved-search alert is built from — so avoiding a source from the job
page's provenance row (`JobSourceRow`) reaches both the feed and the digest.

## Sign-up attribution

The browser reports the visit that led to a brand-new account — campaign tags, the referring
HOST, the landing PATH, never a full URL — to `POST /api/v1/me/attribution`
(`internal/api/handler/me_attribution.go`), which keeps the first report per account in
`user_signup_attribution` (migration 0228), for accounts under ten minutes old only. This is
the complete count and does not depend on analytics consent; Google Analytics only sees the
visitors who accepted cookies (on 2026-11-02 it saw 228 of 644 sign-ups).

The same call sends the GA4 `sign_up` from the server through the Measurement Protocol
(`internal/engage/ga4mp`, needs `GA4_MEASUREMENT_ID` + `GA4_API_SECRET`, inert without them)
and answers `{"data":{"ga4_sent":bool}}`; the browser sends `sign_up` itself ONLY when that
is false, because GA4 has no event-level dedup and both sending would count the account
twice. Never send a client id the visitor did not give: it exists only after they accepted
analytics.

Source report — a LEFT JOIN, since accounts from before 0228, from mobile, or whose browser
could not report have no row:

```sql
SELECT coalesce(a.utm_source, a.referrer_host, '(direct)') src, count(*)
FROM users u LEFT JOIN user_signup_attribution a ON a.user_id = u.id
WHERE u.created_at >= :day GROUP BY 1 ORDER BY 2 DESC
```
