# facetidx conventions

## Scope
The in-memory facet engine behind facetd (openspec/changes/facetd): roaring bitmaps per
attribute value, a filter evaluator over the Meilisearch fragments our own code builds, and
facet counts, totals and distinct counts that must equal Meilisearch v1.49's. It imports
nothing from `internal/search/search` (that package will import this one); only its external
integration test does.

## Always true

- **Built from the document JSON the drain sends Meilisearch**, not from Postgres rows
  (`Builder.Add` takes the bytes, or any value that marshals to them). Only the attributes in
  `indexedAttrs` are kept: exactly the filterable attributes of `search.facetSettings` minus
  `id`, `is_closed` and `is_duplicate`. Add a filterable attribute there and here together.
- **Immutable after `Build`.** A later generation clones the bitmaps it touches.
- **The caller's filter arrives WITHOUT the open-jobs clause.** Every answer subtracts closed,
  duplicate and deleted documents itself (`Index.live`).
- **An unknown filter shape is `ErrUnsupportedFilter`, never match-all.** The grammar is
  `AND`/`OR`/`NOT`, parentheses, `= != < <= > >=`, `IN [..]`, `NOT IN [..]`, `EXISTS`,
  `NOT EXISTS`, `IS EMPTY`, `IS NOT EMPTY`; `id` maps job ids to documents. Top-level `[]string`
  is AND, `[][]string` is AND of OR groups; an empty group constrains nothing.
- **Two counting strategies, chosen per facet and request** by estimated cost: AND each value's
  bitmap with the filter (sum of min(|value|, |filter|), by binary search over sorted
  cardinalities) or walk the filter's documents through the forward array (CSR, uint16 ids
  below 65536 values). Forward arrays exist only for facets with at least 64 values, or 16 when
  multi-valued.

## Meilisearch rules this copies (pinned by `parity_integration_test.go`, 1,103 queries)

- `F != x` matches every document whose F is not x, those WITHOUT F included; but when no
  document carries F at all it matches nothing. `F NOT IN [..]`, `NOT F ...`, `F NOT EXISTS`
  and `F IS NOT EMPTY` are plain complements and match documents without F. (`TestParityAbsentAttribute`
  is the only thing that exercises the first exception.)
- String comparison is case-insensitive. The spelling a distribution reports for case variants
  depends on the order of the indexing batches in Meilisearch, so the parity test compares
  keys lowercased; facetidx reports the bytewise smallest spelling.
- A numeric literal matches a string value too (`x = 5` and `x = "5"`); numbers are indexed both
  ways, and ranges read only the numeric column.
- Distinct (`distinct: attr`): a matching document with no value counts as one of its own.
- A distribution is cut to 1200 values, highest count first. The values above the last count
  are deterministic; WHICH of the values on the last count survive is Meilisearch's traversal
  order of its facet level tree (neither ascending nor descending by value, measured), so the
  parity test compares those by number only. facetidx breaks that tie by descending value.
- `facetStats` are min and max of the numeric column among matching documents, absent when none match.
- `null` and `[]` are present (EXISTS); `[]` and `""` are EMPTY; `null` is not EMPTY. The
  generated documents never carry `null`, so that last part follows Meilisearch's documentation
  and is not pinned by the parity test.

## Not here yet
Writes and copy-on-write generations, snapshots, the HTTP service (`cmd/facetd`) and the router
in `internal/search/search`. Range filters build a bitmap from the sorted column, so a bound that
selects most of a multi-valued column costs a pass over it; a bucketed index is the seam if it
shows in shadow traffic.
