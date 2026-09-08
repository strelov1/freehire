## Why

The job sitemap offers a crawler the whole live search index. The site is "the open-source
search engine for tech jobs"; two thirds of what the sitemap names is not a tech job.

Measured on prod 2026-09-08 against the live `jobs` index (2,014,462 documents, which is
exactly what `/sitemap.xml` enumerates today):

| Slice | Documents | Share |
|---|---:|---:|
| Everything the sitemap names today | 2,014,462 | 100% |
| `is_tech = non_tech` | 1,295,388 | **64.3%** |
| `is_tech` resolved by nothing | 47,906 | 2.4% |
| `is_tech = tech` | 670,890 | 33.3% |
| `is_tech = tech`, minus `likely-evergreen` | **669,352** | 33.2% |

(Counts come from separate probes minutes apart and the index moves under ingest — the total
read 2,014,007 at the first and 2,014,462 at the last. Nothing here turns on the last three
digits.)

By category the non-tech mass is not a fringe: management 301,387, sales 191,969, healthcare
181,760, support 143,699. `software_engineering` is 97,222 — 4.8% of what we ask Google to
crawl. A sampled sitemap page returned 39 non-tech postings out of 60 (Senior Transactional
Attorney, Community Association Manager, Dispatcher Assistant).

A sitemap is a claim about what is worth crawling. Ours currently claims 1.3M pages that
contradict what the site says it is, and spends the crawl budget of the 670k that do not.

This is a sitemap-scope change only. Nothing leaves the catalogue, the API, or search: a
non-tech posting keeps its page, its URL and its place in `/jobs`. Only the crawl invitation
narrows.

## What Changes

- **`sitemapPage` gains a filter.** `internal/search/search/sitemap.go`'s shared reader takes a
  Meilisearch filter expression. `ListSitemapPage` (jobs) passes one; `ListCompanySitemapPage`
  passes none, unchanged.
- **The job sitemap's scope becomes `is_tech = "tech" AND reality.class != "likely-evergreen"`.**
  The first clause is the whole of the change's value. The second excludes the 1,538 postings
  the catalogue's own classifier flags as perpetual listings — 0.2%, and the one reality class
  that is an accusation rather than an age bucket. Inviting a crawler to a posting we publish a
  ghost verdict about (`/features/ghost-jobs`) is a contradiction we can close in the same
  clause.
- **`CountSitemapDocuments` inherits the filter for free**, because it already delegates to
  `ListSitemapPage`. The count the sitemap index tiles by and the pages it names cannot
  disagree — there is no second place to keep in sync.
- **No settings patch and no reindex.** `is_tech` and `reality.class` are already declared in
  `facetSettings().FilterableAttributes`, so the "settings must reach the LIVE index before the
  binary that filters on it" hazard does not apply here.
- **One stale comment is corrected.** `sitemapPage`'s own performance note records "offset 0 and
  offset 1.2M both answer in under 0.25s" from 2026-08-16 at 1.26M documents. Re-measured
  2026-09-08 at 2.01M documents, the deepest unfiltered page takes 4.43s. The lines are being
  edited anyway.

## Impact

- 203 job sub-sitemaps become 68. `/sitemap.xml` re-renders from the new count, so the retired
  offsets stop being listed; a crawler holding a stale index and requesting one gets an empty
  `<urlset>`, which is already this route's documented behaviour. Nothing 404s.
- Search Console will report a large drop in discovered URLs. That is the intent, not a
  regression.
- Worst-case render is unchanged: the deepest filtered page measured 4.07s against today's
  deepest unfiltered 4.43s, both against the SSR route's 10s fetch timeout.

## Non-Goals

- **The company sitemap.** Its scope is `companies.job_count`, a different predicate computed by
  `RefreshCompanyFacets`, and narrowing it to companies with a tech role is its own change.
- **The chunk size.** 10,000 was sized by the fetch timeout, and the margin has eroded as the
  catalogue grew. Halving it would restore the margin, and it deserves its own measurement.
- **Excluding `stale`.** Considered and rejected on evidence — see design.md.
