## Why

The first production run of the `whatjobs` source (added in #1268) failed and had to be rolled back.
Two defects, both invisible to the unit tests and to the pre-flight measurements:

**The feed's `keyword` is a relevance ranking, not a filter.** Board `rust developer` returned 193
postings of which 49 mentioned rust; the rest were one hospital group's *CT Technologist* and
*Nuclear Medicine Technologist* listings. Measured by page depth, relevance collapses immediately
after the first page:

| page | rows | mention `rust` | relevant |
|---|---|---|---|
| 1 | 47 | 47 | 100% |
| 2 | 26 | 4 | 15% |
| 3 | 30 | 4 | 13% |
| 5 | 43 | 0 | 0% |

So `total` (307 for that keyword) measures the padded result set, not the inventory — which means the
board file's keywords were selected against a fictitious number. The non-tech gate does not catch the
padding either: "Technologist" reads as technical, and it rejected only 16% of that board.

**HTTP 429.** Twelve sequential requests from one IP are fine, but the pipeline crawls boards in
parallel and 8 of 10 boards died on rate limiting.

Both have the same cure. Stopping a keyword once its results stop corroborating cuts the crawl from
40 pages to 2, which removes the junk and the request volume together.

## What Changes

- A posting is kept only when the keyword's **corroborating terms** appear in its title or
  description. Generic role words (`developer`, `engineer`, …) are dropped from the keyword first —
  they do not distinguish one posting from another — leaving the terms that carry the meaning:
  `rust developer` → `rust`, `kubernetes engineer` → `kubernetes`.
- **Pagination stops when a page stops corroborating.** Below a minimum share of corroborated
  postings the keyword is treated as exhausted, so the 40-page budget becomes a backstop rather than
  the usual stopping condition.
- The adapter's requests run under a **shared in-flight cap**, following `limitedTrudvsemGetter`, so
  parallel boards no longer trip the feed's rate limit.
- `sources/whatjobs.yml` keywords are re-annotated with corroborated volume instead of the feed's
  reported `total`, which overstates every slice.

## Capabilities

### Modified Capabilities

- `whatjobs-source`: postings must corroborate the keyword; pagination ends on collapsed relevance;
  requests are bounded by an in-flight cap.

## Impact

- **Touched:** `internal/sources/whatjobs.go` (+ test), `internal/sources/pacer.go` (in-flight cap for
  the provider), `internal/sources/registry.go` (wire the cap), `sources/whatjobs.yml` (comments).
- **Behavioural:** measured live across all ten keywords after the fix — **~3200 corroborated
  postings, none failing corroboration, and no rate limiting**. Per keyword the honest inventory
  varies far more than expected: `backend engineer` 1270 (collapses at page 32), `python developer`
  668 (page 17), `kubernetes engineer` 350, `ios developer` 212, `android developer` 171,
  `frontend engineer` 139, `node.js developer` 136, `react developer` 124, `golang` 86 (the feed runs
  dry with no padding at all), `rust developer` 51 (page 2). So the padding is not uniform: a broad
  keyword stays honest for dozens of pages while a thin one collapses immediately.
- **No schema, API, or web changes.**
