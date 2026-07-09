## Why

The `companies.remote_regions` facet currently comes from a static, hand-curated
external directory (the Atul Kumar PDF, loaded by `cmd/backfill-remote-regions`):
only 293 of our companies matched, it never updates, and it duplicates a signal we
can already observe. We hold ~199k open **remote** postings, 93% with a resolved
region — the regions where a company *actually* hires remotely are derivable from
our own catalogue, self-updating, and cover orders of magnitude more companies
than the PDF. The curated list has served its bootstrap purpose; replace it with a
job-derived facet.

## What Changes

- **`companies.remote_regions` becomes a job-derived facet**, maintained by the
  existing periodic recompute (`RefreshCompanyFacets` / `cmd/recount-companies`)
  as the distinct union of `regions` over the company's **open remote jobs**
  (`closed_at IS NULL AND work_mode = 'remote'`) — a remote-scoped sibling of the
  existing `regions` array. This **inverts** the prior invariant: the recompute now
  owns `remote_regions` instead of leaving it untouched.
- **BREAKING (internal):** remove the curated-backfill machinery — delete
  `internal/remoteregion`, `cmd/backfill-remote-regions`, `sources/remote-companies.csv`,
  and the `SetCompanyRemoteRegions` query. Drop the now-unused
  `company_info.remote_regions_raw` audit field going forward.
- **Unchanged:** the `remote_regions` column itself, the `remote_regions` overlap
  facet on `GET /api/v1/companies`, and the "Remote hiring" filter pill in the SPA
  — they keep filtering the same column; only its data source changes.

## Capabilities

### Modified Capabilities

- `companies`: `remote_regions` is redefined from a curated backfilled column to a
  job-derived array (union of regions over open remote jobs), maintained by the
  facet recompute; the recompute requirement and the derived-facets requirement
  change accordingly.

### Removed Capabilities

- `company-remote-regions`: the curated dataset, the region-string mapping
  dictionary, and the slug-matched backfill worker are removed — the signal is now
  derived from jobs, not loaded from an external directory.

## Impact

- **DB access:** `RefreshCompanyFacets` gains a remote-scoped regions aggregate;
  `SetCompanyRemoteRegions`, `ListCompanies`/`CountCompanies` unchanged except the
  facet's provenance. Regenerate `internal/db`.
- **Removed code:** `internal/remoteregion/`, `cmd/backfill-remote-regions/`,
  `sources/remote-companies.csv`, `SetCompanyRemoteRegions`, their tests, and the
  AGENT.md convention (rewritten).
- **Data:** on the next recompute, every company's `remote_regions` is recomputed
  from its remote jobs, overwriting the 293 PDF-sourced values (accepted) and
  populating tens of thousands more; a one-off pass clears stale
  `company_info.remote_regions_raw`.
- **API/UI:** no shape change — same facet param, same pill.
