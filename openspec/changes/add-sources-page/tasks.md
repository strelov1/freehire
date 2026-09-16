## 1. Snapshot storage

- [x] 1.1 Add the migration creating `source_stats` (source text primary key, `open_jobs`,
      `ats_matched_jobs`, nullable `browsable_jobs`, nullable `sample_url`, `measured_at`),
      with the nullable columns commented as "absent means not measured, never zero"
- [x] 1.2 Add the sqlc queries: the grouped per-source aggregate over open postings, the
      snapshot upsert, and the full snapshot read; run `make sqlc`

## 2. The rollup pass

- [x] 2.1 Add a `sourcestats` package under `internal/ingest` that folds the Postgres
      aggregate rows and one Meilisearch `source` facet distribution into the snapshot rows,
      with the Meilisearch leg optional — an absent distribution leaves `browsable_jobs`
      unset rather than zero
- [x] 2.2 Wire the pass into `cmd/rollup-stats`: one facet request for every source, one
      grouped scan, one upsert; an unreachable Meilisearch costs that figure and not the run
- [x] 2.3 Verify the aggregate query reads no description column (a test over the generated
      SQL, so a later edit that adds one fails here rather than on the host)

## 3. The public endpoint

- [x] 3.1 Add `GET /api/v1/sources`, iterating `sources.Taxonomy()` as the spine and joining
      the health rollup and the snapshot onto it by key, so an adapter with neither still
      appears with absent figures
- [x] 3.2 Carry `ats_matched_jobs`/`ats_unmatched_jobs` only for aggregators, and omit them
      entirely for other kinds rather than sending zeros
- [x] 3.3 Assert by test that the response carries no board identifier and no error text,
      the same sanitization `/status` holds itself to

## 4. The page

- [x] 4.1 Add the API client method and the `/sources` server load, memoized module-side for
      a short TTL, degrading per leg like `/open`
- [x] 4.2 Build the page: three kind groups, largest first within each, each row carrying the
      logo (lazy, monogram fallback), display name via `sourceLabel`, the de-duplicated count
      as a link to `/jobs?source=<key>`, last successful read and last yield
- [x] 4.3 Add the aggregator overlap block, worded as unmatched postings — with a test
      asserting the string "exclusive" appears nowhere in the component
- [x] 4.4 Add the client-side search box, including the explicit no-match state
- [x] 4.5 Add page SEO metadata

## 5. Reachability

- [x] 5.1 Link `/sources` from the footer
- [x] 5.2 Add `/sources` to the pages sitemap

## 6. Documentation

- [x] 6.1 Record the new worker pass and the `source_stats` snapshot where the other rollups
      are documented (`CLAUDE.md`'s worker list, `internal/ingest/AGENTS.md`)
