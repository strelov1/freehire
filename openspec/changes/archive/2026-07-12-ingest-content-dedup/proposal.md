## Why

Sources repost the same role under many external ids: on prod, T-Bank alone has
1998 open jobs but only ~619 distinct descriptions (e.g. "Представитель" is 929
byte-identical rows). The repost-identity infrastructure already exists —
`jobs.role_fingerprint` (`hash(company_slug|norm_title|norm_description)`, migration
0003) is computed on every upsert and indexed, and the job-reality signal already
counts each role cluster (`RoleClusterCount`: repost/mass-posting counts). But
nothing **collapses** the cluster: all 929 reposts still appear as separate catalogue
cards and search hits, and each one is enriched — burning LLM budget on duplicates.

## What Changes

- The catalogue shows **one canonical job per role cluster** (`company_slug` +
  `role_fingerprint`); the other open reposts are hidden from the jobs list and
  search. Rows are still stored (the reality signal keeps counting them) — the
  collapse happens at **read/enrichment**, not by dropping inserts.
- A canonical marker `jobs.duplicate_of` is computed per cluster (canon = the
  deterministic `min(id)` among the cluster's open rows; the rest point to it),
  reusing the existing fingerprint + cluster machinery.
- The jobs list, the search index, and the enrichment enqueue all exclude
  non-canonical reposts — so duplicates leave the catalogue, drop out of search, and
  stop consuming enrichment budget.
- The canonical job surfaces its cluster's open count (the existing `mass_count`) as
  its number of openings, so "929 openings" is visible rather than 929 cards.

Explicitly OUT of scope: reworded / cross-source near-duplicates (different
fingerprint) — a separate Track-2 "possible duplicates" feature. Also out of scope:
a one-off backfill of existing rows (a follow-up), and changing the reality signal's
counting (rows are preserved precisely so it is untouched).

## Capabilities

### New Capabilities
- `ingest-content-dedup`: canonical-per-role-cluster selection and the read/enrichment
  collapse — the marker, its recompute, and the list/search/enrichment exclusions,
  built on the existing `role_fingerprint` and role-cluster counts.

### Modified Capabilities
<!-- none: role_fingerprint/reality-signal behaviour is reused unchanged, not respecified -->

## Impact

- `migrations/` — `jobs.duplicate_of` (bigint NULL, indexed). Manual prod apply before
  deploy (initdb migration gotcha). Regenerate `internal/db` via sqlc.
- `internal/db/queries/jobs.sql` — a `RecomputeRoleDuplicates` pass (canon per open
  cluster); the jobs-list and enrichment-enqueue queries gain `duplicate_of IS NULL`.
- `internal/search` — reindex + incremental push skip non-canonical rows.
- Recompute wired into the reindex path (and a cron sibling), mirroring the existing
  role-cluster/reality recompute cadence.
- `internal/jobview` — surface the canonical job's openings count (reusing the
  reality `mass_count`); no other wire-shape change.
