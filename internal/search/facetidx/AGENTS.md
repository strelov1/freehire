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
- **Immutable after `Build`.** Writes never touch an `Index`: `Store.Apply` builds the next one (below).
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

## Writes, generations, snapshots

- **`Batch`** (`write.go`): `Upserts` (full document JSON, a full replace), `Close`, `Duplicate`,
  `Delete` (job ids). Applied in that order; unknown ids are ignored; idempotent. An upsert
  sets the closed and duplicate marks from the document, so a reopened job is reopened. A
  batch with an undecodable document fails whole, before anything is built.
- **`Store`** holds an `atomic.Pointer[Index]`: `Current()` is lock-free, `Apply` and
  `Replace` are serialised by a mutex. `Apply` clones only what the batch touches: marks,
  `live`, the fields it writes (their four per-field bitmaps, the value bitmaps whose
  documents change, numeric column rebuilt by filter-and-merge), the field map and the
  id-overlay; everything else is shared with older generations, which stay valid for any
  reader still holding them. The failure mode to avoid is a write into a shared structure:
  `jobIDs` is appended past every older generation's length, `keys`/`vals` are cloned before
  they change, the shared id map (`docOf`) is never written (new ids go to a small `overlay`,
  merged into a fresh map every `overlayMax` new jobs).
- **Old values are found by scanning, not stored.** To replace a document the writer ANDs the
  replaced set against every value bitmap of the fields that hold any of them
  (`bm.Intersects`), so a write costs O(values of the touched fields) and no per-document
  record is kept (a record per document would cost hundreds of bytes each at 3M documents).
  Measured: 15,000 upserts (10,000 replacing) on 200,000 documents with a 20,000-value
  `company_slug`, 124-137 ms. If that ever shows, a per-document record kept by the writer
  is the seam.
- **Value ids are never reused**, so a value whose documents all left stays with an empty
  bitmap. Everything that reads `vals` must treat that as "no value" (counts skip zero,
  `neq` tests `postings`, not `len(vals)`). The display spelling of case variants is the
  smallest ever seen since the last build; a fresh build recomputes it from live documents,
  so after a spelling's last document leaves the two can differ (Meilisearch itself does not
  determine it).
- **Delete** removes the document's postings and sets `deleted`; the doc id and the job-id
  mapping stay, and a later upsert of that job revives the same document. Doc ids are never
  reused, so the index only grows. Compaction (renumbering live documents into a fresh
  Builder) is the seam; `Replace` with a rebuild does it today.
- **Forward arrays are the trade-off.** Rebuilding a field's CSR per batch costs O(postings of
  the field); a batch drops the arrays of the fields it touches and a single background
  goroutine rebuilds the missing ones from the current generation, installing them only if
  no newer generation appeared meanwhile (same generation number, identical content). In
  between, counting uses the AND strategy: exact, slower for a high-cardinality facet under
  a big filter. Under a drain that publishes faster than a rebuild takes, the arrays may
  stay off; that costs latency, never correctness.
- **Snapshot** (`snapshot.go`, format documented there): magic + version, job ids, the three
  marks, then per attribute its three bitmaps, values with their bitmaps and the numeric
  column, then a CRC-32C of everything. `all`, `live`, the id map, keys, cardinalities,
  prefix sums, `valued` and the forward arrays are recomputed on load. Damage of any kind is
  an error wrapping `ErrBadSnapshot`, never a half index. 200,000 synthetic documents:
  5.5 MB, 13 ms to write, 48 ms to load. Bump `snapshotVersion` when the layout changes; a
  version mismatch is an error, and the caller rebuilds from Postgres.
- **The contract** is `TestBatchesEqualFreshBuild`: after any sequence of batches the store
  answers every tested filter and facet exactly as a `Builder` over the resulting documents.

## Not here yet
The HTTP service (`cmd/facetd`), the Postgres rebuild and the router in
`internal/search/search`. Range filters build a bitmap from the sorted column, so a bound that
selects most of a multi-valued column costs a pass over it; a bucketed index is the seam if it
shows in shadow traffic.
