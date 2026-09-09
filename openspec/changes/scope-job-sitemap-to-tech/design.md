## Context

`/sitemap.xml` is a sitemap index whose job chunks are offset-addressed pages of the live
Meilisearch `jobs` index. `sitemapPage`'s own comment states the reasoning: the sitemap pages
the search index rather than the `jobs` table "because the indexes already hold exactly the sets
worth handing a crawler — open, non-duplicate, non-private, categorized".

Those four predicates are about whether a posting is *findable*. None of them is about whether
it is what this site is for. The index serves search, where a non-tech posting is a legitimate
result behind an explicit `is_tech` facet; the sitemap serves discovery, where it is a claim.
The two scopes have been the same set only because nothing ever separated them.

## Goals / Non-Goals

**Goals:**

- The job sitemap names tech postings and nothing else.
- The count the index tiles by and the pages it serves stay structurally incapable of
  disagreeing.
- No settings patch, no reindex, no change to what search returns.

**Non-Goals:**

- Changing what is *indexed*, *served* or *reachable*. Every non-tech posting keeps its page and
  its `/jobs` listing; only the sitemap stops naming it.
- The company sitemap, and the chunk size (see proposal.md).

## Decisions

### Where the predicate lives

`sitemapPage` takes a `filter string`; `ListSitemapPage` passes the jobs predicate and
`ListCompanySitemapPage` passes `""`. The alternative — a filter argument threaded from the HTTP
handler — was rejected: the handler has no opinion about what a crawlable job is, and a
per-request filter is a public surface nobody asked for.

What makes this placement safe is that `CountSitemapDocuments` already calls `ListSitemapPage`
with `limit 1` rather than counting on its own. The filter therefore reaches the count without
being written twice, and the sitemap index cannot tile by one population while its chunks serve
another. If the two had been independent readers this change would need a shared constant *and*
a test that they still agree; as written it needs neither.

### Excluding `stale` was considered and rejected

The obvious reading of "the sitemap is full of dead postings" is wrong, and the numbers say so.
`internal/job/jobreality/classify.go` is explicit that the classes are not a liveness verdict:
`fresh` is "at most 14 days old with no evergreen signal", `likely-evergreen` needs two
converging ghost signals, and `stale` is the comment's own "everything else".

Measured on prod 2026-09-08:

| Slice | Documents |
|---|---:|
| `tech AND reality.class = fresh` | 208,275 |
| `tech AND reality.class = stale` | 461,069 |
| `tech AND reality.class = likely-evergreen` | 1,542 |
| `tech AND created_ts < now-90d` | **0** |
| `non_tech AND created_ts < now-90d` | **0** |

(The reality rows are one probe and the `tech` total in proposal.md is another, so they differ
by a handful of documents ingested in between: 670,890 − 1,542 ≠ 669,352 by four. The index is
live; no decision here turns on that.)

No document in the index — of either kind — was first seen more than 90 days ago; the catalogue
retires its own tail through the unseen sweep, `close-chronic-boards` and `cmd/prune`. So a
`stale` tech posting is an ordinary open vacancy that is more than a fortnight old, and
excluding the class would have dropped 461,069 live tech jobs for their age. It would also have
made the sitemap churn completely every fourteen days, dropping URLs faster than a crawler of
this site's budget can reach them.

`likely-evergreen` is the only class that asserts something is wrong with a posting, and it is
1,542 documents inside tech. It is excluded because the product publishes a ghost verdict on
exactly these pages, not because it moves a number.

### Reality is a snapshot, and that is acceptable here

`jobreality` is documented as "TIME-DEPENDENT and computed at index/read time, never stored", and
`doc.Reality` is attached by `cmd/reindex`. A document's `reality.class` is therefore as old as
the last full rebuild, and the incremental `search-drain` will not refresh it (reality is not part
of `content_hash` — the same trap `is_tech` and `requires_clearance` already document).

For this filter that is tolerable and worth stating rather than fixing: the clause it feeds
selects 0.2% of the set, the rebuild runs on a timer measured in hours, and a sitemap is a crawl
hint rather than a contract — `sitemapPage` already says so where it accepts that offset paging
is not a stable cursor. Had the design leaned on `reality` for the *primary* clause, this
staleness would have been disqualifying.

### Performance: measured, not assumed

`GetDocumentsWithContext` already POSTs to `/documents/fetch`, so adding a filter changes the
request body and not the transport. Measured on prod 2026-09-08, `fields: [public_slug,
updated_at]`, `limit: 10000`:

| Query | Time |
|---|---:|
| unfiltered, `offset=0` (today) | 0.65s |
| unfiltered, `offset=1,900,000` (today's deepest) | **4.43s** |
| filtered, `offset=0` | 1.18s |
| filtered, `offset=300,000` | 2.19s |
| filtered, `offset=660,000` (new deepest) | **4.07s** |
| filtered, `limit=1` (what the count issues) | 0.035s |

The deepest page gets marginally cheaper, not more expensive, and the count stays free. Both
figures sit against the SSR route's 10s fetch timeout — which is also why the stale comment
claiming 0.25s at 1.2M documents has to go: it would tell the next reader there is a 40x margin
where there is a 2.5x one.

## Risks / Trade-offs

- **A large reported drop in Search Console.** Discovered URLs fall by two thirds within a crawl
  cycle. Expected; the mitigation is knowing it in advance rather than reacting to it.
- **`is_tech` unknown is excluded.** The attribute is written tech-or-non_tech-or-absent, so
  `= "tech"` drops postings no dictionary and no enrichment could classify. That is the right
  default for a claim: we do not assert a posting is a tech job when nothing established that.
  It does mean an enrichment gap costs sitemap coverage, which is a reason to watch the unknown
  bucket, not a reason to widen the filter.
- **The margin against the fetch timeout is thin and shrinking with the catalogue.** This change
  does not worsen it and does not fix it; the chunk size is the lever, deliberately left to its
  own change.

## Migration Plan

None. No schema, no index settings, no backfill. The change is live for the next render of
`/sitemap.xml`, and reverting it is reverting the commit — a crawler that read the narrow index
meets the wide one again with no broken URL in between, because the retired offsets answer with
an empty `urlset` rather than an error in either direction.
