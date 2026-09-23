## Context

`search.Client.ListSitemapPage` reads the jobs index through Meilisearch's
**`/documents`** endpoint. That endpoint addresses any offset directly — which is why
it replaced a Postgres `row_number()` walk that had grown to 64s — but it returns
documents in internal storage order and takes no `sort`. Sorting exists only on
`/search`. So the 70 paged job sub-sitemaps are correct in content and arbitrary in
order, while `web-ssr-seo` has required "newest first" all along.

Measured on the live index on 2026-09-22, sorting by `created_at:desc` under the
existing `jobSitemapFilter`:

A first pass probed with a tiny `limit`, which measures the sort but not the page —
and page size dominates a deep read, so those figures could not settle the chunk count.
Re-measured warm at the page size this route actually serves (`limit=10000`, projected
to slug and lastmod):

| offset | processing time | wall |
|---|---|---|
| 0 | 716 ms | 0.74 s |
| 10,000 | 963 ms | 0.98 s |
| 20,000 | 1,292 ms | 1.31 s |
| 30,000 | 1,776 ms | 1.79 s |

The SSR route's fetch timeout is 10s, so the deepest chunk carries a 5.6x margin. A
cold cache costs materially more (a tiny-limit probe at offset 30,000 measured 7,151 ms
cold against 2,006 ms warm), which is the case the bound keeps bounded. `created_at` is already in the index's `sortableAttributes`
(verified live), so no settings patch is needed and the settings-before-binary hazard
does not arise.

## Goals / Non-Goals

**Goals:**

- A crawler reading the sitemap index top-down reaches the newest postings first.
- The ordering guarantee is tested by asserting order, not shape.
- No change to the existing sub-sitemaps' content, tiling, URLs, or cost.

**Non-Goals:**

- Ordering the full catalogue. A sorted `/search` at offset 660,000 is not measured
  and not needed; the paged chunks stay on `/documents`.
- Raising IndexNow's throughput (`cmd/search-ping` sends 4,800/day against ~11k/day
  of eligible flow). That is a real, separate gap, and it feeds Bing, not OpenAI.
- Anything about the 719,679 postings carrying `is_tech IS NULL`, which no sitemap
  lists at all. That is an enrichment question, and it wants measuring before fixing.

## Decisions

**A separate fresh sub-sitemap, not a sorted main one.** Switching
`ListSitemapPage` to `/search` would order everything, but it puts every one of the
70 chunks on an unmeasured deep sorted offset against a 10s timeout, and the deepest
is 22x deeper than anything measured here. A second, shallow reader adds one route
and risks nothing the existing files already do. Considered and rejected: sorting
everything (unmeasured tail risk), and re-sorting the documents at ingest so storage
order happens to be date order (depends on an implementation detail of the engine,
and a rebuild would silently undo it).

**Four chunks of 10,000, offset-bounded at 30,000.** 10,000 is the size already
proven in production for this route. Four chunks is 40,000 URLs ≈ 3.5 days of flow
(the live index holds 33,929 eligible postings created in the last 3 days), so a
crawler that skips two days still loses nothing. The bound is enforced server-side,
so a hand-written `?offset=500000` cannot reach an unmeasured depth.

**Offset paging, not a date-range filter.** `created_ts` is in
`filterableAttributes`, so a time-windowed variant is available — each chunk its own
hour range, always at offset 0, always a few milliseconds. It is not needed: the
measured margin is 5x, and a time window makes chunk sizes vary with the crawl rate,
so a busy day silently overflows a file. Noted as the seam to reach for if the flow
grows enough to make offset 30,000 expensive.

**The fresh entries go first in the index.** Ordering within a sitemap index is not
binding on any crawler, but it is the only signal available, and it costs nothing.

## Risks / Trade-offs

- **A cold Meilisearch cache costs several times the warm figure** → the bound at
  30,000 keeps the worst case at the one measured depth rather than deeper; a timed-out
  chunk is one missing file on one fetch, and the crawler retries. If cold reads prove
  common in practice, drop to three chunks (offset bound 20,000, 1,292 ms warm).
- **The caller controls the depth through `limit`, not only `offset`** → the bound is
  applied to both: `pageParamsBounded` is given `jobSitemapChunk` as its ceiling rather
  than `sitemapMaxURLs`, so the deepest reachable hit is 40,000. Without that,
  `?offset=30000&limit=50000` reaches hit 80,000 on a public route — found in review,
  and it is the failure `pageParamsWindowed` already records from 2026-09-14: a
  per-minute budget cannot bound an endpoint whose per-request work the caller sets.
- **Offset paging a sorted, growing list drifts between chunk fetches** → at ~11k new
  eligible postings a day, an hour between fetching chunk 0 and chunk 1 shifts the
  window by ~460 documents, so a crawler sees a small overlap or gap. Harmless (the
  paged files already make this argument) and self-correcting on the next fetch, but
  the sorted file has it more acutely, so it is written down rather than discovered.
- **A job appears in both the fresh file and a paged chunk** → permitted by the
  sitemap protocol and already true of any re-tiling; crawlers deduplicate by URL.
- **The fresh window silently stops being fresh if the flow collapses** → the file
  then lists the newest 40,000 whatever their age, which is still correct, just less
  useful. No failure mode, no alarm needed.
- **The effect is not attributable from inside the system** → measure it the same way
  the problem was found: re-run the crawl-coverage measurement 7 days after deploy,
  scoped to jobs created after the deploy, against today's 17.9% baseline.

## Migration Plan

1. Merge and deploy; the routes go live but nothing links to them yet if the index
   entries are deployed in the same build (they are — one change).
2. Verify on prod: fetch `/sitemap-jobs-fresh.xml`, confirm valid XML, and that
   `/sitemap.xml` lists the four fresh entries before the paged ones. Confirm the ORDER
   by looking the listed slugs up in `jobs.created_at`, not by reading the `<lastmod>`
   values — those are `updated_at`, the file is ordered by `created_at`, and a posting
   re-touched by a crawl carries a newer `<lastmod>` than a posting created after it.
   A descending-`<lastmod>` check would therefore pass against a wrongly-ordered file.
3. Wait 7 days, then re-run the coverage measurement.

Rollback is removing the four entries from the sitemap index: the routes then serve
nobody and the sitemap is byte-identical to today's. No data is written, so there is
nothing to undo.

## Open Questions

None. The one open number — whether four chunks or three — is settled by the measured
warm timings. Changing it later means moving TWO constants, in two languages:
`freshSitemapMaxOffset` (Go) and `FRESH_SITEMAP_CHUNKS` (TypeScript). Moving only the
Go one leaves a fourth `<loc>` in the index pointing at a file the API deliberately
answers empty, which looks exactly like a working sitemap. This mirrors the existing
`jobSitemapChunk`/`JOB_SITEMAP_CHUNK` coupling the repo already lives with.
