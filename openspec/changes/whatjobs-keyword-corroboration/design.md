## Context

`whatjobs` shipped in #1268 and failed its first production run. Rolled back by closing its 282 rows
and deleting them from the Meilisearch index (they were searchable — incremental indexing pushes at
ingest time, so closing rows in Postgres alone leaves them visible).

The root cause is that the feed's `keyword` ranks rather than filters. Measured on `rust developer`:
page 1 is 100% relevant, page 2 is 15%, page 5 is 0%. `total` reports 307 for that keyword; the real
inventory is roughly 50. Every keyword volume in the board file was therefore chosen against a
fictitious number.

## Goals / Non-Goals

**Goals:**

- Keep only postings that demonstrably belong to the keyword they were found under.
- Stop paging a keyword once the feed starts padding, which also cuts the request volume that caused
  HTTP 429.
- Bound parallel request concurrency for this provider.

**Non-Goals:**

- Salvaging the padded postings by other means. They are unrelated inventory, not mislabelled roles.
- Raising yield. Post-fix output is honestly small (~50–90 per keyword); that is the inventory.
- A generic relevance-corroboration layer for other aggregators. One provider needs it; the seam is
  noted, not built.

## Decisions

### Corroboration on the keyword's non-generic terms

`developer`, `engineer`, `programmer`, `dev`, `specialist`, `architect` are dropped from the keyword
before matching, because they appear in nearly every technical posting and would corroborate the
radiology padding just as readily as a real match. What remains carries the meaning. Verified against
live first pages — `backend engineer` 45/45, `ios developer` 49/49, `kubernetes engineer` 45/45,
`rust developer` 47/47 kept — so the rule costs nothing where the feed is honest.

Matching is a case-insensitive substring over title + description. Not word-boundary regex: `ios`
must match `iOS`, and `node.js` must match `Node.JS`. A substring risks a false positive (`rust` in
"trust"), which is the cheap direction of error — the term still has to appear, and the alternative
(dropping a real match on tokenization) is worse.

*Alternative considered:* an explicit `required_term` per board-file entry. Rejected as redundant —
it would restate the keyword in almost every row, and a row where it genuinely differs is a sign the
keyword itself is wrong.

### Relevance collapse ends pagination

A page whose corroborated share is below **50%** ends the keyword. The measured cliff is 100% → 15%,
so any threshold in that gap behaves identically; 50% is chosen for being obviously mid-gap rather
than tuned to one sample. Postings already corroborated on that page are kept — they are real, the
page merely marks the end of the relevant run.

This subsumes the rate-limit problem: `rust developer` goes from 40 pages to 2. The 40-page budget
stays as a backstop for a keyword that never collapses.

*Alternative considered:* keep paging and filter. Rejected — it spends 38 requests per keyword to
discard almost everything, which is exactly what produced the 429s.

### In-flight cap over request pacing

`concurrencyLimitedJSONGetter` already exists for `trudvsem` ("its gov API degrades under the
pipeline's board concurrency"). Reused with a cap of **2**. A rate pacer for JSON does not exist yet
and is not built here: sequential requests were never rate-limited in testing, so the trigger is
concurrency, and with the crawl now 2 pages deep per keyword the total volume is an order of
magnitude smaller regardless.

## Risks / Trade-offs

- **A real posting that never names the term is dropped** — a Rust role advertised only as
  "Systems Engineer (memory-safe languages)". → Accepted deliberately: the inverse error puts
  radiology listings in a developer's search results, which is far more damaging to trust than a
  missed posting the catalogue never had.
- **The 50% threshold is calibrated on one keyword's cliff.** → The cliff is steep enough that the
  exact value is not load-bearing; the log line reports why a crawl stopped, so a misfire is visible
  in one run rather than silent.
- **Yield.** Measured after the fix: ~3200 corroborated postings across the ten keywords, none
  failing corroboration and no rate limiting. That is well above the ~50–90 per keyword this design
  first predicted — the collapse point varies from page 2 (`rust developer`) to page 32
  (`backend engineer`), so a broad keyword stays honest much longer than the `rust developer` sample
  suggested. The prediction was wrong in the safe direction, but it was still a prediction from one
  sample: the corroborated counts now live in the board file, and they are the numbers to reason from.
- **`backend engineer` collapses at page 32, close to the 40-page budget.** → If it grows past the
  budget the crawl becomes bounded again and the log says so. Worth splitting into narrower keywords
  at that point rather than raising the budget, since deeper pages are where padding lives.

## Migration Plan

1. Merge, deploy, then run `sources/whatjobs.yml` by hand and read the corroborated counts.
2. Re-annotate the board file's keyword volumes from that run.
3. Only then consider a cron entry.

`WHATJOBS_PUBLISHER_ID` is already set on prod and no cron references the provider, so nothing runs
until triggered. Rollback is the recipe already exercised: close the rows, then delete them from the
`jobs` index by filter.
