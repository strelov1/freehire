## Why

The catalogue is 3.51M open jobs but only 11% carry a confident technical signal —
generic ATS boards (workday 545k, trudvsem 273k, oracle 195k, ukg 164k) pour a
company's whole hiring into an IT job board, so `подсобный рабочий`, `caregiver`,
`registered behavior technician` and `hospice aid` are among the highest-frequency
titles. The non-IT mass costs disk, LLM budget, reindex time and rollup time, and
degrades every list surface. Removing it is only safe if the labelling is good
enough not to remove real IT along with it, so the removal has to be iterative and
auditable rather than one large rule applied once.

## What Changes

- Add a run-once `cmd/prune` worker that permanently deletes jobs matching a
  catalogue-fit rule, in capped batches, `--dry-run` by default, mirroring each
  deletion into the Meilisearch facet index and an archive table.
  **BREAKING** — this is the first hard delete of `jobs` rows; until now the
  lifecycle was soft-close only.
- Add a run-once, read-only `cmd/mine-titles` that reports the highest-frequency
  titles still carrying no `is_tech` signal, so each iteration can be aimed at the
  next real cluster and the remaining group measured.
- Add a `pruned_jobs` archive table recording what was deleted and under which
  rule, without `description`/`enrichment`, so an irreversible deletion stays
  auditable at ~50 MB instead of tens of GB.
- Reject non-technical postings at ingest, before the write path, so a deleted job
  does not return on the next hourly crawl. Rejections are counted separately from
  save failures.
- Add a `cmd/prune --boards` report listing `sources/*.yml` entries whose company
  has never shown any technical evidence, as input to striking those boards out by
  PR.
- Grow `classify.nonTechTitleTerms` with anchored terms for the residual clusters
  (`behavior technician`, `maintenance technician`, `car rental driver`, medical
  specialities), one cluster per iteration. No bare `technician`/`driver`/`server`.

## Capabilities

### New Capabilities

- `catalog-pruning`: the catalogue-fit rule, the hard-delete worker and its safety
  gates, the deletion archive, the residual-title miner, and the board-retirement
  report.

### Modified Capabilities

- `source-ingest`: the pipeline currently SHALL persist *each* fetched posting;
  it must now reject postings the non-tech dictionary flags, counting them
  separately from skips caused by construct/save errors.

## Impact

- **New code**: `cmd/prune`, `cmd/mine-titles`, one migration for `pruned_jobs`,
  one predicate plus two call sites in `internal/pipeline/pipeline.go`.
- **Modified**: `internal/classify/nontech.go` (terms, iteratively),
  `internal/pipeline` stats and per-board logging, `internal/db/queries` for the
  batch delete and archive insert.
- **Reused unchanged**: `classify.IsNonTech`, `jobderive.deriveIsTech`,
  `search.Client.DeleteJobs`, `cmd/backfill-derive`.
- **Data**: physical deletion of roughly 1.5M rows across several iterations.
  `user_jobs`, `user_job_analysis`, `job_reminders`, `subscription_matches`
  cascade away with the job (28 users / 827 interactions on prod, accepted).
  `jobs.duplicate_of` is `NO ACTION`, so each batch must extend to its duplicate
  cluster or the delete fails on the FK.
- **Docs**: `CLAUDE.md` and `docs/agents/job-lifecycle.md` currently state that
  closing is soft and a job is never deleted; both need the pruning exception.
- **Ops**: one `make reindex` and one `cmd/backfill-derive` at the end of the
  campaign, not per iteration — `is_tech` is absent from `content_hash`, so a flip
  on a surviving row does not reach the index on its own.
- **Out of scope**: bucket-C company pruning (needs a calibrated "is this an IT
  company" signal), retiring aggregator sources such as `trudvsem` wholesale, and
  any serving-layer filtering.
