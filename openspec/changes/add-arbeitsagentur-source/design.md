## Context

The Bundesagentur für Arbeit exposes `jobsuche-service`. A spike established:

- `GET pc/v4/jobs?berufsfeld=<field>&size=100&page=N` with header `X-API-Key: jobboerse-jobsuche`
  returns HTTP 200 keyless. Response: `{ stellenangebote: [...], maxErgebnisse, page, size, facetten }`.
- Each posting: `refnr` (stable id, e.g. `13647-2027459153-S`), `titel`, `arbeitgeber` (company),
  `arbeitsort` (`{ort, region, land, plz, ...}`), `aktuelleVeroeffentlichungsdatum` (YYYY-MM-DD),
  and optionally `externeUrl` (present when the posting is re-listed from another board).
- **No description in the search payload.** The detail API (`pc/v2|v4/jobsdetails/<base64(refnr)>`,
  `ed/v1/jobdetails/…`) returns 403. But the public SSR page
  `https://www.arbeitsagentur.de/jobsuche/jobdetail/<refnr>` returns 200 and carries the
  `Stellenbeschreibung` block and a `<meta name="description">` summary.
- **externeUrl dominates.** For `was=Softwareentwickler`, 46/50 carried `externeUrl`. Filtering by
  `berufsfeld` (the agency's own taxonomy) rather than `was` raises the first-party share sharply
  (11–29 of 50). So `berufsfeld` is both the scoping and the quality lever.
- **Pagination cap.** `page*size ≲ 10 000` (page 400 → HTTP 400). Each IT berufsfeld totals
  ~700–10 400, so a `veroeffentlichtseit`-bounded window stays well inside the cap.
- Repeated `berufsfeld` params do NOT OR (multi-value → `maxErgebnisse=0`); each field is a separate
  query, hence a separate board file entry.

This is the DataArt/onstrider two-stage shape — enumerate, then per-item detail fetch over
`fetchDetails` — but with a keyed JSON search front (`GetJSONWithHeaders`) instead of a sitemap.

## Goals / Non-Goals

**Goals:**
- A keyless (public static key), board-per-`berufsfeld`, multi-company `arbeitsagentur` adapter that
  emits only first-party postings with a scraped description.
- Reuse `GetJSONWithHeaders`, `GetHTML`, and `fetchDetails`; no new shared helper unless a shape forces it.

**Non-Goals:**
- Ingesting `externeUrl` re-lists. They duplicate other boards and apply off-site; dropped.
- Following `externeUrl` to its destination (a linksource job); out of scope for this change.
- Full-backlog crawl. Each run is a `veroeffentlichtseit` window; the standard sweep soft-closes
  postings that age out and stop being emitted.
- German-language enrichment tuning. Skill tags survive; seniority/category will be sparse. Accepted.

## Decisions

- **Board = `berufsfeld`.** The board file entry's `board` is the professional-field label passed as
  the `berufsfeld` query param. Multi-company (company per posting = `arbeitgeber`), board-based, so
  the adapter carries neither the `boardless` nor `aggregator` marker and is filterable by default —
  the same posture as other board-keyed multi-company sources. *Alternative rejected:* `was=` keyword
  boards — far more `externeUrl` noise and less precise.

- **First-party = absence of `externeUrl`.** The mapper drops any posting with a non-empty
  `externeUrl`. *Alternative rejected:* keeping all and deduping later — the re-lists are mostly
  German boards we do not otherwise cover, so there is no first-party twin to dedup against, and the
  apply URL would point off-site.

- **Description from the SSR jobdetail page.** For each kept posting, `fetchDetails` GETs
  `https://www.arbeitsagentur.de/jobsuche/jobdetail/<refnr>` and extracts the `Stellenbeschreibung`
  content (falling back to the `<meta name="description">` summary when the block is absent). The
  same URL is the `Job.URL`. A detail fetch that fails or yields no description does not drop the
  posting — it is emitted with an empty description (title/company/location still carry value) and a
  failed page does not abort the crawl. *Alternative rejected:* shipping with no description at all —
  too thin; the SSR page is the only description source.

- **Pagination.** Loop `page=1..N` at `size=100`, stopping when a page returns fewer than `size`
  postings, when `page*size` reaches `maxErgebnisse`, or at the depth cap. Bound the set up front with
  `veroeffentlichtseit=<days>` so each run is a small, fresh window.

- **Public static key as a constant.** `X-API-Key: jobboerse-jobsuche` is a well-known public value,
  not a secret; it lives as a code constant and the provider registers unconditionally in
  `sources.All` (unlike the env-keyed USAJobs/Reed).

## Risks / Trade-offs

- **Detail fan-out.** Up to a few thousand SSR page fetches per berufsfeld per run. → The
  first-party filter drops the majority up front, and `veroeffentlichtseit` bounds each run to a
  fresh window; `fetchDetails` throttles with the default worker pool. If the site rate-limits the
  prod IP, enroll in `proxiedProviders` later (a config flip, noted as a seam — not built now).
- **German language weakens enrichment facets.** → Accepted; skill tags are language-agnostic and
  the raw title/company/location are still useful. Not a blocker.
- **`page*size` depth cap (~10 000).** A berufsfeld larger than the cap loses its deep tail. → The
  `veroeffentlichtseit` window keeps each run under the cap; if a field still exceeds it, the drop is
  logged rather than silent.
- **Static key could be revoked or the SSR markup could drift.** → Both surface as a board-health
  failure (search 4xx or empty descriptions); the mapping is pinned to real fixtures in tests.

## Migration Plan

- No DB migration, no API change, no new dependency, no secret.
- Deploy: add a `cmd/ingest sources/arbeitsagentur.yml` cron schedule in freehire-ops.

## Open Questions

- Does the prod datacenter IP get rate-limited on the SSR jobdetail pages? Resolve with a manual prod
  run; `proxiedProviders` enrollment is the fallback (config flip).
- Exact `veroeffentlichtseit` window (7 vs 14 days) — tune during implementation against real volume.
