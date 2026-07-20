## 1. City-agnostic role fingerprint

- [x] 1.1 Add a shared separator helper (strip the last trailing clause after ` , ` / ` | ` / ` @ ` / spaced ` - ` ` — ` ` – `, suffix-only, keep original when result < 2 words) with unit tests covering the Towa case, the seniority-prefix case, and the < 2-word guard.
- [x] 1.2 Wire the strip into `normalizeRoleText` / `RoleFingerprint` in `internal/jobhash/rolefingerprint.go`; assert per-city variants hash equal and different-description roles hash differently.
- [x] 1.3 Confirm the description stays in the fingerprint (regression test: same stripped title + different description → different fingerprint).

## 2. Canonical geography union

- [x] 2.1 Add `RoleClusterGeoAll` query in `internal/db/queries/jobs.sql`: `(company_slug, role_fingerprint) → union(countries, regions, cities)` over open rows of multi-row clusters; regenerate sqlc.
- [x] 2.2 Build the geo lookup once in `cmd/reindex` (mirror `buildRealityLookup`/`RoleClusterCountsAll`) and thread it into `splitJobs` document building.
- [x] 2.3 Merge the cluster geo union onto the canonical document in `internal/search` (a `JobDocument` method), leaving non-canon reposts excluded; unit-test that a canon carries the union and a singleton is unchanged.

## 3. Backfill existing rows

- [x] 3.1 Decide backfill home (extend `cmd/backfill-derive` vs one-shot) and recompute `role_fingerprint` for open rows with the new function. → dedicated `cmd/backfill-role-fingerprint` (role_fingerprint is a jobhash, not jobderive, concern; matches the existing `cmd/backfill-*` family).
- [x] 3.2 Document the deploy order (backfill → reindex) in the change/ops notes so existing clusters collapse and canons gain unioned geography. → AGENTS.md commands + design.md migration plan.

## 4. Verification

- [x] 4.1 Behavioral check: the fingerprint collapses the real Towa titles (unit `TestRoleFingerprint_CollapsesCitySuffix`); the geo union widens a canon and leaves singletons unchanged (unit `TestMergeClusterGeography_*`, `TestSplitJobs_CanonGetsClusterGeoUnion`); `RoleClusterGeoAll` runs read-only on prod (3.98M rows) in ~84s returning 275,824 multi-row clusters (single-scan rewrite from the 3× self-join).
- [x] 4.2 Reality-signal counts share the city-agnostic fingerprint (RoleClusterCountsAll groups by `role_fingerprint`, unchanged), so merged per-city clusters count together post-backfill; documented in the `job-reality-signal` MODIFIED delta. Likely-evergreen still requires convergence, so counts alone do not reclassify.
- [x] 4.3 `go build ./... && go vet ./... && go test ./...` green.
