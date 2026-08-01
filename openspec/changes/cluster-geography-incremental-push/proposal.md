## Why

A collapsed multi-city role loses the cities its reposts hold, every time anything touches it,
until the next full rebuild.

Four call sites turn a `db.Job` into the document Meilisearch holds, and they answer the
geography question differently:

| site | widens with the cluster's geography? |
|---|---|
| `cmd/reindex/main.go:530` (full rebuild) | yes — `doc.MergeClusterGeography` |
| `cmd/ingest/store.go` (per crawled row) | no |
| `internal/linkimport/linkimport.go` (per imported link) | no |
| `cmd/embed/indexer.go` (per newly embedded row) | no |

That would be harmless if the incremental pushes were additive. They are not.
`search.Client.SubmitJobs` uses `UpdateDocumentsWithContext` — a field-level update — and
`jobview.Job`'s `Countries`, `Regions` and `Cities` carry no `omitempty`, so those three keys
are present in every pushed document. An incremental push therefore **replaces** the widened
union with the canon's own narrow set. A role open in Kraków, Wien and Düsseldorf that collapses
to the Düsseldorf canon is findable by all three after a reindex, and by Düsseldorf alone as
soon as its next crawl reports any content change.

The behaviour was specified, not merely unguarded. `ingest-content-dedup`'s requirement reads
"When the search document for a canonical job is built **by a full reindex**, it SHALL carry the
union…". Scoping the guarantee to the rebuild is what left the three incremental writers free to
undo it, and `MergeClusterGeography`'s own doc comment records the reason: only the reindex had
the whole cluster in view, because the only cluster-geography query in the schema
(`RoleClusterGeoAll`) is a whole-catalogue aggregate.

## What Changes

- A new `RoleClusterGeo` query returns one cluster's geography union, keyed by
  `(company_slug, role_fingerprint)` — the per-row counterpart of the whole-catalogue
  `RoleClusterGeoAll`, exactly as `RoleClusterCount` is the per-row counterpart of
  `RoleClusterCountsAll`.
- The three incremental pushers call `doc.MergeClusterGeography` with it, so a push widens the
  canon instead of narrowing it.
- They ask only when there is something to widen with. Each already fetches `RoleClusterCount`
  for the reality signal; `mass_count <= 1` means the canon is the cluster's only open row, so
  the union is its own geography and the second query is skipped. This is exact, not an
  approximation.
- The requirement drops "by a full reindex": the guarantee is about the document, whichever
  writer produces it.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `ingest-content-dedup`: "The canonical job unions its cluster's geography" stops being scoped
  to the full reindex and gains the incremental-push case.

## Impact

- `internal/db/queries/jobs.sql` — one new query; `internal/db/` regenerated via `make sqlc`.
  No migration: the query reads existing columns.
- `cmd/ingest/store.go`, `internal/linkimport/linkimport.go`, `cmd/embed/indexer.go` — each
  gains the conditional lookup and the merge.
- One extra query per indexed row **only** for rows in a multi-row open cluster. Singletons —
  the overwhelming majority — are unaffected.
- No change to the full reindex, to `MergeClusterGeography` itself, or to any wire shape.
- Note for sequencing: PR #1385 (`worktree-job-derived-columns`) is open against
  `internal/linkimport/linkimport.go` too. Different region of the file (the upsert mapping, not
  the index push), but whichever lands second may need a trivial rebase.
