## Context

`RefreshCompanyFacets` already rebuilds each company's `regions` as the distinct
union of `jobs.regions` over its open jobs. `remote_regions` was bolted on as a
separate curated pipeline (dataset → dictionary → slug-matched backfill) because,
at the time, "where a company hires remotely" was framed as a declared fact. With
`jobs.work_mode` populated (199k open remote jobs, 93% with a resolved region),
the same signal is observable from our catalogue: it is just `regions` scoped to
remote jobs.

## Goals / Non-Goals

**Goals:**
- Derive `remote_regions` from open remote jobs, in the existing recompute.
- Delete the curated machinery (package, worker, dataset, query) — net simpler.
- Preserve the column, API facet, and UI pill unchanged.

**Non-Goals:**
- Including `hybrid` jobs (decision: `remote` only — a clean "fully remote" signal).
- Guessing a region for a remote job whose geography did not resolve (unchanged
  "never guess" stance — such jobs contribute nothing, exactly like the `regions`
  facet).
- A new column or read-model field — `remote_regions` already exists.

## Decisions

**Fold into `RefreshCompanyFacets`, don't add a worker.** `remote_regions` is now
one more denormalized aggregate maintained by the single set-based recompute, in
lockstep with `regions`/`countries`/`job_count`. This deletes a whole cron worker
and its dataset rather than adding one. A new CTE mirrors the existing `reg` CTE
but filters `work_mode = 'remote'`; the UPDATE adds `remote_regions` to the SET
list and to the `IS DISTINCT FROM` change-guard so no-op rows are still skipped.

**`remote` only.** Per the product decision, the remote scope is
`work_mode = 'remote'`. `hybrid`/`onsite`/unclassified jobs never contribute, so
`remote_regions ⊆ regions` for every company.

**Remove, don't deprecate.** The project is MVP-fluid; the curated pipeline has no
other consumer, so `internal/remoteregion`, `cmd/backfill-remote-regions`,
`sources/remote-companies.csv`, and `SetCompanyRemoteRegions` are deleted outright.
The recompute-guard test flips to assert the recompute now *populates*
`remote_regions` from remote jobs.

## Risks / Trade-offs

- **Coverage skew toward crawled boards** — we only know remote regions where we
  hold a remote posting. → Accepted; this is strictly more coverage than the
  293-company PDF, and it self-heals as ingest runs. A company hiring remotely in a
  region we've never crawled simply isn't tagged there (same conservative bias as
  every other job-derived facet).
- **Loss of the PDF's declared values** for companies with no remote postings. →
  Accepted by decision — the derived signal replaces the curated one.
- **Eventual consistency** — a newly ingested remote job isn't reflected until the
  next recompute. → Same as every other company facet; no change.

## Migration Plan

1. Ship the `RefreshCompanyFacets` change + remove the curated code; regenerate sqlc.
2. Deploy; the migration story is nil (the column already exists in prod).
3. Trigger `cmd/recount-companies` once so `remote_regions` repopulates from remote
   jobs immediately (otherwise it lands on the next scheduled run).
4. One-off `UPDATE companies SET company_info = company_info - 'remote_regions_raw'`
   to drop the stale audit field.
5. Rollback: revert the binary; the column persists and the prior recompute logic
   simply stops writing `remote_regions` (values freeze). No schema rollback.

## Open Questions

- None. Hybrid-inclusion and reference-directory ideas are explicitly out of scope.
