## 1. Share the canon lookup

- [x] 1.1 New `internal/jobdedup` with `CanonicalForRole`, carrying the rule that the canon
      must be OLDER than the row just written so the synchronous answer agrees with
      `RecomputeRoleDuplicatesForCompany` instead of being inverted by it
- [x] 1.2 Move `internal/linkimport`'s private copy onto it, leaving that caller's own gate
      (generic source only) at the call site

## 2. Mark the repost as ingest writes it

- [x] 2.1 Integration test: two per-city copies of one role through `dbStore.Save` leave the
      second marked `duplicate_of` the first, one document pushed to the index, one enrichment
      row — failing first on all three counts
- [x] 2.2 `dbStore.Save` asks `jobdedup.CanonicalForRole` inside the upsert transaction and
      marks the row when a canon exists
- [x] 2.3 A marked row skips the enrichment enqueue and the live index push
- [x] 2.4 `clustersByRole` gates the lookup to newly inserted, unmarked rows, with a unit test
      covering the re-crawl and already-marked cases

## 3. Documentation

- [x] 3.1 Delta spec on `ingest-content-dedup`: the marker is assigned at write time, with the
      recompute keeping the cases the write path deliberately skips
- [x] 3.2 Note the two-tier dedup in `internal/pipeline/AGENTS.md`

## 4. Not done here

- [ ] 4.1 Collapse the 30 live AgileEngine copies on prod — left to the scheduled reindex
- [ ] 4.2 Nothing narrows the window for a fan-out that arrives inside ONE crawl pass:
      concurrent board goroutines can each see no canon under READ COMMITTED, so a burst may
      leave a few unmarked copies for the recompute. Measure before deciding it needs a lock.
