## Why

A posting that the fuzzy-description pass suppresses takes its geography out of search with it:
the surviving canonical row inherits none of the hidden rows' `countries`, `regions` or `cities`,
so a candidate filtering by the hidden city is told the vacancy does not exist. Reported as
issue #2225 with two reproductions (COBS Bread — Sales Assistant in Chestermere; Intelcom /
Dragonfly — Operations Coordinator in Scarborough): both are readable by slug, both return
`total: 0` from `/agent/jobs/search`.

The cause is that every geography union in the codebase is keyed by
`(company_slug, role_fingerprint)`, and the fuzzy pass — by construction — only ever sees rows the
exact role pass left *un*clustered, so their fingerprints differ and the union never matches. The
fuzzy pass exists precisely to catch per-city variants of one role (its threshold was tuned on
them), so this is not an edge case: it is the pass's main population.

A second, independent defect compounds it. `duplicate_of_fuzzy` is never released. Migrations 0114
and 0115 moved the marker from `duplicate_of` into an owned column, and the fuzzy pass's candidate
queries still filter `duplicate_of IS NULL` — so a fuzzy-marked row is invisible to every
subsequent run and its marker survives the canon closing or the descriptions diverging. Two
comments in `jobs.sql` still promise the old, now-false reversibility.

## What Changes

- Replace the role-fingerprint geography union with a union over the **`duplicate_of` closure**,
  keyed by the owner job's id: a searchable row's facets widen with the geography of every open
  row whose duplicate chain terminates at it. This subsumes the role-cluster case (its members
  point at the canon) and adds the fuzzy and multi-hop cases the fingerprint key cannot express.
- **BREAKING (internal):** remove `RoleClusterGeo`, `RoleClusterGeoAll` and `RoleClusterGeoFor`.
  The three index writers (`cmd/reindex`, `cmd/search-drain`, `internal/ingest/linkimport`) ask
  the new closure queries by job id instead of by `(company_slug, role_fingerprint)`.
- Serve `/api/v1/jobs/:slug/copies` from the same closure instead of `ListRoleClusterCopies`'s
  role-fingerprint grouping, so the existing "Other locations (N)" tab reports the whole group.
  Opening a *suppressed* posting resolves to its owner's closure rather than to its own role
  cluster.
- Let the fuzzy pass **release** stale markers: its candidate queries admit rows the exact passes
  did not claim (including already fuzzy-marked ones), and `MarkFuzzyDuplicatesForCompany` writes
  NULL for a row no longer clustered — mirroring `RecomputeRoleDuplicatesForCompanies`. A released
  row re-enters `search_outbox` through the existing transition bookkeeping.
- Correct the two stale comments in `internal/platform/db/queries/jobs.sql` that assert the fuzzy
  pass is reversed by the standard recompute.

No web/ changes: the "Other locations" tab and the `/jobs/:slug/copies` page already render
whatever the endpoint returns. No new columns and no migration: `jobs_duplicate_of_idx`
(migration 0012) already indexes the traversal.

## Capabilities

### New Capabilities

None. This change corrects existing behaviour; every requirement it touches belongs to a spec that
already exists.

### Modified Capabilities

- `fuzzy-description-role-dedup`: the pass must be reversible — a marker is re-decided on every
  run and cleared when the row no longer clusters; suppressed rows must not remove their
  geography from the surviving canonical row.
- `job-cluster-copies`: the copies of a posting are its duplicate closure, not its role-fingerprint
  cluster; a suppressed posting resolves to its owner's closure.
- `ingest-content-dedup`: the canonical job's geography union is defined over the rows it
  represents — its duplicate closure — rather than over its role-fingerprint cluster.

## Impact

- **SQL** — `internal/platform/db/queries/jobs.sql`: three `RoleClusterGeo*` queries removed, two
  closure queries added (whole-catalogue / by-id-set — the link import passes a one-element slice
  rather than needing a third), `ListRoleClusterCopies` replaced,
  two fuzzy candidate queries and `MarkFuzzyDuplicatesForCompany` amended. `make sqlc` regenerates
  `internal/platform/db`.
- **Workers** — `cmd/reindex` (`buildClusterGeoLookup`, `splitJobs`), `cmd/search-drain/indexer.go`.
- **API** — `internal/api/handler/copies.go`. Response shape unchanged; membership widens.
- **Ingest** — `internal/ingest/linkimport`.
- **Search index** — the widened facets only reach Meilisearch through a rebuild or a drain push,
  so the fix requires a full `make reindex` after deploy, preceded by `REINDEX_DEDUP_ONLY=1` to
  release the stale markers. The reindex timer must be stopped for the manual run.
- **Tests** — `internal/platform/db/fuzzy_dedup_integration_test.go` pins the current
  never-release behaviour (`TestFuzzyDedup_CandidateTitlesSkipAlreadyMarkedRows`) and is inverted.
- **Risk** — the whole-catalogue closure query is recursive and runs against ~1.4M open rows during
  a rebuild; it must be measured before rollout. Prior art: an unbounded dedup pass once ran 23h
  against a 12h unit timeout.
